package imap

import (
	"encoding/base64"
	"log"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/authguard"
)

func (s *session) needAuth(tag string) bool {
	if s.user == nil {
		s.w.WriteString(tag + " NO please login first\r\n")
		s.w.Flush()
		return false
	}
	return true
}

func (s *session) needSelected(tag string) bool {
	if !s.needAuth(tag) {
		return false
	}
	if s.mbox == "" {
		s.w.WriteString(tag + " NO no mailbox selected\r\n")
		s.w.Flush()
		return false
	}
	return true
}

func (s *session) login(email, pass string) bool {
	ip := auth.HostOf(s.conn.RemoteAddr().String())
	if authguard.Default.Blocked(ip) {
		log.Printf("auth blocked proto=imap ip=%s user=%s", ip, email)
		return false
	}
	u, err := auth.AuthenticateMail(s.db, email, pass, ip, auth.ScopeIMAP)
	if err != nil {
		authguard.Default.Fail(ip)
		log.Printf("auth failure proto=imap ip=%s user=%s err=%v", ip, email, err)
		time.Sleep(500 * time.Millisecond)
		return false
	}
	authguard.Default.Reset(ip)
	s.user = u
	return true
}

// AUTHENTICATE PLAIN [b64]（SASL-IR 兼容）
func (s *session) doAuthPlain(rest string) bool {
	resp := strings.TrimSpace(rest)
	if strings.HasPrefix(strings.ToUpper(resp), "PLAIN") {
		resp = strings.TrimSpace(resp[5:])
	} else if resp != "" {
		return false
	}
	if resp == "" || resp == "*" {
		s.w.WriteString("+\r\n")
		s.w.Flush()
		l, err := s.r.ReadString('\n')
		if err != nil {
			return false
		}
		resp = strings.TrimRight(l, "\r\n")
	}
	if resp == "*" {
		s.w.WriteString("* BAD auth cancelled\r\n")
		s.w.Flush()
		return false
	}
	b, err := b64decode(resp)
	if err != nil {
		return false
	}
	p := strings.SplitN(string(b), "\x00", 3)
	if len(p) != 3 {
		return false
	}
	return s.login(p[1], p[2])
}

// base64（容忍缺 padding）
func b64decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}
