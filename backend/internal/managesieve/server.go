// Package managesieve 实现 ManageSieve（RFC 5804）的一个实用子集，
// 让 Thunderbird 等客户端可以管理服务端 Sieve 脚本。鉴权使用「应用专用密码」(PAT)。
package managesieve

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/authguard"
	"mailserver/internal/model"
	"mailserver/internal/sieve"

	"gorm.io/gorm"
)

func Serve(addr string, db *gorm.DB, tlsConf *tls.Config) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("managesieve listen fail:", err)
		return
	}
	fmt.Println("managesieve on", addr, "tls=", tlsConf != nil)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, db, tlsConf, false)
	}
}

type sess struct {
	conn    net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	db      *gorm.DB
	tlsConf *tls.Config
	tls     bool
	user    *model.User
}

func (s *sess) line(f string, a ...any) {
	s.w.WriteString(fmt.Sprintf(f, a...) + "\r\n")
	s.w.Flush()
}
func (s *sess) ok(msg string)  { s.line("OK %q", msg) }
func (s *sess) no(msg string)  { s.line("NO %q", msg) }
func (s *sess) bye(msg string) { s.line("BYE %q", msg) }

func (s *sess) capability() {
	s.line(`"IMPLEMENTATION" "Sweetcorn ManageSieve"`)
	s.line(`"SIEVE" "%s"`, strings.Join(sieve.Capabilities(), " "))
	if s.tls || s.tlsConf == nil {
		s.line(`"SASL" "PLAIN"`)
	}
	if s.tlsConf != nil && !s.tls {
		s.line(`"STARTTLS"`)
	}
	s.line(`"VERSION" "1.0"`)
	s.ok("Sweetcorn ManageSieve ready")
}

func (s *sess) needAuth() bool {
	if s.user == nil {
		s.no("Please authenticate first")
		return false
	}
	return true
}

func handle(conn net.Conn, db *gorm.DB, tlsConf *tls.Config, encrypted bool) {
	defer conn.Close()
	s := &sess{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn), db: db, tlsConf: tlsConf, tls: encrypted}
	s.capability()
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd, rest := cut(line)
		switch strings.ToUpper(cmd) {
		case "CAPABILITY":
			s.capability()
		case "NOOP":
			s.ok("NOOP completed")
		case "LOGOUT":
			s.ok("Goodbye")
			return
		case "STARTTLS":
			if tlsConf == nil || s.tls {
				s.no("TLS not available")
				continue
			}
			s.ok("Begin TLS negotiation now")
			tc := tls.Server(conn, tlsConf)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			s.conn = conn
			s.r = bufio.NewReader(conn)
			s.w = bufio.NewWriter(conn)
			s.tls = true
			s.user = nil // TLS 后重置已认证状态，防状态穿越
		case "AUTHENTICATE":
			if s.tlsConf != nil && !s.tls {
				s.no("STARTTLS required")
				continue
			}
			s.authenticate(rest)
		case "LISTSCRIPTS":
			if s.needAuth() {
				s.listScripts()
			}
		case "GETSCRIPT":
			if s.needAuth() {
				s.getScript(rest)
			}
		case "PUTSCRIPT":
			if s.needAuth() {
				s.putScript(rest)
			}
		case "SETACTIVE":
			if s.needAuth() {
				s.setActive(rest)
			}
		case "DELETESCRIPT":
			if s.needAuth() {
				s.deleteScript(rest)
			}
		case "CHECKSCRIPT":
			if s.needAuth() {
				s.checkScript(rest)
			}
		case "RENAME":
			if s.needAuth() {
				s.rename(rest)
			}
		case "HAVESPACE":
			s.ok("Have space")
		default:
			s.no("Unknown command")
		}
	}
}

func cut(s string) (string, string) {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// nextArg 解析下一个参数：带引号的字符串，或 {n}/{n+} 字面量。
func (s *sess) nextArg(rest string) (val, remainder string, ok bool) {
	rest = strings.TrimLeft(rest, " ")
	if rest == "" {
		return "", "", false
	}
	if rest[0] == '"' {
		var b strings.Builder
		i := 1
		for i < len(rest) {
			if rest[i] == '\\' && i+1 < len(rest) {
				b.WriteByte(rest[i+1])
				i += 2
				continue
			}
			if rest[i] == '"' {
				return b.String(), rest[i+1:], true
			}
			b.WriteByte(rest[i])
			i++
		}
		return "", "", false
	}
	if rest[0] == '{' {
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", "", false
		}
		numStr := rest[1:end]
		plus := strings.HasSuffix(numStr, "+")
		n, err := strconv.Atoi(strings.TrimSuffix(numStr, "+"))
		if err != nil || n < 0 || n > 1<<20 {
			return "", "", false
		}
		if !plus {
			_, _ = s.r.ReadString('\n') // 消费 CRLF
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(s.r, buf); err != nil {
			return "", "", false
		}
		return string(buf), rest[end+1:], true
	}
	return "", "", false
}

func (s *sess) authenticate(rest string) {
	mech, rest, ok := s.nextArg(rest)
	if !ok || !strings.EqualFold(mech, "PLAIN") {
		s.no("Only PLAIN supported")
		return
	}
	b64, _, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing credentials")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		s.no("Bad credentials")
		return
	}
	p := strings.SplitN(string(raw), "\x00", 3)
	if len(p) != 3 {
		s.no("Bad credentials")
		return
	}
	ip := auth.HostOf(s.conn.RemoteAddr().String())
	if authguard.Default.Blocked(ip) {
		log.Printf("auth blocked proto=managesieve ip=%s", ip)
		s.no("Too many attempts, try later")
		return
	}
	u, err := auth.AuthenticateMail(s.db, p[1], p[2], ip, auth.ScopeSieve)
	if err != nil {
		authguard.Default.Fail(ip)
		log.Printf("auth failure proto=managesieve ip=%s user=%s err=%v", ip, p[1], err)
		time.Sleep(500 * time.Millisecond)
		s.no("Authentication failed")
		return
	}
	authguard.Default.Reset(ip)
	s.user = u
	s.ok("Authentication successful")
}

func (s *sess) listScripts() {
	var ss []model.SieveScript
	s.db.Where("user_id = ?", s.user.ID).Order("name").Find(&ss)
	for _, sc := range ss {
		if sc.Active {
			s.line("%q ACTIVE", sc.Name)
		} else {
			s.line("%q", sc.Name)
		}
	}
	s.ok("Listscripts completed")
}

func (s *sess) getScript(rest string) {
	name, _, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing script name")
		return
	}
	var sc model.SieveScript
	if err := s.db.Where("user_id = ? AND name = ?", s.user.ID, name).First(&sc).Error; err != nil {
		s.no("Script not found")
		return
	}
	s.line("{%d}", len(sc.Script))
	s.w.WriteString(sc.Script)
	s.w.Flush()
	s.ok("Getscript completed")
}

func (s *sess) putScript(rest string) {
	name, rem, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing script name")
		return
	}
	body, _, ok := s.nextArg(rem)
	if !ok {
		s.no("Missing script body")
		return
	}
	if _, err := sieve.Compile(body); err != nil {
		s.no("Syntax error: " + err.Error())
		return
	}
	var sc model.SieveScript
	if err := s.db.Where("user_id = ? AND name = ?", s.user.ID, name).First(&sc).Error; err == nil {
		s.db.Model(&sc).Updates(map[string]any{"script": body, "updated_at": time.Now()})
	} else {
		s.db.Create(&model.SieveScript{UserID: s.user.ID, Name: name, Script: body, UpdatedAt: time.Now()})
	}
	s.ok("Putscript completed")
}

func (s *sess) setActive(rest string) {
	name, _, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing script name")
		return
	}
	s.db.Model(&model.SieveScript{}).Where("user_id = ?", s.user.ID).Update("active", false)
	if name != "" {
		res := s.db.Model(&model.SieveScript{}).Where("user_id = ? AND name = ?", s.user.ID, name).Update("active", true)
		if res.RowsAffected == 0 {
			s.no("Script not found")
			return
		}
	}
	s.ok("Setactive completed")
}

func (s *sess) deleteScript(rest string) {
	name, _, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing script name")
		return
	}
	s.db.Where("user_id = ? AND name = ?", s.user.ID, name).Delete(&model.SieveScript{})
	s.ok("Deletescript completed")
}

func (s *sess) checkScript(rest string) {
	body, _, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing script body")
		return
	}
	if _, err := sieve.Compile(body); err != nil {
		s.no("Syntax error: " + err.Error())
		return
	}
	s.ok("Checkscript completed")
}

func (s *sess) rename(rest string) {
	oldName, rem, ok := s.nextArg(rest)
	if !ok {
		s.no("Missing script name")
		return
	}
	newName, _, ok := s.nextArg(rem)
	if !ok {
		s.no("Missing new name")
		return
	}
	res := s.db.Model(&model.SieveScript{}).Where("user_id = ? AND name = ?", s.user.ID, oldName).Update("name", newName)
	if res.RowsAffected == 0 {
		s.no("Script not found")
		return
	}
	s.ok("Rename completed")
}
