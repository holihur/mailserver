// Package deliver 统一本地投递：应用 CEL 收信规则，支持 trash/move/forward。
// 被入站 SMTP 与发件队列（本站互投）共用，保证行为一致。
package deliver

import (
	"strings"

	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/rules"

	"gorm.io/gorm"
)

// ToUser 按规则把邮件投递给用户：
//   - action=forward：转发到目标地址，并在收件箱保留一份；
//   - 其他：按命中文件夹（默认 inbox）存入。
func ToUser(db *gorm.DB, u *model.User, from, to, cc, bcc, subject, body, atts string) {
	d := rules.Apply(db, u.ID, rules.Input{
		From: from, To: to, Cc: cc, Bcc: bcc, Subject: subject, Body: body,
		Size: len(body), Attachments: len(message.ParseAttachments(atts)),
	}, "inbox")
	if d.Action == "forward" && len(d.Forward) > 0 {
		Forward(db, from, d.Forward, subject, body, atts)
		db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc, Bcc: bcc,
			Subject: subject, Body: body, Attachments: atts, Folder: "inbox"})
		return
	}
	db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc, Bcc: bcc,
		Subject: subject, Body: body, Attachments: atts, Folder: d.Folder})
}

// Forward 把邮件投递到目标列表：本地用户进其收件箱，外部地址入发件队列外发。
func Forward(db *gorm.DB, from string, targets []string, subject, body, atts string) {
	for _, t := range targets {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || !strings.Contains(t, "@") {
			continue
		}
		var u model.User
		if err := db.Where("LOWER(email) = ?", t).First(&u).Error; err == nil {
			db.Create(&model.Mail{UserID: u.ID, From: from, To: t,
				Subject: subject, Body: body, Attachments: atts, Folder: "inbox"})
		} else {
			db.Create(&model.Mail{From: from, To: t,
				Subject: subject, Body: body, Attachments: atts, Folder: "sent", Read: true})
		}
	}
}
