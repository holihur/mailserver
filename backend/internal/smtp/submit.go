package smtp

// SMTP 提交（submission，默认 :587）：给 Foxmail/Outlook/手机客户端发信。
// 25 被封不影响它——它是“收客户端的信存库”，外发由 queue 经中继 587 投递。
// AUTH PLAIN/LOGIN（账密 = 注册邮箱+密码）；MAIL FROM 必须等于登录邮箱（防冒发）。
// TLS：配了证书则支持 STARTTLS（明文下拒绝 AUTH）；没配则允许明文（仅内网/可信网用）。

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/authguard"
	"mailserver/internal/contacts"
	"mailserver/internal/htmlsanitize"
	"mailserver/internal/message"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

func ServeSubmit(addr string, host func() string, db *gorm.DB, tlsConf *tls.Config, maxBytes int64) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("submit listen fail:", err)
		return
	}
	fmt.Println("smtp submit on", addr, "tls=", tlsConf != nil, "max", maxBytes>>20, "MB")
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleSubmit(c, host, db, tlsConf, false, maxBytes)
	}
}

// 465 隐式 TLS：连接即握手
func ServeSubmitTLS(addr string, host func() string, db *gorm.DB, tlsConf *tls.Config, maxBytes int64) {
	if tlsConf == nil {
		fmt.Println("submit-tls skipped: no cert")
		return
	}
	ln, err := tls.Listen("tcp", addr, tlsConf)
	if err != nil {
		fmt.Println("submit-tls listen fail:", err)
		return
	}
	fmt.Println("smtp submit-tls on", addr, "max", maxBytes>>20, "MB")
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleSubmit(c, host, db, tlsConf, true, maxBytes)
	}
}

type submitter struct {
	r      *bufio.Reader
	w      *bufio.Writer
	db     *gorm.DB
	host   func() string
	remote string
	tls    bool // 已加密
	user   *model.User
	from   string
	rcpts  []string
}

func (s *submitter) reply(msg string) { s.w.WriteString(msg + "\r\n"); s.w.Flush() }

func handleSubmit(conn net.Conn, host func() string, db *gorm.DB, tlsConf *tls.Config, encrypted bool, maxBytes int64) {
	if maxBytes <= 0 {
		maxBytes = 25 << 20
	}
	defer conn.Close()
	s := &submitter{r: bufio.NewReader(conn), w: bufio.NewWriter(conn), db: db, host: host, remote: conn.RemoteAddr().String(), tls: encrypted}
	s.reply("220 " + host() + " ESMTP Sweetcorn")
	var data strings.Builder
	inData, overLimit := false, false

	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if overLimit {
				// 超限：丢弃剩余内容直至结束符，再回 552，保持协议同步
				if line == "." {
					inData, overLimit = false, false
					data.Reset()
					s.reply("552 5.3.4 message too large")
				}
				continue
			}
			if line == "." {
				inData = false
				s.queueMail(data.String())
				data.Reset()
				continue
			}
			if strings.HasPrefix(line, ".") {
				line = line[1:] // 透明传输解义
			}
			data.WriteString(line + "\n")
			if int64(data.Len()) > maxBytes {
				overLimit = true
				data.Reset()
			}
			continue
		}
		cmd, arg := splitCmd(line)
		switch cmd {
		case "EHLO", "HELO":
			s.reply("250-" + host() + " Hello")
			if tlsConf != nil && !s.tls {
				s.reply("250-STARTTLS")
			}
			s.reply("250 AUTH PLAIN LOGIN")
		case "STARTTLS":
			if tlsConf == nil || s.tls {
				s.reply("502 not supported")
				continue
			}
			s.reply("220 ready")
			tc := tls.Server(conn, tlsConf)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			s.r = bufio.NewReader(conn)
			s.w = bufio.NewWriter(conn)
			s.tls = true
			// RFC: TLS 后重置会话状态
			s.user, s.from, s.rcpts = nil, "", nil
		case "AUTH":
			if !s.tls && tlsConf != nil {
				s.reply("530 Must issue STARTTLS first")
				continue
			}
			if !s.doAuth(arg) {
				s.reply("535 auth failed")
				continue
			}
			s.reply("235 OK authenticated")
		case "MAIL":
			if s.user == nil {
				s.reply("530 auth required")
				continue
			}
			from := extractAddr(arg)
			if !strings.EqualFold(from, s.user.Email) {
				s.reply("553 from must match login user")
				continue
			}
			s.from = from
			s.rcpts = nil
			s.reply("250 OK")
		case "RCPT":
			if s.from == "" {
				s.reply("503 need MAIL first")
				continue
			}
			if len(s.rcpts) >= 20 {
				s.reply("452 too many recipients")
				continue
			}
			s.rcpts = append(s.rcpts, extractAddr(arg))
			s.reply("250 OK")
		case "DATA":
			if s.from == "" || len(s.rcpts) == 0 {
				s.reply("503 need RCPT first")
				continue
			}
			s.reply("354 End with .")
			inData = true
		case "RSET":
			s.from, s.rcpts = "", nil
			data.Reset()
			s.reply("250 OK")
		case "NOOP":
			s.reply("250 OK")
		case "QUIT":
			s.reply("221 Bye")
			return
		default:
			s.reply("502 unimplemented")
		}
	}
}

func splitCmd(line string) (string, string) {
	if i := strings.IndexByte(line, ' '); i >= 0 {
		return strings.ToUpper(line[:i]), strings.TrimSpace(line[i+1:])
	}
	return strings.ToUpper(line), ""
}

func (s *submitter) doAuth(arg string) bool {
	parts := strings.Fields(arg)
	mech := ""
	init := ""
	if len(parts) > 0 {
		mech = strings.ToUpper(parts[0])
	}
	if len(parts) > 1 {
		init = parts[1]
	}
	switch mech {
	case "PLAIN":
		resp := init
		if resp == "" || resp == "=" {
			s.reply("334 ")
			l, err := s.r.ReadString('\n')
			if err != nil {
				return false
			}
			resp = strings.TrimRight(l, "\r\n")
		}
		b, err := base64.StdEncoding.DecodeString(resp)
		if err != nil {
			return false
		}
		p := strings.SplitN(string(b), "\x00", 3)
		if len(p) != 3 {
			return false
		}
		return s.checkUser(p[1], p[2])
	case "LOGIN":
		s.reply("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
		lu, err := s.r.ReadString('\n')
		if err != nil {
			return false
		}
		s.reply("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
		lp, err := s.r.ReadString('\n')
		if err != nil {
			return false
		}
		ub, err1 := base64.StdEncoding.DecodeString(strings.TrimRight(lu, "\r\n"))
		pb, err2 := base64.StdEncoding.DecodeString(strings.TrimRight(lp, "\r\n"))
		if err1 != nil || err2 != nil {
			return false
		}
		return s.checkUser(string(ub), string(pb))
	}
	return false
}

func (s *submitter) checkUser(email, pass string) bool {
	ip := auth.HostOf(s.remote)
	if authguard.Default.Blocked(ip) {
		log.Printf("auth blocked proto=smtp-submit ip=%s user=%s", ip, email)
		return false
	}
	u, err := auth.AuthenticateMail(s.db, email, pass, ip, auth.ScopeSMTP)
	if err != nil {
		authguard.Default.Fail(ip)
		log.Printf("auth failure proto=smtp-submit ip=%s user=%s err=%v", ip, email, err)
		time.Sleep(500 * time.Millisecond)
		return false
	}
	authguard.Default.Reset(ip)
	s.user = u
	return true
}

// 每个收件人存一封 sent（队列按单 To 投递），本地用户顺手投一份 inbox
func (s *submitter) queueMail(raw string) {
	subject, body, htmlBody, atts := message.ParseInbound(raw)
	for _, to := range s.rcpts {
		m := model.Mail{UserID: s.user.ID, From: s.user.Email, To: to, Subject: subject, Body: body, BodyHTML: htmlsanitize.Sanitize(htmlBody), Attachments: atts, Folder: "sent", Read: true, Status: "queued"}
		s.db.Create(&m)
		contacts.Collect(s.db, s.user.ID, s.user.Email, to)
	}
	s.reply(fmt.Sprintf("250 OK queued for %d rcpt(s)", len(s.rcpts)))
}
