package message

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mailserver/internal/model"
)

func mail() *model.Mail {
	return &model.Mail{
		ID: 7, From: "a@example.com", To: "b@x.com, c@y.com", Cc: "d@z.com", Bcc: "e@secret.com",
		Subject: "hi", Body: "line1\nline2", CreatedAt: time.Unix(1700000000, 0),
	}
}

func TestRecipients(t *testing.T) {
	got := Recipients(mail())
	want := []string{"b@x.com", "c@y.com", "d@z.com", "e@secret.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// 去重
	m := &model.Mail{To: "a@b.c", Cc: "A@B.C"}
	if r := Recipients(m); len(r) != 1 {
		t.Fatalf("dedup failed: %v", r)
	}
}

func TestPartsPlainNoBcc(t *testing.T) {
	hdrs, body := Parts("mail.example.com", mail())
	full := string(Serialize(hdrs, body))
	if strings.Contains(full, "Bcc") || strings.Contains(full, "secret.com") {
		t.Fatalf("Bcc must not appear in message:\n%s", full)
	}
	if !strings.Contains(full, "Cc: d@z.com") {
		t.Fatal("Cc header missing")
	}
	if !strings.Contains(full, "Content-Type: text/plain; charset=utf-8") {
		t.Fatal("plain content-type missing")
	}
	if !strings.Contains(full, "line1\r\nline2") {
		t.Fatal("body CRLF missing")
	}
}

func TestPartsAttachments(t *testing.T) {
	m := mail()
	atts := []Attachment{{Name: "a.txt", Type: "text/plain", Data: "aGVsbG8=", Size: 5}}
	b, _ := json.Marshal(atts)
	m.Attachments = string(b)

	hdrs, body := Parts("mail.example.com", m)
	full := string(Serialize(hdrs, body))
	if !strings.Contains(full, "multipart/mixed") {
		t.Fatal("should be multipart")
	}
	if !strings.Contains(full, `filename="a.txt"`) || !strings.Contains(full, "aGVsbG8=") {
		t.Fatal("attachment not embedded")
	}
	if strings.Contains(full, "secret.com") {
		t.Fatal("Bcc leaked")
	}
}

func TestParseAttachments(t *testing.T) {
	if ParseAttachments("") != nil {
		t.Fatal("empty should be nil")
	}
	if ParseAttachments("not json") != nil {
		t.Fatal("bad json should be nil")
	}
	if a := ParseAttachments(`[{"name":"x","type":"text/plain","data":"eA=="}]`); len(a) != 1 || a[0].Name != "x" {
		t.Fatalf("got %v", a)
	}
}
