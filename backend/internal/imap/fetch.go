package imap

import (
	"fmt"
	"strconv"
	"strings"

	"mailserver/internal/message"
	"mailserver/internal/model"
)

// ---- FETCH ----

func splitFetch(rest string) (string, string) {
	f := strings.Fields(rest)
	if len(f) == 0 {
		return "", ""
	}
	set := f[0]
	atts := strings.TrimSpace(rest[len(set):])
	return set, atts
}

// 顶层切分 att 列表（BODY[...] 内的空格/括号不切）
func splitAtts(s string) []string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
	}
	var out []string
	var cur strings.Builder
	depth, inQ := 0, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQ = !inQ
			cur.WriteByte(c)
			continue
		}
		if !inQ {
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
			}
			if c == ' ' && depth == 0 {
				if cur.Len() > 0 {
					out = append(out, cur.String())
					cur.Reset()
				}
				continue
			}
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func expandSeq(set string, n int) []int {
	var out []int
	for _, part := range strings.Split(set, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, ":") {
			a, b, _ := strings.Cut(part, ":")
			lo := seqNum(a, n)
			hi := seqNum(b, n)
			if lo > hi {
				lo, hi = hi, lo
			}
			for i := lo; i <= hi && i <= n; i++ {
				if i >= 1 {
					out = append(out, i)
				}
			}
		} else {
			if v := seqNum(part, n); v >= 1 && v <= n {
				out = append(out, v)
			}
		}
	}
	return out
}

func seqNum(s string, n int) int {
	s = strings.TrimSpace(s)
	if s == "*" {
		return n
	}
	v, _ := strconv.Atoi(s)
	return v
}

func expandUID(set string, max uint32) []uint32 {
	var out []uint32
	for _, part := range strings.Split(set, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, ":") {
			a, b, _ := strings.Cut(part, ":")
			lo := uidNum(a, max)
			hi := uidNum(b, max)
			if lo > hi {
				lo, hi = hi, lo
			}
			for i := lo; i <= hi; i++ {
				out = append(out, i)
			}
		} else {
			out = append(out, uidNum(part, max))
		}
	}
	return out
}

func uidNum(s string, max uint32) uint32 {
	s = strings.TrimSpace(s)
	if s == "*" {
		return max
	}
	v, _ := strconv.ParseUint(s, 10, 32)
	return uint32(v)
}

func (s *session) fetchAtts(seqs []int, atts string) {
	u := strings.ToUpper(strings.TrimSpace(atts))
	var list []string
	switch u {
	case "ALL":
		list = []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE"}
	case "FAST":
		list = []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE"}
	case "FULL":
		list = []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE", "BODY[]"}
	default:
		list = splitAtts(atts)
	}
	for _, seq := range seqs {
		if seq < 1 || seq > len(s.items) {
			continue
		}
		it := &s.items[seq-1]
		if it.deleted {
			continue
		}
		var parts []string
		var litName, lit string // 延迟字面量：BODY[] {n}\r\n + 内容
		markSeen := false
		for _, a := range list {
			au := strings.ToUpper(a)
			switch {
			case au == "UID":
				parts = append(parts, fmt.Sprintf("UID %d", it.id))
			case au == "FLAGS":
				parts = append(parts, fmt.Sprintf("FLAGS (%s)", flagsOf(it)))
			case au == "INTERNALDATE":
				parts = append(parts, fmt.Sprintf("INTERNALDATE \"%s\"", it.date.Format("02-Jan-2006 15:04:05 -0700")))
			case au == "RFC822.SIZE":
				parts = append(parts, fmt.Sprintf("RFC822.SIZE %d", it.size))
			case au == "ENVELOPE":
				parts = append(parts, "ENVELOPE "+envelope(it, s.host()))
			case au == "BODYSTRUCTURE":
				parts = append(parts, "BODYSTRUCTURE "+bodyStruct(it))
			case strings.HasPrefix(au, "BODY[") || au == "BODY":
				peek := strings.Contains(au, "PEEK")
				sec, partial := parseBodySec(a)
				raw := it.raw(s.host())
				chunk := bodySection(raw, sec)
				chunk = applyPartial(chunk, partial)
				litName = a
				lit = chunk
				if !peek {
					markSeen = true
				}
			case au == "RFC822":
				litName = a
				lit = it.raw(s.host())
				markSeen = true
			case au == "RFC822.HEADER":
				litName = a
				lit, _ = splitHeadBody(it.raw(s.host()))
			case au == "RFC822.TEXT":
				litName = a
				_, lit = splitHeadBody(it.raw(s.host()))
			}
		}
		if markSeen && !it.read && !s.readonly {
			it.read = true
			s.db.Model(&model.Mail{}).Where("id = ?", it.id).Update("read", true)
			// 刷新 FLAGS 显示
			for i, p := range parts {
				if strings.HasPrefix(p, "FLAGS ") {
					parts[i] = fmt.Sprintf("FLAGS (%s)", flagsOf(it))
				}
			}
		}
		if litName != "" {
			s.untagged("%d FETCH (%s %s {%d})", seq, strings.Join(parts, " "), litName, len(lit))
			s.w.Flush()
			s.w.WriteString(lit + "\r\n")
			s.w.WriteString(")\r\n")
		} else {
			s.untagged("%d FETCH (%s)", seq, strings.Join(parts, " "))
		}
		s.w.Flush()
	}
}

// BODY[...] 解析：返回 section 描述 + partial
func parseBodySec(a string) (string, string) {
	u := strings.ToUpper(a)
	i := strings.Index(u, "[")
	j := strings.LastIndex(u, "]")
	if i < 0 || j < 0 {
		return "", ""
	}
	sec := a[i+1 : j]
	partial := ""
	if len(u) > j+1 && u[j+1] == '<' {
		partial = u[j+2 : len(u)-1]
	}
	return sec, partial
}

func bodySection(raw, sec string) string {
	head, body := splitHeadBody(raw)
	su := strings.ToUpper(strings.TrimSpace(sec))
	if su == "" {
		return raw
	}
	if su == "HEADER" {
		return head + "\r\n"
	}
	if su == "TEXT" {
		return body
	}
	if strings.HasPrefix(su, "HEADER.FIELDS.NOT") {
		inner := sec[strings.Index(sec, "(")+1 : strings.LastIndex(sec, ")")]
		skip := map[string]bool{}
		for _, f := range strings.Fields(inner) {
			skip[strings.ToUpper(strings.Trim(f, "\""))] = true
		}
		return filterHead(head, skip, true) + "\r\n"
	}
	if strings.HasPrefix(su, "HEADER.FIELDS") {
		inner := sec[strings.Index(sec, "(")+1 : strings.LastIndex(sec, ")")]
		keep := map[string]bool{}
		for _, f := range strings.Fields(inner) {
			keep[strings.ToUpper(strings.Trim(f, "\""))] = true
		}
		return filterHead(head, keep, false) + "\r\n"
	}
	return raw
}

func envelope(it *item, host string) string {
	date := it.date.Format("Mon, 2 Jan 2006 15:04:05 -0700")
	return fmt.Sprintf("(%s %s %s NIL NIL %s %s NIL NIL %s)",
		qstr(date), qstr(subj(it.subject)), addrList(it.from), addrList(it.to), addrList(it.cc),
		qstr(fmt.Sprintf("<%d@%s>", it.id, host)))
}

func bodyStruct(it *item) string {
	bc := strings.ReplaceAll(it.body, "\n", "\r\n")
	lines := 1 + strings.Count(it.body, "\n")
	atts := message.ParseAttachments(it.attachments)
	if len(atts) == 0 {
		return fmt.Sprintf("(\"TEXT\" \"PLAIN\" (\"CHARSET\" \"UTF-8\") NIL NIL \"8BIT\" %d %d)", len(bc), lines)
	}
	parts := []string{
		fmt.Sprintf("(\"TEXT\" \"PLAIN\" (\"CHARSET\" \"UTF-8\") NIL NIL \"8BIT\" %d %d)", len(bc), lines),
	}
	for _, a := range atts {
		typ, sub := "APPLICATION", "OCTET-STREAM"
		if a.Type != "" {
			if i := strings.Index(a.Type, "/"); i > 0 {
				typ = strings.ToUpper(a.Type[:i])
				sub = strings.ToUpper(a.Type[i+1:])
			} else {
				typ = strings.ToUpper(a.Type)
			}
		}
		parts = append(parts, fmt.Sprintf("(%q %q (\"NAME\" %q) NIL NIL \"BASE64\" %d NIL (\"ATTACHMENT\" (\"FILENAME\" %q)))",
			typ, sub, a.Name, len(a.Data), a.Name))
	}
	return "(" + strings.Join(parts, " ") + " \"MIXED\")"
}
