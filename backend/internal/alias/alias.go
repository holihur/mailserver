// Package alias 解析收件人别名 / 转发：把发给某地址的邮件投递到一个或多个目标。
package alias

import (
	"strings"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

// Match 返回匹配该地址的别名；精确地址优先于整域 catch-all（@domain）。无匹配返回 nil。
func Match(db *gorm.DB, addr string) *model.MailAlias {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return nil
	}
	var exact model.MailAlias
	if err := db.Where("LOWER(source) = ? AND enabled = ?", addr, true).First(&exact).Error; err == nil {
		return &exact
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return nil
	}
	var catch model.MailAlias
	if err := db.Where("source = ? AND enabled = ?", addr[at:], true).First(&catch).Error; err == nil {
		return &catch
	}
	return nil
}

// Targets 解析目标地址列表（逗号/分号/空白分隔，去重、小写）。
func Targets(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" || !strings.Contains(f, "@") || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// ValidSource 校验别名来源：完整地址（abc@example.com）或整域（@example.com）。
func ValidSource(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " /") {
		return false
	}
	if strings.HasPrefix(s, "@") {
		return strings.Contains(s[1:], ".")
	}
	return strings.Contains(s, "@")
}
