// Package quota 计算与判断用户存储配额。
package quota

import (
	"mailserver/internal/model"

	"gorm.io/gorm"
)

// Usage 返回用户已用字节数（正文 + HTML 正文 + 附件长度之和）。
func Usage(db *gorm.DB, uid uint) int64 {
	var n int64
	db.Raw("SELECT COALESCE(SUM(length(body) + length(body_html) + length(attachments)), 0) FROM mails WHERE user_id = ?", uid).Scan(&n)
	return n
}

// Exceeded 判断用户是否已超出配额（QuotaMB<=0 表示不限）。
func Exceeded(db *gorm.DB, u *model.User) bool {
	if u == nil || u.QuotaMB <= 0 {
		return false
	}
	return Usage(db, u.ID) >= int64(u.QuotaMB)<<20
}
