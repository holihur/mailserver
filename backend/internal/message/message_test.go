package message

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mailserver/internal/model"
)

func sampleMail() *model.Mail {
	return &model.Mail{
		ID: 7, From: "a@example.com", To: "b@x.com, c@y.com", Cc: "d@z.com", Bcc: "e@secret.com",
		Subject: "hi", Body: "line1\nline2", CreatedAt: time.Unix(1700000000, 0),
	}
}

func TestRecipients(t *testing.T) {
	got := Recipients(sampleMail())
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
	hdrs, body := Parts("mail.example.com", sampleMail())
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
	m := sampleMail()
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

func TestParseInboundEncodedSubject(t *testing.T) {
	raw := "From: a@b.c\r\nTo: x@y.z\r\nSubject: =?utf-8?b?5YWl56uZIFNNVFAg6Ieq5rWL?=\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhello\r\n"
	subj, body, _, atts := ParseInbound(raw)
	if subj != "入站 SMTP 自测" {
		t.Fatalf("subject=%q", subj)
	}
	if body != "hello\r\n" || atts != "" {
		t.Fatalf("body=%q atts=%q", body, atts)
	}
}

func TestParseInboundMultipartAttachment(t *testing.T) {
	raw := strings.Join([]string{
		"From: a@b.c",
		"To: x@y.z",
		"Subject: =?utf-8?q?hi?=",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="B"`,
		"",
		"--B",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"正文",
		"--B",
		`Content-Type: application/pdf; name="a.pdf"`,
		`Content-Disposition: attachment; filename="a.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		"aGVsbG8=",
		"--B--",
		"",
	}, "\r\n")
	subj, body, _, atts := ParseInbound(raw)
	if subj != "hi" || body != "正文" {
		t.Fatalf("subj=%q body=%q", subj, body)
	}
	list := ParseAttachments(atts)
	if len(list) != 1 || list[0].Name != "a.pdf" || list[0].Data != "aGVsbG8=" {
		t.Fatalf("atts=%v", list)
	}
}

func TestParseInboundQuotedPrintable(t *testing.T) {
	raw := "Subject: t\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nh=C3=A9llo\r\n"
	_, body, _, _ := ParseInbound(raw)
	if strings.TrimRight(body, "\r\n") != "héllo" {
		t.Fatalf("body=%q", body)
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

func TestParseInboundHTML(t *testing.T) {
	raw := "Subject: hi\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<html><body><p>Hello <b>World</b></p></body></html>"
	subj, body, htmlBody, _ := ParseInbound(raw)
	if subj != "hi" {
		t.Fatalf("subj=%q", subj)
	}
	if !strings.Contains(htmlBody, "<b>World</b>") {
		t.Fatalf("html=%q", htmlBody)
	}
	if !strings.Contains(body, "Hello") || strings.Contains(body, "<") {
		t.Fatalf("body 应为纯文本=%q", body)
	}
}
