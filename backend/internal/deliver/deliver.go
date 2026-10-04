// Package deliver 统一本地投递：先应用用户 Sieve 脚本，再应用 CEL 收信规则；HTML 正文清洗后入库。
// 被入站 SMTP 与发件队列（本站互投）共用，保证行为一致。
package deliver

import (
	"strconv"
	"strings"

	"mailserver/internal/htmlsanitize"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/quota"
	"mailserver/internal/rules"
	"mailserver/internal/sieve"

	"gorm.io/gorm"
)

// ToUser 把邮件投递给用户：
//  1. 超出配额则丢弃（入站 SMTP 已在 RCPT 阶段拒绝）；
//  2. 若有启用的 Sieve 脚本且命中，按脚本动作处理（fileinto/redirect/discard/keep）；
//  3. 否则按 CEL 规则处理（trash/move/forward）。
func ToUser(db *gorm.DB, u *model.User, from, to, cc, bcc, subject, body, htmlBody, atts string) {
	if quota.Exceeded(db, u) {
		return
	}
	htmlSafe := htmlsanitize.Sanitize(htmlBody)
	if sieveDeliver(db, u, from, to, cc, subject, body, htmlSafe, atts) {
		return
	}
	d := rules.Apply(db, u.ID, rules.Input{
		From: from, To: to, Cc: cc, Bcc: bcc, Subject: subject, Body: body,
		Size: len(body), Attachments: len(message.ParseAttachments(atts)),
	}, "inbox")
	if d.Action == "forward" && len(d.Forward) > 0 {
		Forward(db, from, d.Forward, subject, body, atts)
		db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc, Bcc: bcc,
			Subject: subject, Body: body, BodyHTML: htmlSafe, Attachments: atts, Folder: "inbox"})
		return
	}
	db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc, Bcc: bcc,
		Subject: subject, Body: body, BodyHTML: htmlSafe, Attachments: atts, Folder: d.Folder})
}

// sieveDeliver 执行用户的启用脚本；返回 true 表示已处理（不再走 CEL 规则）。
func sieveDeliver(db *gorm.DB, u *model.User, from, to, cc, subject, body, htmlSafe, atts string) bool {
	var sc model.SieveScript
	if err := db.Where("user_id = ? AND active = ?", u.ID, true).First(&sc).Error; err != nil {
		return false
	}
	prog, err := sieve.Compile(sc.Script)
	if err != nil {
		return false
	}
	res := prog.Execute(sieve.NewContext(map[string]string{
		"From": from, "To": to, "Cc": cc, "Subject": subject,
	}, len(body)))
	if len(res.Actions) == 0 {
		return false
	}
	var folders, redirects []string
	discard, seen := false, false
	for _, a := range res.Actions {
		switch a.Type {
		case "discard":
			discard = true
		case "fileinto":
			folders = append(folders, sieveFolderKey(db, u.ID, a.Arg))
		case "redirect":
			redirects = append(redirects, a.Arg)
		case "addflag":
			if strings.EqualFold(strings.TrimSpace(a.Arg), "\\Seen") {
				seen = true
			}
		}
	}
	if discard {
		return true
	}
	for _, r := range redirects {
		Forward(db, from, []string{r}, subject, body, atts)
	}
	if len(folders) == 0 {
		folders = []string{"inbox"}
	}
	for _, f := range folders {
		db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc,
			Subject: subject, Body: body, BodyHTML: htmlSafe, Attachments: atts, Folder: f, Read: seen})
	}
	return true
}

// sieveFolderKey 把 Sieve 的 fileinto 名称映射为文件夹键；未知名称自动创建自定义文件夹。
func sieveFolderKey(db *gorm.DB, uid uint, name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "", "inbox":
		return "inbox"
	case "trash", "junk", "spam":
		return "trash"
	case "drafts", "draft":
		return "draft"
	case "sent":
		return "sent"
	case "deleted":
		return "deleted"
	}
	var f model.MailFolder
	if err := db.Where("user_id = ? AND LOWER(name) = ?", uid, n).First(&f).Error; err == nil {
		return "c" + strconv.FormatUint(uint64(f.ID), 10)
	}
	f = model.MailFolder{UserID: uid, Name: name}
	db.Create(&f)
	return "c" + strconv.FormatUint(uint64(f.ID), 10)
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
				Subject: subject, Body: body, Attachments: atts, Folder: "sent", Read: true, Status: "queued"})
		}
	}
}
