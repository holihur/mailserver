package imap

// IMAP4rev1 子集（:143，明文 LOGIN；配证书则 STARTTLS + :993 隐式 TLS）。
// 给手机自带邮箱 / Outlook / Thunderbird 同步用（POP3 只能取，IMAP 能同步已读/星标/删）。
// 文件夹映射：INBOX/Sent/Drafts/Trash <-> inbox/sent/draft/trash；UID 取 Mail.ID（单调唯一）。
// 低内存：SELECT 只快照元数据（上限 500），正文按需单封加载；单封 2MB 上限。

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const maxSnap = 500

type item struct {
	id             uint32
	date           time.Time
	from, to       string
	subject, body  string
	read, starred  bool
	deleted        bool
	size           int // len(raw) 精确值
}

func (it *item) raw(host string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d@%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n",
		it.from, it.to, subj(it.subject), it.date.Format(time.RFC1123Z), it.id, host)
	b.WriteString(strings.ReplaceAll(it.body, "\n", "\r\n"))
	return b.String()
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

func Serve(addr, host string, db *gorm.DB, tlsConf *tls.Config) {
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

func ServeTLS(addr, host string, db *gorm.DB, tlsConf *tls.Config) {
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
	host     string
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

func handle(conn net.Conn, host string, db *gorm.DB, tlsConf *tls.Config, encrypted bool) {
	defer conn.Close()
	s := &session{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn),
		db: db, host: host, tlsConf: tlsConf, tls: encrypted}
	s.w.WriteString("* OK mailserver IMAP4rev1 ready\r\n")
	s.w.Flush()
	for {
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
			capa := "IMAP4rev1 LITERAL+ AUTH=PLAIN IDLE"
			if tlsConf != nil && !s.tls {
				capa += " STARTTLS"
			}
			s.untagged("CAPABILITY " + capa)
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
		case "LOGIN":
			u, p, good := parse2quoted(rest)
			if !good || !s.login(u, p) {
				no("login failed")
				continue
			}
			ok("logged in")
		case "AUTHENTICATE":
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
			s.db.Model(&model.Mail{}).Where("user_id = ? AND folder = ? AND `read` = ?", s.user.ID, folder, false).Count(&unseen)
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
			sub, rest2 := cut2(rest)
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
				if s.copySeq(rest2, true) {
					ok("copy done")
				} else {
					no("copy failed")
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
			if s.copySeq(rest, false) {
				ok("copy done")
			} else {
				no("copy failed")
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
			if s.doAppend(rest) {
				ok("appended")
			} else {
				no("append failed")
			}
		case "IDLE":
			s.w.WriteString("+ idling\r\n")
			s.w.Flush()
			for {
				l, err := s.r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "DONE" {
					break
				}
			}
			ok("idle done")
		case "CREATE", "DELETE", "RENAME", "SUBSCRIBE", "UNSUBSCRIBE":
			ok("ok")
		default:
			bad("unknown command")
		}
	}
}

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
	var u model.User
	if err := s.db.Where("email = ?", strings.TrimSpace(email)).First(&u).Error; err != nil {
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(pass)) != nil {
		return false
	}
	s.user = &u
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

func (s *session) loadBox() {
	var mails []model.Mail
	s.db.Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Order("id").Limit(maxSnap).Find(&mails)
	s.items = s.items[:0]
	for _, m := range mails {
		it := item{id: uint32(m.ID), date: m.CreatedAt, from: m.From, to: m.To,
			subject: m.Subject, body: m.Body, read: m.Read, starred: m.Starred}
		it.size = len(it.raw(s.host))
		s.items = append(s.items, it)
	}
}

func (s *session) uidNext() uint32 { return s.maxUID() + 1 }
func (s *session) maxUID() uint32 {
	var m uint32
	for _, it := range s.items {
		if it.id > m {
			m = it.id
		}
	}
	if m == 0 && s.user != nil && s.folder != "" {
		row := struct{ M uint32 }{0}
		s.db.Model(&model.Mail{}).Select("COALESCE(MAX(id),0) AS m").
			Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Scan(&row)
		m = row.M
	}
	return m
}

func (s *session) uidsToSeqs(uids []uint32) []int {
	var out []int
	for _, u := range uids {
		for i, it := range s.items {
			if it.id == u && !it.deleted {
				out = append(out, i+1)
				break
			}
		}
	}
	return out
}

func flagsOf(it *item) string {
	var f []string
	if it.read {
		f = append(f, "\\Seen")
	}
	if it.starred {
		f = append(f, "\\Flagged")
	}
	if it.deleted {
		f = append(f, "\\Deleted")
	}
	return strings.Join(f, " ")
}

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
				parts = append(parts, "ENVELOPE "+envelope(it, s.host))
			case au == "BODYSTRUCTURE":
				parts = append(parts, "BODYSTRUCTURE "+bodyStruct(it))
			case strings.HasPrefix(au, "BODY[") || au == "BODY":
				peek := strings.Contains(au, "PEEK")
				sec, partial := parseBodySec(a)
				raw := it.raw(s.host)
				chunk := bodySection(raw, sec)
				chunk = applyPartial(chunk, partial)
				litName = a
				lit = chunk
				if !peek {
					markSeen = true
				}
			case au == "RFC822":
				litName = a
				lit = it.raw(s.host)
				markSeen = true
			case au == "RFC822.HEADER":
				litName = a
				lit, _ = splitHeadBody(it.raw(s.host))
			case au == "RFC822.TEXT":
				litName = a
				_, lit = splitHeadBody(it.raw(s.host))
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

func envelope(it *item, host string) string {
	date := it.date.Format("Mon, 2 Jan 2006 15:04:05 -0700")
	return fmt.Sprintf("(%s %s %s %s NIL NIL NIL NIL %s)",
		qstr(date), qstr(subj(it.subject)), addrList(it.from), addrList(it.to),
		qstr(fmt.Sprintf("<%d@%s>", it.id, host)))
}

func bodyStruct(it *item) string {
	bc := strings.ReplaceAll(it.body, "\n", "\r\n")
	lines := 1
	for _, c := range it.body {
		if c == '\n' {
			lines++
		}
	}
	return fmt.Sprintf("(\"TEXT\" \"PLAIN\" (\"CHARSET\" \"UTF-8\") NIL NIL \"7BIT\" %d %d)", len(bc), lines)
}

// ---- STORE ----

func (s *session) storeSeq(rest string) bool {
	set, flagstr, silent := splitStore(rest)
	seqs := expandSeq(set, len(s.items))
	return s.applyStoreSeqs(seqs, flagstr, silent)
}

func (s *session) storeUID(rest string) bool {
	set, flagstr, silent := splitStore(rest)
	uids := expandUID(set, s.maxUID())
	return s.applyStoreSeqs(s.uidsToSeqs(uids), flagstr, silent)
}

func splitStore(rest string) (string, string, bool) {
	f := strings.Fields(rest)
	if len(f) < 2 {
		return "", "", false
	}
	set := f[0]
	op := strings.ToUpper(f[1])
	silent := strings.HasSuffix(op, ".SILENT")
	op = strings.TrimSuffix(op, ".SILENT")
	flags := strings.TrimSpace(rest[strings.Index(rest, f[1])+len(f[1]):])
	flags = strings.Trim(flags, "()")
	return set, op + " " + flags, silent
}

func (s *session) applyStoreSeqs(seqs []int, flagstr string, silent bool) bool {
	if s.readonly {
		return false
	}
	op := "replace"
	fu := strings.ToUpper(flagstr)
	if strings.HasPrefix(fu, "+FLAGS") {
		op = "add"
	} else if strings.HasPrefix(fu, "-FLAGS") {
		op = "del"
	}
	want := map[string]bool{}
	for _, f := range strings.Fields(flagstr) {
		f = strings.ToUpper(strings.Trim(f, "()"))
		want[f] = true
	}
	for _, seq := range seqs {
		if seq < 1 || seq > len(s.items) {
			continue
		}
		it := &s.items[seq-1]
		if it.deleted && !want["\\DELETED"] && op != "del" {
			continue
		}
		seen := it.read
		flag := it.starred
		del := it.deleted
		set := func(flag string, v bool) {
			switch flag {
			case "\\SEEN":
				seen = v
			case "\\FLAGGED":
				flag = v
			case "\\DELETED":
				del = v
			}
		}
		switch op {
		case "add":
			for f := range want {
				set(f, true)
			}
		case "del":
			for f := range want {
				set(f, false)
			}
		default:
			seen, flag, del = false, false, false
			for f := range want {
				set(f, true)
			}
		}
		it.read, it.starred, it.deleted = seen, flag, del
		s.db.Model(&model.Mail{}).Where("id = ?", it.id).Updates(map[string]any{"read": seen, "starred": flag})
		if !silent {
			s.untagged("%d FETCH (UID %d FLAGS (%s))", seq, it.id, flagsOf(it))
		}
	}
	return true
}

// ---- EXPUNGE / COPY / APPEND ----

func (s *session) expunge(report bool) {
	seq := 0
	kept := s.items[:0]
	for _, it := range s.items {
		seq++
		if it.deleted {
			if report {
				s.untagged("%d EXPUNGE", seq)
				seq--
			}
			s.db.Model(&model.Mail{}).Where("id = ?", it.id).Update("folder", "trash")
			continue
		}
		kept = append(kept, it)
	}
	s.items = kept
}

func (s *session) copySeq(rest string, uidMode bool) bool {
	i := strings.LastIndex(rest, " ")
	if i < 0 {
		return false
	}
	set, mbox := strings.TrimSpace(rest[:i]), strings.Trim(strings.TrimSpace(rest[i+1:]), "\"")
	_, folder, good := toFolder(mbox)
	if !good {
		return false
	}
	var seqs []int
	if uidMode {
		seqs = s.uidsToSeqs(expandUID(set, s.maxUID()))
	} else {
		seqs = expandSeq(set, len(s.items))
	}
	for _, seq := range seqs {
		if seq < 1 || seq > len(s.items) {
			continue
		}
		it := s.items[seq-1]
		s.db.Create(&model.Mail{UserID: s.user.ID, From: it.from, To: it.to,
			Subject: it.subject, Body: it.body, Folder: folder, Read: it.read, Starred: it.starred})
	}
	return true
}

// APPEND mbox [flags] {n[+]}
func (s *session) doAppend(rest string) bool {
	lb := strings.LastIndex(rest, "{")
	rb := strings.LastIndex(rest, "}")
	if lb < 0 || rb < 0 || rb < lb {
		return false
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(rest[lb+1:rb], "+"))
	if n <= 0 || n > 2<<20 {
		return false
	}
	mbox := strings.TrimSpace(rest[:lb])
	flags := ""
	if fi := strings.Index(mbox, "("); fi >= 0 {
		fe := strings.Index(mbox, ")")
		if fe > fi {
			flags = mbox[fi+1 : fe]
			mbox = strings.TrimSpace(mbox[:fi])
		}
	}
	_, folder, good := toFolder(strings.Trim(mbox, "\""))
	if !good {
		return false
	}
	literalPlus := strings.HasSuffix(rest[lb+1:rb], "+")
	if !literalPlus {
		s.w.WriteString("+ ready\r\n")
		s.w.Flush()
	}
	buf := make([]byte, n)
	read := 0
	for read < n {
		k, err := s.r.Read(buf[read:])
		if err != nil || k <= 0 {
			return false
		}
		read += k
	}
	// 吃掉结尾 CRLF
	s.r.ReadString('\n')
	raw := string(buf)
	subject, body := splitRaw(raw)
	from := headField(raw, "from")
	if from == "" {
		from = s.user.Email
	}
	to := headField(raw, "to")
	m := model.Mail{UserID: s.user.ID, From: from, To: to, Subject: subject, Body: body, Folder: folder}
	fu := strings.ToUpper(flags)
	if strings.Contains(fu, "\\SEEN") {
		m.Read = true
	}
	if strings.Contains(fu, "\\FLAGGED") {
		m.Starred = true
	}
	s.db.Create(&m)
	if folder == s.folder && s.mbox != "" {
		s.loadBox()
	}
	return true
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

// ---- SEARCH ----

type cursor struct {
	toks []string
	pos  int
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQ = !inQ
			continue
		}
		if !inQ && (c == ' ' || c == '(' || c == ')') {
			flush()
			if c == '(' || c == ')' {
				out = append(out, string(c))
			}
			continue
		}
		cur.WriteByte(c)
	}
	flush()
	return out
}

func (s *session) search(args string, uidMode bool) []uint32 {
	c := &cursor{toks: tokenize(args)}
	// 跳过 CHARSET xxx
	var toks []string
	for i := 0; i < len(c.toks); i++ {
		if strings.EqualFold(c.toks[i], "CHARSET") {
			i++
			continue
		}
		toks = append(toks, c.toks[i])
	}
	c.toks = toks
	m := s.matchOr(c)
	var out []uint32
	for i, it := range s.items {
		if it.deleted {
			continue
		}
		if m(&it) {
			if uidMode {
				out = append(out, it.id)
			} else {
				out = append(out, uint32(i+1))
			}
		}
	}
	return out
}

// 顶层隐式 AND；OR 带两个单操作数（NOT/括号/原子）
func (s *session) matchOr(c *cursor) func(*item) bool {
	var parts []func(*item) bool
	for c.pos < len(c.toks) && c.toks[c.pos] != ")" {
		if strings.EqualFold(c.toks[c.pos], "OR") {
			c.pos++
			a := s.matchNot(c)
			b := s.matchNot(c)
			parts = append(parts, func(it *item) bool { return a(it) || b(it) })
			continue
		}
		parts = append(parts, s.matchNot(c))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return func(it *item) bool {
		for _, p := range parts {
			if !p(it) {
				return false
			}
		}
		return true
	}
}

func (s *session) matchNot(c *cursor) func(*item) bool {
	if c.pos < len(c.toks) && strings.EqualFold(c.toks[c.pos], "NOT") {
		c.pos++
		inner := s.matchNot(c)
		return func(it *item) bool { return !inner(it) }
	}
	if c.pos < len(c.toks) && c.toks[c.pos] == "(" {
		c.pos++
		m := s.matchOr(c)
		if c.pos < len(c.toks) && c.toks[c.pos] == ")" {
			c.pos++
		}
		return m
	}
	return s.matchAtom(c)
}

func (s *session) matchAtom(c *cursor) func(*item) bool {
	if c.pos >= len(c.toks) {
		return func(*item) bool { return true }
	}
	tok := strings.ToUpper(c.toks[c.pos])
	c.pos++
	yes := func(*item) bool { return true }
	switch tok {
	case "ALL":
		return yes
	case "SEEN":
		return func(it *item) bool { return it.read }
	case "UNSEEN", "NEW":
		return func(it *item) bool { return !it.read }
	case "FLAGGED":
		return func(it *item) bool { return it.starred }
	case "UNFLAGGED":
		return func(it *item) bool { return !it.starred }
	case "DELETED":
		return func(it *item) bool { return it.deleted }
	case "UNDELETED":
		return func(it *item) bool { return !it.deleted }
	case "RECENT":
		return yes
	case "OLD":
		return func(*item) bool { return false }
	case "SMALLER", "LARGER":
		n := nextNum(c)
		if tok == "SMALLER" {
			return func(it *item) bool { return it.size < n }
		}
		return func(it *item) bool { return it.size > n }
	case "SUBJECT", "FROM", "TO", "BODY", "TEXT", "CC", "BCC":
		str := nextStr(c)
		l := strings.ToLower(str)
		return func(it *item) bool {
			var hay string
			switch tok {
			case "SUBJECT":
				hay = it.subject
			case "FROM", "CC", "BCC":
				hay = it.from
			case "TO":
				hay = it.to
			case "BODY":
				hay = it.body
			default:
				hay = it.subject + "\n" + it.body + "\n" + it.from + "\n" + it.to
			}
			return strings.Contains(strings.ToLower(hay), l)
		}
	case "HEADER":
		field := nextStr(c)
		str := nextStr(c)
		l := strings.ToLower(str)
		return func(it *item) bool {
			var hay string
			switch strings.ToUpper(field) {
			case "SUBJECT":
				hay = it.subject
			case "FROM":
				hay = it.from
			case "TO":
				hay = it.to
			default:
				hay = ""
			}
			return strings.Contains(strings.ToLower(hay), l)
		}
	case "BEFORE", "ON", "SINCE":
		d := nextDate(c)
		switch tok {
		case "BEFORE":
			return func(it *item) bool { return dayOf(it.date).Before(d) }
		case "SINCE":
			return func(it *item) bool { return !dayOf(it.date).Before(d) }
		default:
			return func(it *item) bool { return dayOf(it.date).Equal(d) }
		}
	case "SENTBEFORE", "SENTON", "SENTSINCE":
		_ = nextStr(c)
		return yes
	case "UID":
		set := ""
		if c.pos < len(c.toks) {
			set = c.toks[c.pos]
			c.pos++
		}
		in := map[uint32]bool{}
		for _, u := range expandUID(set, s.maxUID()) {
			in[u] = true
		}
		return func(it *item) bool { return in[it.id] }
	default:
		return yes // 未知条件忽略（不断连，宁可多返回）
	}
}

func nextStr(c *cursor) string {
	if c.pos >= len(c.toks) {
		return ""
	}
	s := c.toks[c.pos]
	c.pos++
	return s
}

func nextNum(c *cursor) int {
	n, _ := strconv.Atoi(nextStr(c))
	return n
}

func nextDate(c *cursor) time.Time {
	t, _ := time.Parse("2-Jan-2006", nextStr(c))
	return dayOf(t)
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func uidsStr(v []uint32, _ bool) string {
	if len(v) == 0 {
		return ""
	}
	var b strings.Builder
	for _, n := range v {
		b.WriteString(fmt.Sprintf(" %d", n))
	}
	return b.String()
}

