package message

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"

	"golang.org/x/net/html"
)

const (
	maxBody    = 200000
	maxPart    = 8 << 20
	maxAttsLen = 8 << 20
)

// ParseInbound 解析收到的原始邮件：解码 RFC2047 主题、按传输编码解码正文、
// 从 multipart 中提取附件（base64 存 JSON）与 HTML 正文。找不到时退化为原文。
// 返回：主题、纯文本正文、HTML 正文（可能为空）、附件 JSON。
func ParseInbound(raw string) (subject, body, htmlBody, attachments string) {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		s, b, a := fallback(raw)
		return s, b, "", a
	}
	subject = decodeHeader(msg.Header.Get("Subject"))
	ct := msg.Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(ct)
	if strings.HasPrefix(mediaType, "multipart/") {
		atts, text, html := parseMultipart(msg.Body, params["boundary"], 0)
		body = text
		htmlBody = html
		attachments = marshalAtts(atts)
	} else {
		b, _ := io.ReadAll(io.LimitReader(msg.Body, maxPart))
		dec := decodeBytes(b, msg.Header.Get("Content-Transfer-Encoding"))
		if mediaType == "text/html" {
			htmlBody = string(dec)
			body = htmlToText(htmlBody)
		} else {
			body = string(dec)
		}
	}
	if strings.TrimSpace(subject) == "" {
		subject = "(无主题)"
	}
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	if len(htmlBody) > maxBody*2 {
		htmlBody = htmlBody[:maxBody*2]
	}
	return subject, body, htmlBody, attachments
}

func parseMultipart(r io.Reader, boundary string, depth int) ([]Attachment, string, string) {
	if depth > 5 {
		b, _ := io.ReadAll(io.LimitReader(r, maxPart))
		return nil, string(b), ""
	}
	if boundary == "" {
		b, _ := io.ReadAll(io.LimitReader(r, maxPart))
		return nil, string(b), ""
	}
	mr := multipart.NewReader(r, boundary)
	var atts []Attachment
	var text, html string
	total := 0
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		ctype := p.Header.Get("Content-Type")
		mt, params, _ := mime.ParseMediaType(ctype)
		data, _ := io.ReadAll(io.LimitReader(p, maxPart))

		if strings.HasPrefix(mt, "multipart/") {
			subAt, subText, subHTML := parseMultipart(bytes.NewReader(data), params["boundary"], depth+1)
			atts = append(atts, subAt...)
			if text == "" {
				text = subText
			}
			if html == "" {
				html = subHTML
			}
			continue
		}
		name := p.FileName()
		if name != "" || (mt != "" && !strings.HasPrefix(mt, "text/")) {
			raw := decodeBytes(data, p.Header.Get("Content-Transfer-Encoding"))
			if total+len(raw) <= maxAttsLen {
				total += len(raw)
				atts = append(atts, Attachment{Name: name, Type: mt, Data: base64.StdEncoding.EncodeToString(raw), Size: len(raw)})
			}
			continue
		}
		dec := decodeBytes(data, p.Header.Get("Content-Transfer-Encoding"))
		switch mt {
		case "text/html":
			if html == "" {
				html = string(dec)
			}
		default:
			if text == "" {
				text = string(dec)
			}
		}
	}
	if text == "" {
		text = htmlToText(html)
	}
	return atts, text, html
}

func decodeHeader(s string) string {
	if s == "" {
		return ""
	}
	if out, err := new(mime.WordDecoder).DecodeHeader(s); err == nil {
		return out
	}
	return s
}

func decodeBytes(b []byte, cte string) []byte {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		out, _ := io.ReadAll(io.LimitReader(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(b)), maxPart))
		return out
	case "quoted-printable":
		out, _ := io.ReadAll(io.LimitReader(quotedprintable.NewReader(bytes.NewReader(b)), maxPart))
		return out
	}
	return b
}

// htmlToText 提取 HTML 纯文本（用于列表摘要与纯文本回退）。
func htmlToText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3":
				b.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	lines := strings.Split(b.String(), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

func marshalAtts(a []Attachment) string {
	if len(a) == 0 {
		return ""
	}
	b, _ := json.Marshal(a)
	return string(b)
}

// fallback 用于非标准邮件：直接取正文。
func fallback(raw string) (string, string, string) {
	subject := "(无主题)"
	body := raw
	if i := strings.Index(raw, "\n\n"); i >= 0 {
		head, b := raw[:i], raw[i+2:]
		for _, l := range strings.Split(head, "\n") {
			if strings.HasPrefix(strings.ToLower(l), "subject:") {
				subject = decodeHeader(strings.TrimSpace(l[8:]))
			}
		}
		body = b
	}
	if len(body) > maxBody {
		body = body[:maxBody]
	}
	return subject, body, ""
}
