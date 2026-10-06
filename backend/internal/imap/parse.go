package imap

import (
	"fmt"
	"strconv"
	"strings"
)

func cut2(line string) (string, string, string) {
	f := strings.IndexByte(line, ' ')
	if f < 0 {
		return line, "", ""
	}
	rest := strings.TrimLeft(line[f+1:], " ")
	i := strings.IndexByte(rest, ' ')
	if i < 0 {
		return line[:f], rest, ""
	}
	return line[:f], rest[:i], strings.TrimLeft(rest[i+1:], " ")
}

func firstToken(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return strings.Trim(f[0], "\"")
}

// LOGIN 的两个参数（支持引号）
func parse2quoted(s string) (string, string, bool) {
	var args []string
	for len(s) > 0 {
		s = strings.TrimLeft(s, " ")
		if s == "" {
			break
		}
		if s[0] == '"' {
			i := 1
			var b strings.Builder
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == '"' {
					break
				}
				b.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return "", "", false
			}
			args = append(args, b.String())
			s = s[i+1:]
		} else {
			i := strings.IndexByte(s, ' ')
			if i < 0 {
				args = append(args, s)
				break
			}
			args = append(args, s[:i])
			s = s[i+1:]
		}
	}
	if len(args) != 2 {
		return "", "", false
	}
	return args[0], args[1], true
}

func splitHeadBody(raw string) (string, string) {
	if i := strings.Index(raw, "\r\n\r\n"); i >= 0 {
		return raw[:i], raw[i+4:]
	}
	return raw, ""
}

func filterHead(head string, set map[string]bool, not bool) string {
	var out []string
	for _, l := range strings.Split(head, "\r\n") {
		name := l
		if i := strings.IndexByte(l, ':'); i >= 0 {
			name = l[:i]
		}
		_, has := set[strings.ToUpper(name)]
		if has != not {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\r\n")
}

func applyPartial(chunk, partial string) string {
	if partial == "" {
		return chunk
	}
	start, length := 0, len(chunk)
	if p := strings.SplitN(partial, ".", 2); len(p) == 2 {
		start, _ = strconv.Atoi(p[0])
		length, _ = strconv.Atoi(p[1])
	} else {
		start, _ = strconv.Atoi(partial)
	}
	if start < 0 {
		start = 0
	}
	if start >= len(chunk) {
		return ""
	}
	end := start + length
	if end > len(chunk) {
		end = len(chunk)
	}
	return chunk[start:end]
}

func qstr(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

func addrList(email string) string {
	email = strings.TrimSpace(email)
	p := strings.SplitN(email, "@", 2)
	if len(p) != 2 || p[0] == "" || p[1] == "" {
		return "((\"\" NIL NIL NIL))"
	}
	return fmt.Sprintf("((%s NIL %s %s))", qstr(email), qstr(p[0]), qstr(p[1]))
}

func splitRaw(raw string) (subject, body string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.SplitN(raw, "\n\n", 2)
	head, b := "", raw
	if len(parts) == 2 {
		head, b = parts[0], parts[1]
	}
	for _, l := range strings.Split(head, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimLeft(l, " ")), "subject:") {
			subject = strings.TrimSpace(l[strings.Index(l, ":")+1:])
			break
		}
	}
	if len(b) > 20000 {
		b = b[:20000]
	}
	return subject, b
}

func headField(raw, name string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	for _, l := range strings.Split(raw, "\n") {
		if l == "" {
			break
		}
		if i := strings.IndexByte(l, ':'); i > 0 && strings.EqualFold(strings.TrimSpace(l[:i]), name) {
			v := strings.TrimSpace(l[i+1:])
			if a, b := strings.Index(v, "<"), strings.Index(v, ">"); a >= 0 && b > a {
				return v[a+1 : b]
			}
			return v
		}
	}
	return ""
}
