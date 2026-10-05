// Package mailsearch 提供统一邮件检索：解析「字段:值」语法并构造 GORM 查询，
// 供网页 /api/mails?q=、JMAP Email/query、IMAP SEARCH 共用，避免各处重复实现。
package mailsearch

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Term 表示一个检索词；Field 为空表示在任意字段（主题/发件人/收件人/正文）中匹配。
type Term struct {
	Field string // "", from, to, subject, body
	Value string
}

// Query 是一组 AND 关系的检索条件。
type Query struct {
	Terms         []Term
	After         time.Time // created_at >= After（当天 00:00）
	Before        time.Time // created_at < Before（当天 00:00）
	HasAttachment bool
}

var fieldAliases = map[string]string{
	"from": "from", "to": "to", "subject": "subject", "body": "body",
}

// Parse 解析 Gmail 子集语法：
//
//	from:alice   to:bob   subject:hi   body:hello
//	after:2024-01-01   before:2024-02-01   has:attachment
//
// 其余裸词按任意字段匹配；多个条件之间为 AND。支持双引号包裹含空格的值。
func Parse(raw string) Query {
	var q Query
	for _, tok := range splitTokens(raw) {
		low := strings.ToLower(tok)
		switch {
		case low == "has:attachment" || low == "has:attachments":
			q.HasAttachment = true
		case strings.HasPrefix(low, "after:"):
			q.After = parseDay(tok[len("after:"):])
		case strings.HasPrefix(low, "before:"):
			q.Before = parseDay(tok[len("before:"):])
		default:
			if i := strings.Index(tok, ":"); i > 0 {
				if field, ok := fieldAliases[strings.ToLower(tok[:i])]; ok {
					if v := strings.TrimSpace(tok[i+1:]); v != "" {
						q.Terms = append(q.Terms, Term{Field: field, Value: v})
					}
					continue
				}
			}
			if tok != "" {
				q.Terms = append(q.Terms, Term{Value: tok})
			}
		}
	}
	return q
}

// IsEmpty 判断是否没有任何条件。
func (q Query) IsEmpty() bool {
	return len(q.Terms) == 0 && q.After.IsZero() && q.Before.IsZero() && !q.HasAttachment
}

// Apply 把条件追加到 GORM 查询。字段名用双引号包裹以兼容保留字 from/to。
func (q Query) Apply(tx *gorm.DB) *gorm.DB {
	for _, t := range q.Terms {
		like := "%" + escapeLike(t.Value) + "%"
		switch t.Field {
		case "from":
			tx = tx.Where(`"from" ILIKE ? ESCAPE '\'`, like)
		case "to":
			tx = tx.Where(`"to" ILIKE ? ESCAPE '\'`, like)
		case "subject":
			tx = tx.Where(`subject ILIKE ? ESCAPE '\'`, like)
		case "body":
			tx = tx.Where(`body ILIKE ? ESCAPE '\'`, like)
		default:
			tx = tx.Where(`(subject ILIKE ? ESCAPE '\' OR "from" ILIKE ? ESCAPE '\' OR "to" ILIKE ? ESCAPE '\' OR body ILIKE ? ESCAPE '\')`,
				like, like, like, like)
		}
	}
	if q.HasAttachment {
		tx = tx.Where("attachments <> '' AND attachments <> '[]'")
	}
	if !q.After.IsZero() {
		tx = tx.Where("created_at >= ?", q.After)
	}
	if !q.Before.IsZero() {
		tx = tx.Where("created_at < ?", q.Before)
	}
	return tx
}

// CoarseFilter 为一组文本词生成 OR 连接的 SQL 片段与参数（命中任意词即为真）。
// 用于 IMAP 先由数据库把候选集缩小，再做精确布尔匹配，避免 500 封快照与内存全表扫描。
func CoarseFilter(terms []Term) (string, []any) {
	var parts []string
	var args []any
	for _, t := range terms {
		like := "%" + escapeLike(t.Value) + "%"
		switch t.Field {
		case "from":
			parts = append(parts, `"from" ILIKE ? ESCAPE '\'`)
			args = append(args, like)
		case "to":
			parts = append(parts, `"to" ILIKE ? ESCAPE '\'`)
			args = append(args, like)
		case "subject":
			parts = append(parts, `subject ILIKE ? ESCAPE '\'`)
			args = append(args, like)
		case "body":
			parts = append(parts, `body ILIKE ? ESCAPE '\'`)
			args = append(args, like)
		default:
			parts = append(parts, `(subject ILIKE ? ESCAPE '\' OR "from" ILIKE ? ESCAPE '\' OR "to" ILIKE ? ESCAPE '\' OR body ILIKE ? ESCAPE '\')`)
			args = append(args, like, like, like, like)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// parseDay 解析 YYYY-MM-DD / YYYY/MM/DD / RFC3339，失败返回零值。
func parseDay(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			y, m, d := t.Date()
			return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		}
	}
	return time.Time{}
}

// splitTokens 按空白切词，保留双引号内的空格并去掉引号。
func splitTokens(s string) []string {
	var out []string
	var b strings.Builder
	inQ := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQ = !inQ
		case !inQ && (c == ' ' || c == '\t' || c == '\r' || c == '\n'):
			flush()
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return out
}
