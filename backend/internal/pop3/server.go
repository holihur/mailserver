package pop3

// POP3 取信（默认 :110，明文 USER/PASS；配证书则 :995 隐式 TLS + STLS 可用）。
// 给 Foxmail/Outlook/手机自带邮箱用。低内存：登录时快照 inbox（上限 500 封），逐行应答。
// DELE 在 QUIT 时生效，映射为移入 trash（与 web 端一致，可恢复）。

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const maxBox = 500

type msg struct {
	id      uint
	uid     string
	size    int
	raw     string // RFC5322 全文
	deleted bool
}

func Serve(addr, host string, db *gorm.DB, tlsConf *tls.Config) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("pop3 listen fail:", err)
		return
	}
	fmt.Println("pop3 on", addr, "tls=", tlsConf != nil)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, host, db, tlsConf, false)
	}
}

func ServeTLS(addr, host string, db *gorm.DB, tlsConf *tls.Config) {
	if tlsConf == nil {
		fmt.Println("pop3s skipped: no cert")
		return
	}
	ln, err := tls.Listen("tcp", addr, tlsConf)
	if err != nil {
		fmt.Println("pop3s listen fail:", err)
		return
	}
	fmt.Println("pop3s on", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, host, db, tlsConf, true)
	}
}

type session struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
	db   *gorm.DB
	host string
	tls  bool
	user *model.User
	name string
	box  []msg
}

func (s *session) ok(format string, a ...any) {
	s.w.WriteString("+OK " + fmt.Sprintf(format, a...) + "\r\n")
	s.w.Flush()
}
func (s *session) err(msg string) {
	s.w.WriteString("-ERR " + msg + "\r\n")
	s.w.Flush()
}

func handle(conn net.Conn, host string, db *gorm.DB, tlsConf *tls.Config, encrypted bool) {
	defer conn.Close()
	s := &session{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn), db: db, host: host, tls: encrypted}
	s.ok("mailserver POP3 ready")
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd, arg := line, ""
		if i := strings.IndexByte(line, ' '); i >= 0 {
			cmd, arg = strings.ToUpper(line[:i]), strings.TrimSpace(line[i+1:])
		} else {
			cmd = strings.ToUpper(line)
		}
		switch cmd {
		case "CAPA":
			s.ok("capability list follows")
			s.w.WriteString("USER\r\nPIPELINING\r\nUIDL\r\nTOP\r\n")
			if tlsConf != nil && !s.tls {
				s.w.WriteString("STLS\r\n")
			}
			s.w.WriteString(".\r\n")
			s.w.Flush()
		case "STLS":
			if tlsConf == nil || s.tls || s.user != nil {
				s.err("not supported")
				continue
			}
			s.ok("begin TLS")
			tc := tls.Server(conn, tlsConf)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			s.conn = conn
			s.r = bufio.NewReader(conn)
			s.w = bufio.NewWriter(conn)
			s.tls = true
		case "USER":
			if s.user != nil {
				s.err("already authed")
				continue
			}
			s.name = strings.TrimSpace(arg)
			if s.name == "" {
				s.err("need username")
				continue
			}
			s.ok("send PASS")
		case "PASS":
			if s.name == "" || s.user != nil {
				s.err("need USER first")
				continue
			}
			var u model.User
			if err := s.db.Where("email = ?", s.name).First(&u).Error; err != nil {
				s.err("auth failed")
				continue
			}
			if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(arg)) != nil {
				s.err("auth failed")
				continue
			}
			if u.Disabled {
				s.err("account disabled")
				continue
			}
			s.user = &u
			s.loadBox()
			s.ok("mailbox locked, %d message(s)", len(s.box))
		case "STAT":
			if !s.needAuth() {
				continue
			}
			n, size := 0, 0
			for _, m := range s.box {
				if !m.deleted {
					n++
					size += m.size
				}
			}
			s.ok("%d %d", n, size)
		case "LIST":
			if !s.needAuth() {
				continue
			}
			if arg == "" {
				s.ok("scan listing follows")
				for i, m := range s.box {
					if !m.deleted {
						s.w.WriteString(fmt.Sprintf("%d %d\r\n", i+1, m.size))
					}
				}
				s.w.WriteString(".\r\n")
				s.w.Flush()
			} else {
				m, good := s.pick(arg)
				if !good {
					s.err("no such message")
					continue
				}
				s.ok("%s %d", arg, m.size)
			}
		case "UIDL":
			if !s.needAuth() {
				continue
			}
			if arg == "" {
				s.ok("uid listing follows")
				for i, m := range s.box {
					if !m.deleted {
						s.w.WriteString(fmt.Sprintf("%d %s\r\n", i+1, m.uid))
					}
				}
				s.w.WriteString(".\r\n")
				s.w.Flush()
			} else {
				m, good := s.pick(arg)
				if !good {
					s.err("no such message")
					continue
				}
				s.ok("%s %s", arg, m.uid)
			}
		case "RETR":
			if !s.needAuth() {
				continue
			}
			m, good := s.pick(arg)
			if !good {
				s.err("no such message")
				continue
			}
			if len(m.raw) > 2<<20 {
				s.err("message too large")
				continue
			}
			s.ok("%d octets", m.size)
			s.writeDot(m.raw)
		case "TOP":
			if !s.needAuth() {
				continue
			}
			f := strings.Fields(arg)
			if len(f) != 2 {
				s.err("usage: TOP msg n")
				continue
			}
			m, good := s.pick(f[0])
			if !good {
				s.err("no such message")
				continue
			}
			n, _ := strconv.Atoi(f[1])
			s.ok("top follows")
			s.writeDot(topLines(m.raw, n))
		case "DELE":
			if !s.needAuth() {
				continue
			}
			i, good := s.idx(arg)
			if !good {
				s.err("no such message")
				continue
			}
			s.box[i].deleted = true
			s.ok("marked for deletion")
		case "RSET":
			if !s.needAuth() {
				continue
			}
			for i := range s.box {
				s.box[i].deleted = false
			}
			s.ok("reset")
		case "NOOP":
			s.ok("")
		case "QUIT":
			if s.user != nil {
				for _, m := range s.box {
					if m.deleted {
						s.db.Model(&model.Mail{}).Where("id = ? AND user_id = ?", m.id, s.user.ID).Update("folder", "trash")
					}
				}
			}
			s.ok("bye")
			return
		default:
			s.err("unknown command")
		}
	}
}

func (s *session) needAuth() bool {
	if s.user == nil {
		s.err("auth first")
		return false
	}
	return true
}

func (s *session) idx(arg string) (int, bool) {
	f := strings.Fields(arg)
	if len(f) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(f[0])
	if err != nil || n < 1 || n > len(s.box) || s.box[n-1].deleted {
		return 0, false
	}
	return n - 1, true
}

func (s *session) pick(arg string) (msg, bool) {
	i, ok := s.idx(arg)
	if !ok {
		return msg{}, false
	}
	return s.box[i], true
}

func (s *session) loadBox() {
	var mails []model.Mail
	s.db.Where("user_id = ? AND folder = ?", s.user.ID, "inbox").Order("id DESC").Limit(maxBox).Find(&mails)
	s.box = s.box[:0]
	for _, m := range mails {
		raw := buildRaw(s.host, &m)
		s.box = append(s.box, msg{id: m.ID, uid: fmt.Sprintf("%d-%d", m.ID, m.CreatedAt.Unix()), size: len(raw), raw: raw})
	}
}

func buildRaw(host string, m *model.Mail) string {
	subj := m.Subject
	if subj == "" {
		subj = "(无主题)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d@%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n",
		m.From, m.To, subj, m.CreatedAt.Format(time.RFC1123Z), m.ID, host)
	b.WriteString(strings.ReplaceAll(m.Body, "\n", "\r\n"))
	return b.String()
}

// dot-stuffing 后写出
func (s *session) writeDot(raw string) {
	r := bufio.NewScanner(strings.NewReader(raw))
	r.Buffer(make([]byte, 64*1024), 1<<20)
	for r.Scan() {
		l := r.Text()
		if strings.HasPrefix(l, ".") {
			s.w.WriteString(".")
		}
		s.w.WriteString(l + "\r\n")
	}
	s.w.WriteString(".\r\n")
	s.w.Flush()
}

func topLines(raw string, n int) string {
	if n < 0 {
		n = 0
	}
	idx := strings.Index(raw, "\r\n\r\n")
	head, body := raw, ""
	if idx >= 0 {
		head, body = raw[:idx], raw[idx+4:]
	}
	lines := strings.Split(body, "\r\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return head + "\r\n\r\n" + strings.Join(lines, "\r\n")
}
