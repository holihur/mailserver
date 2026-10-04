package message

import (
	"fmt"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

// MigrateAttachments 把库中所有 base64 附件落盘为 blob（幂等），返回处理的邮件数。
func MigrateAttachments(db *gorm.DB) (int, error) {
	if blobStore == nil {
		return 0, fmt.Errorf("blob 存储未初始化")
	}
	var mails []model.Mail
	if err := db.Select("id", "attachments").Where("attachments <> '' AND attachments LIKE ?", "%\"data\"%").Find(&mails).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, m := range mails {
		nb := Blobify(m.Attachments)
		if nb != m.Attachments {
			if err := db.Model(&model.Mail{}).Where("id = ?", m.ID).Update("attachments", nb).Error; err != nil {
				continue
			}
			n++
		}
	}
	return n, nil
}

// GCBlobs 删除未被任何邮件引用的 blob 文件，返回删除数。
func GCBlobs(db *gorm.DB) (int, error) {
	if blobStore == nil {
		return 0, nil
	}
	ids, err := blobStore.List()
	if err != nil {
		return 0, err
	}
	referenced := map[string]bool{}
	var atts []string
	db.Model(&model.Mail{}).Where("attachments <> ''").Pluck("attachments", &atts)
	for _, a := range atts {
		for _, at := range ParseAttachments(a) {
			if at.Blob != "" {
				referenced[at.Blob] = true
			}
		}
	}
	n := 0
	for _, id := range ids {
		if !referenced[id] {
			if blobStore.Delete(id) == nil {
				n++
			}
		}
	}
	return n, nil
}
