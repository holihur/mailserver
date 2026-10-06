package imap

// IMAP4rev1 子集（:143，明文 LOGIN；配证书则 STARTTLS + :993 隐式 TLS）。
// 给手机自带邮箱 / Outlook / Thunderbird 同步用（POP3 只能取，IMAP 能同步已读/星标/删）。
// 文件夹映射：INBOX/Sent/Drafts/Trash <-> inbox/sent/draft/trash；UID 取 Mail.ID（单调唯一）。
// 低内存：SELECT 只快照元数据（上限 500），正文按需单封加载；单封 2MB 上限。

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"mailserver/internal/message"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

const maxSnap = 500

type item struct {
	id            uint32
	date          time.Time
	from, to      string
	cc            string
	subject, body string
	attachments   string
	read, starred bool
	deleted       bool
	size          int // len(raw) 精确值
}

func (it *item) raw(host string) string {
	return string(message.Build(host, &model.Mail{ID: uint(it.id), From: it.from, To: it.to, Cc: it.cc,
		Subject: it.subject, Body: it.body, Attachments: it.attachments, CreatedAt: it.date}))
}

func subj(s string) string {
	if s == "" {
		return "(无主题)"
	}
	return s
}

var mboxes = []struct{ imap, folder string }{
	{"INBOX", "inbox"}, {"Sent", "sent"}, {"Drafts", "draft"}, {"Trash", "trash"},
}

func toFolder(name string) (string, string, bool) {
	name = strings.Trim(name, "\"")
	for _, m := range mboxes {
		if strings.EqualFold(m.imap, name) {
			return m.imap, m.folder, true
		}
	}
	return "", "", false
}

func Serve(addr string, host func() string, db *gorm.DB, tlsConf *tls.Config) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("imap listen fail:", err)
		return
	}
	fmt.Println("imap on", addr, "tls=", tlsConf != nil)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, host, db, tlsConf, false)
	}
}

func ServeTLS(addr string, host func() string, db *gorm.DB, tlsConf *tls.Config) {
	if tlsConf == nil {
		fmt.Println("imaps skipped: no cert")
		return
	}
	ln, err := tls.Listen("tcp", addr, tlsConf)
	if err != nil {
		fmt.Println("imaps listen fail:", err)
		return
	}
	fmt.Println("imaps on", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(c, host, db, tlsConf, true)
	}
}

type session struct {
	conn     net.Conn
	r        *bufio.Reader
	w        *bufio.Writer
	db       *gorm.DB
	host     func() string
	tlsConf  *tls.Config
	tls      bool
	user     *model.User
	mbox     string // IMAP 名
	folder   string // db folder
	items    []item
	readonly bool
}

func (s *session) untagged(f string, a ...any) {
	s.w.WriteString("* " + fmt.Sprintf(f, a...) + "\r\n")
}

func handle(conn net.Conn, host func() string, db *gorm.DB, tlsConf *tls.Config, encrypted bool) {
	defer conn.Close()
	s := &session{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn),
		db: db, host: host, tlsConf: tlsConf, tls: encrypted}
	s.w.WriteString("* OK Sweetcorn IMAP4rev1 ready\r\n")
	s.w.Flush()
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		tag, cmd, rest := cut2(line)
		if tag == "" || cmd == "" {
			continue
		}
		cmd = strings.ToUpper(cmd)
		ok := func(msg string) { s.w.WriteString(tag + " OK " + msg + "\r\n"); s.w.Flush() }
		no := func(msg string) { s.w.WriteString(tag + " NO " + msg + "\r\n"); s.w.Flush() }
		bad := func(msg string) { s.w.WriteString(tag + " BAD " + msg + "\r\n"); s.w.Flush() }

		switch cmd {
		case "CAPABILITY":
			capa := "IMAP4rev1 LITERAL+ AUTH=PLAIN IDLE MOVE UIDPLUS NAMESPACE ID QUOTA"
			if tlsConf != nil && !s.tls {
				capa += " STARTTLS LOGINDISABLED"
			}
			s.untagged("CAPABILITY %s", capa)
			s.w.Flush()
			ok("capability done")
		case "STARTTLS":
			if tlsConf == nil || s.tls {
				no("not supported")
				continue
			}
			s.w.WriteString(tag + " OK begin TLS\r\n")
			s.w.Flush()
			tc := tls.Server(conn, tlsConf)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			s.conn = conn
			s.r = bufio.NewReader(conn)
			s.w = bufio.NewWriter(conn)
			s.tls = true
			// RFC 2595：STARTTLS 后重置会话状态，防状态穿越
			s.user, s.mbox, s.folder, s.items = nil, "", "", nil
		case "LOGIN":
			if tlsConf != nil && !s.tls {
				no("[PRIVACYREQUIRED] LOGIN disabled, use STARTTLS")
				continue
			}
			u, p, good := parse2quoted(rest)
			if !good || !s.login(u, p) {
				no("login failed")
				continue
			}
			ok("logged in")
		case "AUTHENTICATE":
			if tlsConf != nil && !s.tls {
				no("[PRIVACYREQUIRED] AUTH disabled, use STARTTLS")
				continue
			}
			if !s.doAuthPlain(rest) {
				no("auth failed")
				continue
			}
			ok("authenticated")
		case "LOGOUT":
			s.untagged("BYE bye")
			s.w.WriteString(tag + " OK logout\r\n")
			s.w.Flush()
			return
		case "NOOP":
			s.w.Flush()
			ok("noop done")
		case "LIST", "LSUB":
			pat := rest
			if i := strings.LastIndex(rest, "\""); i >= 0 {
				pat = strings.Trim(rest[i+1:], "\" ")
			} else if f := strings.Fields(rest); len(f) > 0 {
				pat = f[len(f)-1]
			}
			_ = pat
			for _, m := range mboxes {
				s.untagged("%s (\\HasNoChildren) \"/\" %s", cmd, m.imap)
			}
			s.w.Flush()
			ok("list done")
		case "SELECT", "EXAMINE":
			if !s.needAuth(tag) {
				continue
			}
			iname, folder, good := toFolder(firstToken(rest))
			if !good {
				no("no such mailbox")
				continue
			}
			s.mbox, s.folder = iname, folder
			s.readonly = cmd == "EXAMINE"
			s.loadBox()
			s.untagged("FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
			s.untagged("%d EXISTS", len(s.items))
			s.untagged("%d RECENT", len(s.items))
			s.untagged("OK [UIDVALIDITY 1]")
			s.untagged("OK [UIDNEXT %d]", s.uidNext())
			s.w.Flush()
			if s.readonly {
				ok("[READ-ONLY] selected")
			} else {
				ok("[READ-WRITE] selected")
			}
		case "STATUS":
			if !s.needAuth(tag) {
				continue
			}
			mname := firstToken(rest)
			iname, folder, good := toFolder(mname)
			if !good {
				no("no such mailbox")
				continue
			}
			var total int64
			var unseen int64
			var maxID uint32
			s.db.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", s.user.ID, folder).Count(&total)
			s.db.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", s.user.ID, folder).Where(map[string]any{"read": false}).Count(&unseen)
			row := struct{ M uint32 }{0}
			s.db.Model(&model.Mail{}).Select("COALESCE(MAX(id),0) AS m").Where("user_id = ? AND folder = ?", s.user.ID, folder).Scan(&row)
			maxID = row.M
			s.untagged("STATUS %s (MESSAGES %d UNSEEN %d UIDNEXT %d UIDVALIDITY 1)", iname, total, unseen, maxID+1)
			s.w.Flush()
			ok("status done")
		case "FETCH":
			if !s.needSelected(tag) {
				continue
			}
			set, atts := splitFetch(rest)
			seqs := expandSeq(set, len(s.items))
			s.fetchAtts(seqs, atts)
			s.w.Flush()
			ok("fetch done")
		case "UID":
			if !s.needSelected(tag) {
				continue
			}
			sub, _, rest2 := cut2(rest)
			sub = strings.ToUpper(sub)
			switch sub {
			case "FETCH":
				set, atts := splitFetch(rest2)
				uids := expandUID(set, s.maxUID())
				s.fetchAtts(s.uidsToSeqs(uids), atts)
				s.w.Flush()
				ok("fetch done")
			case "STORE":
				if s.storeUID(rest2) {
					s.w.Flush()
					ok("store done")
				} else {
					bad("store failed")
				}
			case "SEARCH":
				uids := s.search(rest2, true)
				s.untagged("SEARCH%s", uidsStr(uids, true))
				s.w.Flush()
				ok("search done")
			case "COPY":
				if src, dst, good := s.copySeq(rest2, true); good {
					ok(fmt.Sprintf("[COPYUID 1 %s %s] copy done", uidsStr(src, true), uidsStr(dst, true)))
				} else {
					no("copy failed")
				}
			case "MOVE":
				if src, dst, good := s.moveSeq(rest2, true); good {
					ok(fmt.Sprintf("[COPYUID 1 %s %s] move done", uidsStr(src, true), uidsStr(dst, true)))
				} else {
					no("move failed")
				}
			default:
				bad("unsupported UID command")
			}
		case "STORE":
			if !s.needSelected(tag) {
				continue
			}
			if s.storeSeq(rest) {
				s.w.Flush()
				ok("store done")
			} else {
				bad("store failed")
			}
		case "SEARCH":
			if !s.needSelected(tag) {
				continue
			}
			seqs := s.search(rest, false)
			s.untagged("SEARCH%s", uidsStr(seqs, false))
			s.w.Flush()
			ok("search done")
		case "COPY":
			if !s.needSelected(tag) {
				continue
			}
			if src, dst, good := s.copySeq(rest, false); good {
				ok(fmt.Sprintf("[COPYUID 1 %s %s] copy done", uidsStr(src, true), uidsStr(dst, true)))
			} else {
				no("copy failed")
			}
		case "MOVE":
			if !s.needSelected(tag) {
				continue
			}
			if src, dst, good := s.moveSeq(rest, false); good {
				ok(fmt.Sprintf("[COPYUID 1 %s %s] move done", uidsStr(src, true), uidsStr(dst, true)))
			} else {
				no("move failed")
			}
		case "EXPUNGE":
			if !s.needSelected(tag) {
				continue
			}
			s.expunge(true)
			s.w.Flush()
			ok("expunged")
		case "CLOSE":
			if !s.needSelected(tag) {
				continue
			}
			s.expunge(false)
			s.items = nil
			s.mbox = ""
			s.w.Flush()
			ok("closed")
		case "CHECK":
			ok("checked")
		case "APPEND":
			if !s.needAuth(tag) {
				continue
			}
			if uid, good := s.doAppend(rest); good {
				ok(fmt.Sprintf("[APPENDUID 1 %d] appended", uid))
			} else {
				no("append failed")
			}
		case "NAMESPACE":
			s.untagged(`NAMESPACE (("" "/")) NIL NIL`)
			s.w.Flush()
			ok("namespace done")
		case "ID":
			s.untagged(`ID ("name" "Sweetcorn" "version" "1.0")`)
			s.w.Flush()
			ok("id done")
		case "GETQUOTA":
			if !s.needAuth(tag) {
				continue
			}
			s.quota()
			ok("quota done")
		case "GETQUOTAROOT":
			if !s.needAuth(tag) {
				continue
			}
			s.untagged("QUOTAROOT %s \"\"", firstToken(rest))
			s.quota()
			ok("quota done")
		case "IDLE":
			s.w.WriteString("+ idling\r\n")
			s.w.Flush()
			s.idle()
			ok("idle done")
		case "CREATE", "DELETE", "RENAME", "SUBSCRIBE", "UNSUBSCRIBE":
			ok("ok")
		default:
			bad("unknown command")
		}
	}
}
