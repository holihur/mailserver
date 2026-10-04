package imap

import (
	"bufio"
	"net"
	"os"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"
)

func readUntil(t *testing.T, r *bufio.Reader, tag string) []string {
	t.Helper()
	var out []string
	for i := 0; i < 80; i++ {
		l, _ := r.ReadString('\n')
		l = strings.TrimRight(l, "\r\n")
		out = append(out, l)
		if strings.HasPrefix(l, tag+" ") {
			return out
		}
	}
	t.Fatalf("未收到 tag %s 的回复，已读: %v", tag, out)
	return out
}

func startsWithAny(lines []string, sub string) bool {
	return strings.Contains(strings.Join(lines, "\n"), sub)
}

// #8：NAMESPACE / ID / CAPABILITY 扩展通告（无需 DB）。
func TestImapExtensions(t *testing.T) {
	srv, cli := net.Pipe()
	go handle(srv, func() string { return "mail.test.local" }, nil, nil, false)
	defer cli.Close()
	r := bufio.NewReader(cli)
	w := bufio.NewWriter(cli)
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	send := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }

	read()

	send("a CAPABILITY")
	caps := readUntil(t, r, "a")
	for _, ext := range []string{"MOVE", "UIDPLUS", "NAMESPACE", "ID", "QUOTA"} {
		if !startsWithAny(caps, ext) {
			t.Fatalf("CAPABILITY 应含 %s: %v", ext, caps)
		}
	}

	send("b NAMESPACE")
	ns := readUntil(t, r, "b")
	if !startsWithAny(ns, `NAMESPACE (("" "/"))`) {
		t.Fatalf("NAMESPACE 回复不正确: %v", ns)
	}

	send("c ID NIL")
	id := readUntil(t, r, "c")
	if !startsWithAny(id, "ID (") {
		t.Fatalf("ID 回复不正确: %v", id)
	}
}

// #8：MOVE 移动邮件、GETQUOTA 返回配额（需 DB 与 PAT）。
func TestImapMoveAndQuota(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, mails, mail_tokens, mail_folders RESTART IDENTITY CASCADE")
	u := model.User{Email: "imap@test.local"}
	g.Create(&u)
	plain, prefix, _ := auth.NewMailToken()
	g.Create(&model.MailToken{UserID: u.ID, Name: "t", Prefix: prefix, Hash: auth.HashMailToken(plain), Scopes: "imap"})
	g.Create(&model.Mail{UserID: u.ID, From: "a@x", To: "imap@test.local", Subject: "m1", Folder: "inbox"})
	g.Create(&model.Mail{UserID: u.ID, From: "a@x", To: "imap@test.local", Subject: "m2", Folder: "inbox"})

	srv, cli := net.Pipe()
	go handle(srv, func() string { return "mail.test.local" }, g, nil, false)
	defer cli.Close()
	r := bufio.NewReader(cli)
	w := bufio.NewWriter(cli)
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	send := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }

	read() // greeting
	send(`a LOGIN "imap@test.local" "` + plain + `"`)
	if l := readUntil(t, r, "a"); !strings.HasPrefix(l[len(l)-1], "a OK") {
		t.Fatalf("LOGIN 失败: %v", l)
	}
	send("b SELECT INBOX")
	if l := readUntil(t, r, "b"); !startsWithAny(l, "EXISTS") {
		t.Fatalf("SELECT 失败: %v", l)
	}
	send("c MOVE 1 Trash")
	mv := readUntil(t, r, "c")
	if !strings.HasPrefix(mv[len(mv)-1], "c OK") || !startsWithAny(mv, "COPYUID") {
		t.Fatalf("MOVE 失败: %v", mv)
	}
	var moved int64
	g.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", u.ID, "trash").Count(&moved)
	if moved != 1 {
		t.Fatalf("应有 1 封移动到 trash，实际 %d", moved)
	}
	send(`d GETQUOTA ""`)
	q := readUntil(t, r, "d")
	if !startsWithAny(q, "QUOTA") {
		t.Fatalf("GETQUOTA 回复不正确: %v", q)
	}
}
