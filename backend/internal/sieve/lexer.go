package sieve

import (
	"fmt"
	"strings"
)

// ---------- 词法 ----------

type tok struct{ kind, val string }

func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.'
}

func lex(s string) ([]tok, error) {
	var out []tok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '#':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '"':
			i++
			var b strings.Builder
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
					b.WriteByte(s[i])
					i++
					continue
				}
				b.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("字符串未闭合")
			}
			i++
			out = append(out, tok{"str", b.String()})
		case c == '{' || c == '}' || c == '(' || c == ')' || c == ',' || c == ';' || c == '[' || c == ']':
			out = append(out, tok{"punct", string(c)})
			i++
		case c == ':':
			j := i + 1
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			out = append(out, tok{"tag", strings.ToLower(s[i:j])})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			out = append(out, tok{"num", s[i:j]})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			out = append(out, tok{"ident", s[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("非法字符: %q", string(c))
		}
	}
	return out, nil
}
