package smtp

import (
	"bufio"
	"encoding/base64"
	"net"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

type submitClient struct {
	r *bufio.Reader
	w *bufio.Writer
}

func dialSubmitN(t *testing.T, g *gorm.DB, maxBytes int64) *submitClient {
	t.Helper()
	srv, cli := net.Pipe()
	go handleSubmit(srv, func() string { return "mail.test.local" }, g, nil, false, maxBytes)
	t.Cleanup(func() { cli.Close() })
	c := &submitClient{r: bufio.NewReader(cli), w: bufio.NewWriter(cli)}
	if !strings.HasPrefix(c.read(), "220") {
		t.Fatal("缺少 220 greeting")
	}
	return c
}

func (c *submitClient) read() string {
	line, _ := c.r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func (c *submitClient) send(s string) {
	c.w.WriteString(s + "\r\n")
	c.w.Flush()
}

func (c *submitClient) cmd(s string) string {
	c.send(s)
	for {
		l := c.read()
		if strings.HasPrefix(l, "250-") {
			continue
		}
		return l
	}
}

// #2：提交端口超过上限 -> 552，协议保持同步，邮件不入库。
func TestSubmitMessageTooLargeResync(t *testing.T) {
	g := smtpTestDB(t)
	u := model.User{Email: "sub@test.local"}
	g.Create(&u)
	plain, prefix, _ := auth.NewMailToken()
	g.Create(&model.MailToken{UserID: u.ID, Name: "t", Prefix: prefix, Hash: auth.HashMailToken(plain), Scopes: "smtp"})

	c := dialSubmitN(t, g, 1024)
	c.cmd("EHLO test")
	authLine := base64.StdEncoding.EncodeToString([]byte("\x00sub@test.local\x00" + plain))
	if rep := c.cmd("AUTH PLAIN " + authLine); !strings.HasPrefix(rep, "235") {
		t.Fatalf("AUTH 应成功: %q", rep)
	}
	c.cmd("MAIL FROM:<sub@test.local>")
	c.cmd("RCPT TO:<sub@test.local>")
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
	if rep := c.cmd("NOOP"); !strings.HasPrefix(rep, "250") {
		t.Fatalf("超限后协议应同步，NOOP 实际 %q", rep)
	}
	var n int64
	g.Model(&model.Mail{}).Count(&n)
	if n != 0 {
		t.Fatalf("超限邮件不应入库，实际 %d", n)
	}
}

// 正常大小可成功入队（sent）。
func TestSubmitOK(t *testing.T) {
	g := smtpTestDB(t)
	u := model.User{Email: "ok@test.local"}
	g.Create(&u)
	plain, prefix, _ := auth.NewMailToken()
	g.Create(&model.MailToken{UserID: u.ID, Name: "t", Prefix: prefix, Hash: auth.HashMailToken(plain), Scopes: "smtp"})

	c := dialSubmitN(t, g, 1<<20)
	c.cmd("EHLO test")
	authLine := base64.StdEncoding.EncodeToString([]byte("\x00ok@test.local\x00" + plain))
	c.cmd("AUTH PLAIN " + authLine)
	c.cmd("MAIL FROM:<ok@test.local>")
	c.cmd("RCPT TO:<ok@test.local>")
	c.send("DATA")
	c.read()
	c.send("Subject: hi")
	c.send("")
	c.send("body")
	c.send(".")
	c.read()

	var n int64
	g.Model(&model.Mail{}).Where("folder = ?", "sent").Count(&n)
	if n != 1 {
		t.Fatalf("应产生 1 封 sent，实际 %d", n)
	}
}
