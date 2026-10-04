// Package contacts 提供通讯录的共享写入逻辑（供发信投递时自动收录收件人）。
package contacts

import (
	"strings"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

// Collect 把发件收件人（To/Cc/Bcc，支持 "姓名 <邮箱>"）自动收录到用户通讯录。
// 已存在的邮箱不覆盖，保留用户自己填写的姓名/备注；无效项与 skip 地址忽略。
// uid=0（如系统转发产生的邮件）直接跳过。
func Collect(db *gorm.DB, uid uint, skip string, fields ...string) {
	if uid == 0 || db == nil {
		return
	}
	skip = strings.ToLower(strings.TrimSpace(skip))
	for _, field := range fields {
		for _, raw := range strings.FieldsFunc(field, func(r rune) bool { return r == ',' || r == ';' }) {
			name, email := splitAddress(raw)
			if email == "" || email == skip {
				continue
			}
			var n int64
			db.Model(&model.Contact{}).Where("user_id = ? AND email = ?", uid, email).Count(&n)
			if n > 0 {
				continue
			}
			db.Create(&model.Contact{UserID: uid, Email: email, Name: name})
		}
	}
}

// splitAddress 解析单个收件人：支持 "Name <a@b.com>"、<a@b.com>、a@b.com。
func splitAddress(raw string) (name, email string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if i := strings.Index(raw, "<"); i >= 0 {
		name = strings.Trim(strings.TrimSpace(raw[:i]), `"`)
		if j := strings.Index(raw[i+1:], ">"); j >= 0 {
			email = raw[i+1 : i+1+j]
		}
	} else {
		email = raw
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return "", ""
	}
	return name, email
}
