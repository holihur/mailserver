package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/mailsearch"
	"mailserver/internal/model"
	"mailserver/internal/push"

	"gorm.io/gorm"
)

func (m *MailBox) Batch(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(m.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		IDs       []uint `json:"ids"`
		Action    string `json:"action"`
		Folder    string `json:"folder"`
		All       bool   `json:"all"`
		Q         string `json:"q"`
		SrcFolder string `json:"src_folder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	// 清空垃圾箱：移入「已删除」（软删除，可再恢复或彻底删除）。
	if in.Action == "empty" {
		res := m.DB.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", uid, "trash").Update("folder", "deleted")
		writeJSON(w, 200, map[string]any{"ok": true, "count": res.RowsAffected})
		return
	}
	if !in.All && len(in.IDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "未选择邮件"})
		return
	}
	if len(in.IDs) > 500 {
		in.IDs = in.IDs[:500]
	}
	// All=true：对当前文件夹（可带搜索条件）的全部邮件生效，不止当前页。
	var tx *gorm.DB
	if in.All {
		tx = m.DB.Where("user_id = ? AND folder = ?", uid, in.SrcFolder)
		if strings.TrimSpace(in.Q) != "" {
			tx = mailsearch.Parse(in.Q).Apply(tx)
		}
	} else {
		tx = m.DB.Where("user_id = ? AND id IN ?", uid, in.IDs)
	}
	var res *gorm.DB
	switch in.Action {
	case "delete":
		res = tx.Model(&model.Mail{}).Update("folder", "deleted")
	case "purge":
		res = tx.Delete(&model.Mail{})
	case "trash":
		res = tx.Model(&model.Mail{}).Update("folder", "trash")
	case "star":
		res = tx.Model(&model.Mail{}).Update("starred", true)
	case "unstar":
		res = tx.Model(&model.Mail{}).Update("starred", false)
	case "read":
		res = tx.Model(&model.Mail{}).Update("read", true)
	case "unread":
		res = tx.Model(&model.Mail{}).Update("read", false)
	case "move":
		if in.Folder == "" {
			writeJSON(w, 400, map[string]string{"error": "缺少目标文件夹"})
			return
		}
		res = tx.Model(&model.Mail{}).Update("folder", in.Folder)
	default:
		writeJSON(w, 400, map[string]string{"error": "不支持的操作"})
		return
	}
	count := len(in.IDs)
	if in.All {
		count = int(res.RowsAffected)
	}
	push.Notify(uid)
	writeJSON(w, 200, map[string]any{"ok": true, "count": count})
}

// POST /api/mails  {to,subject,body,folder:sent|draft}
