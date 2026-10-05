// Package quota 计算与判断用户存储配额。
package quota

import (
	"mailserver/internal/model"

	"gorm.io/gorm"
)

// usageQuery 统计用户已用字节：正文 + HTML 正文 + 附件解码后的真实字节。
//
// 附件只对元数据里的 size 字段求和（jsonb_array_elements），而不是 length(attachments)，
// 否则 blob 化之后 attachments 只剩文件名/blob id 等元数据，会严重少计。
// 存量邮件若有缺失 size，先执行 `mailserver fix-attachment-sizes` 回填。
const usageQuery = `
SELECT COALESCE(SUM(
    LENGTH(COALESCE(body, '')) + LENGTH(COALESCE(body_html, '')) +
    CASE WHEN COALESCE(attachments, '') ~ '^\s*\[' THEN
        COALESCE((
            SELECT SUM(COALESCE(NULLIF(elem->>'size', '')::bigint, 0))
            FROM jsonb_array_elements(attachments::jsonb) AS elem
        ), 0)
    ELSE 0 END
), 0)
FROM mails WHERE user_id = ?`

// Usage 返回用户已用字节数（正文 + HTML 正文 + 附件真实字节）。
func Usage(db *gorm.DB, uid uint) int64 {
	var n int64
	db.Raw(usageQuery, uid).Scan(&n)
	return n
}

// Exceeded 判断用户是否已超出配额（QuotaMB<=0 表示不限）。
func Exceeded(db *gorm.DB, u *model.User) bool {
	if u == nil || u.QuotaMB <= 0 {
		return false
	}
	return Usage(db, u.ID) >= int64(u.QuotaMB)<<20
}
