package smtp

import (
	"bufio"
	"crypto/tls"
	"net"
	"os"
	"strings"
	"testing"

	"mailserver/internal/db"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

func smtpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.Exec("TRUNCATE users, mails, mail_aliases, mail_rules, mail_folders, sieve_scripts, contacts, mail_tokens RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return g
}

// client 通过 net.Pipe 与 handle 对话。
type smtpClient struct {
	r *bufio.Reader
	w *bufio.Writer
}

func dialHandle(t *testing.T, g *gorm.DB) *smtpClient {
	return dialHandleN(t, g, 1<<20)
}

func dialHandleN(t *testing.T, g *gorm.DB, maxBytes int64) *smtpClient {
	t.Helper()
	srv, cli := net.Pipe()
	go handle(srv, g, maxBytes, "", nil)
	t.Cleanup(func() { cli.Close() })
	c := &smtpClient{r: bufio.NewReader(cli), w: bufio.NewWriter(cli)}
	if !strings.HasPrefix(c.read(), "220") {
		t.Fatal("缺少 220 greeting")
	}
	return c
}

func (c *smtpClient) read() string {
	line, _ := c.r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func (c *smtpClient) send(s string) {
	c.w.WriteString(s + "\r\n")
	c.w.Flush()
}

// cmd 发送一条命令并返回一行回复（自动跳过 250-/354 多行前缀）。
func (c *smtpClient) cmd(s string) string {
	c.send(s)
	for {
		l := c.read()
		if strings.HasPrefix(l, "250-") || strings.HasPrefix(l, "354") {
			if strings.HasPrefix(l, "354") {
				return l
			}
			continue
		}
		return l
	}
}

func TestInboundStartTLSAdvertised(t *testing.T) {
	srv, cli := net.Pipe()
	go handle(srv, nil, 1<<20, "", &tls.Config{})
	defer cli.Close()
	r := bufio.NewReader(cli)
	w := bufio.NewWriter(cli)
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	if !strings.HasPrefix(read(), "220") {
		t.Fatal("缺少 greeting")
	}
	w.WriteString("EHLO x\r\n")
	w.Flush()
	var lines []string
	for {
		l := read()
		lines = append(lines, l)
		if !strings.HasPrefix(l, "250-") {
			break
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "STARTTLS") {
		t.Fatalf("EHLO 应通告 STARTTLS: %v", lines)
	}
}

func TestInboundMultipleRecipients(t *testing.T) {
	g := smtpTestDB(t)
	g.Create(&model.User{Email: "u1@test.local"})
	g.Create(&model.User{Email: "u2@test.local"})

	c := dialHandle(t, g)
	c.cmd("EHLO test")
	if !strings.HasPrefix(c.cmd("MAIL FROM:<ext@example.com>"), "250") {
		t.Fatal("MAIL FROM 应成功")
	}
	if !strings.HasPrefix(c.cmd("RCPT TO:<u1@test.local>"), "250") {
		t.Fatal("RCPT u1 应成功")
	}
	if !strings.HasPrefix(c.cmd("RCPT TO:<u2@test.local>"), "250") {
		t.Fatal("RCPT u2 应成功")
	}
	// 未知收件人不影响其它已知收件人
	if !strings.HasPrefix(c.cmd("RCPT TO:<nobody@test.local>"), "550") {
		t.Fatal("未知收件人应 550")
	}
	c.send("DATA")
	if !strings.HasPrefix(c.read(), "354") {
		t.Fatal("DATA 应回 354")
	}
	c.send("Subject: hi")
	c.send("")
	c.send("..leading-dot") // 透明传输：应存为 ".leading-dot"
	c.send(".")
	if !strings.HasPrefix(c.read(), "250") {
		t.Fatal("DATA 结束应 250")
	}

	for _, u := range []string{"u1@test.local", "u2@test.local"} {
		var n int64
		g.Model(&model.Mail{}).Where("folder = ? AND LOWER(\"to\") = ?", "inbox", u).Count(&n)
		if n != 1 {
			t.Fatalf("%s 应收 1 封，实际 %d", u, n)
		}
	}
	var m model.Mail
	g.Where("LOWER(\"to\") = ?", "u1@test.local").First(&m)
	if !strings.Contains(m.Body, ".leading-dot") || strings.Contains(m.Body, "..leading-dot") {
		t.Fatalf("点透明传输失败，body=%q", m.Body)
	}
}

func TestInboundSecondMailNoRecipientReuse(t *testing.T) {
	g := smtpTestDB(t)
	g.Create(&model.User{Email: "u1@test.local"})

	c := dialHandle(t, g)
	c.cmd("EHLO test")
	c.cmd("MAIL FROM:<a@example.com>")
	c.cmd("RCPT TO:<u1@test.local>")
	c.send("DATA")
	c.read()
	c.send("Subject: one")
	c.send("")
	c.send("body")
	c.send(".")
	c.read()

	// 第二封只发 MAIL FROM，不重发 RCPT：不得沿用上一封的收件人
	c.cmd("MAIL FROM:<b@example.com>")
	if rep := c.cmd("DATA"); !strings.HasPrefix(rep, "503") {
		t.Fatalf("无收件人时 DATA 应 503，实际 %q", rep)
	}
	var n int64
	g.Model(&model.Mail{}).Count(&n)
	if n != 1 {
		t.Fatalf("第二封不应投递，期望 1 封，实际 %d", n)
	}
}

// #2：超过大小上限 -> 552，且协议保持同步，邮件不入库。
func TestInboundMessageTooLargeResync(t *testing.T) {
	g := smtpTestDB(t)
	g.Create(&model.User{Email: "big@test.local"})

	c := dialHandleN(t, g, 1024)
	c.cmd("EHLO test")
	c.cmd("MAIL FROM:<ext@example.com>")
	c.cmd("RCPT TO:<big@test.local>")
	c.send("DATA")
	if !strings.HasPrefix(c.read(), "354") {
		t.Fatal("DATA 应回 354")
	}
	chunk := strings.Repeat("x", 300)
	for i := 0; i < 10; i++ { // 共 3000+ 字节 > 1024
		c.send(chunk)
	}
	c.send(".")
	if rep := c.read(); !strings.HasPrefix(rep, "552") {
		t.Fatalf("超限应回 552，实际 %q", rep)
	}
	// 协议未错位：下一条命令仍能正常响应
	if rep := c.cmd("NOOP"); !strings.HasPrefix(rep, "250") {
		t.Fatalf("超限后协议应同步，NOOP 实际 %q", rep)
	}
	var n int64
	g.Model(&model.Mail{}).Count(&n)
	if n != 0 {
		t.Fatalf("超限邮件不应入库，实际 %d", n)
	}
}
