package message

import (
	"encoding/json"
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

// BackfillAttachmentSizes 回填历史邮件附件元数据中的真实字节数 Size（幂等），
// 返回被更新的邮件数。blob 附件按文件大小、旧 base64 附件按解码长度计算。
// 配额改为按 Size 求和后，必须对存量数据执行一次，避免历史附件被少计。
func BackfillAttachmentSizes(db *gorm.DB) (int, error) {
	var updated int
	var lastID uint
	const batch = 200
	for {
		var mails []model.Mail
		if err := db.Select("id", "attachments").
			Where("id > ? AND attachments <> '' AND attachments <> '[]'", lastID).
			Order("id").Limit(batch).Find(&mails).Error; err != nil {
			return updated, err
		}
		if len(mails) == 0 {
			break
		}
		for _, m := range mails {
			lastID = m.ID
			atts := ParseAttachments(m.Attachments)
			if len(atts) == 0 {
				continue
			}
			changed := false
			for i := range atts {
				if atts[i].Size > 0 {
					continue
				}
				if sz := atts[i].RealSize(); sz > 0 {
					atts[i].Size = int(sz)
					changed = true
				}
			}
			if !changed {
				continue
			}
			b, err := json.Marshal(atts)
			if err != nil {
				continue
			}
			if err := db.Model(&model.Mail{}).Where("id = ?", m.ID).Update("attachments", string(b)).Error; err != nil {
				return updated, err
			}
			updated++
		}
	}
	return updated, nil
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
