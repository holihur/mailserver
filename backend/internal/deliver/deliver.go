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

// Options 投递时可选的回执 / 认证元数据。
type Options struct {
	ReceiptTo   string // 要求回执的地址（请求方）
	IsMDN       bool   // 本条是已读回执
	ReceiptFor  uint   // MDN 关联的原邮件 ID
	AuthResults string // SPF/DKIM/DMARC 结果描述
	Quarantine  bool   // DMARC 隔离：强制投到垃圾箱
	Bulk        bool   // 批量/自动生成邮件（不发 vacation 回复）
}

// VacationHook 由 main 注入：发送 vacation 自动回复（含去重）；未注入则不回复。
var VacationHook func(db *gorm.DB, uid uint, from, subject, text string)

// ToUser 把邮件投递给用户：
//  1. 超出配额则丢弃（入站 SMTP 已在 RCPT 阶段拒绝）；
//  2. 若有启用的 Sieve 脚本且命中，按脚本动作处理（fileinto/redirect/discard/keep）；
//  3. 否则按 CEL 规则处理（trash/move/forward）。
func ToUser(db *gorm.DB, u *model.User, from, to, cc, bcc, subject, body, htmlBody, atts string, opts ...Options) {
	o := Options{}
	if len(opts) > 0 {
		o = opts[0]
	}
	if quota.Exceeded(db, u) {
		return
	}
	htmlSafe := htmlsanitize.Sanitize(htmlBody)
	if sieveDeliver(db, u, from, to, cc, subject, body, htmlSafe, atts, o) {
		return
	}
	d := rules.Apply(db, u.ID, rules.Input{
		From: from, To: to, Cc: cc, Bcc: bcc, Subject: subject, Body: body,
		Size: len(body), Attachments: len(message.ParseAttachments(atts)),
	}, "inbox")
	if d.Action == "forward" && len(d.Forward) > 0 {
		Forward(db, from, d.Forward, subject, body, atts)
		folder := "inbox"
		if o.Quarantine {
			folder = "trash"
		}
		db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc, Bcc: bcc,
			Subject: subject, Body: body, BodyHTML: htmlSafe, Attachments: atts, Folder: folder,
			ReceiptTo: o.ReceiptTo, IsMDN: o.IsMDN, ReceiptFor: o.ReceiptFor, AuthResults: o.AuthResults})
		return
	}
	folder := d.Folder
	if o.Quarantine {
		folder = "trash"
	}
	db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc, Bcc: bcc,
		Subject: subject, Body: body, BodyHTML: htmlSafe, Attachments: atts, Folder: folder,
		ReceiptTo: o.ReceiptTo, IsMDN: o.IsMDN, ReceiptFor: o.ReceiptFor, AuthResults: o.AuthResults})
}

// sieveDeliver 执行用户的启用脚本；返回 true 表示已处理（不再走 CEL 规则）。
func sieveDeliver(db *gorm.DB, u *model.User, from, to, cc, subject, body, htmlSafe, atts string, o Options) bool {
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
	reject, vacation := false, ""
	for _, a := range res.Actions {
		switch a.Type {
		case "discard":
			discard = true
		case "reject", "ereject":
			reject = true
		case "vacation":
			vacation = a.Arg
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
	if discard || reject {
		return true // 拒收/丢弃：不投递
	}
	if vacation != "" && !o.Bulk && VacationHook != nil && from != "" && !strings.EqualFold(strings.TrimSpace(from), u.Email) {
		VacationHook(db, u.ID, from, subject, vacation)
	}
	for _, r := range redirects {
		Forward(db, from, []string{r}, subject, body, atts)
	}
	if len(folders) == 0 {
		folders = []string{"inbox"}
	}
	for _, f := range folders {
		if o.Quarantine {
			f = "trash"
		}
		db.Create(&model.Mail{UserID: u.ID, From: from, To: to, Cc: cc,
			Subject: subject, Body: body, BodyHTML: htmlSafe, Attachments: atts, Folder: f, Read: seen,
			ReceiptTo: o.ReceiptTo, IsMDN: o.IsMDN, ReceiptFor: o.ReceiptFor, AuthResults: o.AuthResults})
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
