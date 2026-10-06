package imap

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/mailsearch"
	"mailserver/internal/model"
)

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
	// 含 TEXT/BODY 等文本条件时，先由数据库粗筛候选集（避免 500 封快照 + 内存全扫），
	// 再做精确布尔匹配；非 UID 模式的序号按当前文件夹全量顺序还原。
	if terms := imapTextTerms(toks); len(terms) > 0 {
		return s.searchDB(c, terms, uidMode)
	}
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

// imapTextTerms 从 IMAP SEARCH 记号中提取全部文本字面量（不消费游标），
// 只用于「粗筛」，多提取无妨（OR 条件只扩大候选集，不会漏）。
func imapTextTerms(toks []string) []mailsearch.Term {
	var terms []mailsearch.Term
	for i := 0; i < len(toks); i++ {
		switch strings.ToUpper(toks[i]) {
		case "SUBJECT", "FROM", "TO", "BODY", "TEXT", "CC", "BCC":
			if i+1 >= len(toks) {
				continue
			}
			field := ""
			switch strings.ToUpper(toks[i]) {
			case "SUBJECT":
				field = "subject"
			case "FROM":
				field = "from"
			case "TO":
				field = "to"
			case "BODY":
				field = "body"
			}
			i++
			terms = append(terms, mailsearch.Term{Field: field, Value: toks[i]})
		case "HEADER":
			if i+2 >= len(toks) {
				continue
			}
			field := ""
			switch strings.ToLower(toks[i+1]) {
			case "subject":
				field = "subject"
			case "from":
				field = "from"
			case "to":
				field = "to"
			}
			i += 2
			terms = append(terms, mailsearch.Term{Field: field, Value: toks[i]})
		}
	}
	return terms
}

// searchDB 用数据库粗筛出候选邮件，再逐封做精确匹配，序号按文件夹全量顺序还原。
func (s *session) searchDB(c *cursor, terms []mailsearch.Term, uidMode bool) []uint32 {
	var ids []uint32
	s.db.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Order("id").Pluck("id", &ids)
	seq := make(map[uint32]uint32, len(ids))
	for i, id := range ids {
		seq[id] = uint32(i + 1)
	}
	expr, args := mailsearch.CoarseFilter(terms)
	if expr == "" {
		return s.searchByItems(c, uidMode)
	}
	var mails []model.Mail
	s.db.Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Where(expr, args...).Order("id").Find(&mails)
	m := s.matchOr(c)
	var out []uint32
	for _, mm := range mails {
		it := item{id: uint32(mm.ID), date: mm.CreatedAt, from: mm.From, to: mm.To, cc: mm.Cc,
			subject: mm.Subject, body: mm.Body, attachments: mm.Attachments, read: mm.Read, starred: mm.Starred}
		if !m(&it) {
			continue
		}
		if uidMode {
			out = append(out, it.id)
		} else if sq, ok := seq[it.id]; ok {
			out = append(out, sq)
		}
	}
	return out
}

// searchByItems 对当前快照逐封匹配（无文本条件时的回退路径）。
func (s *session) searchByItems(c *cursor, uidMode bool) []uint32 {
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
