package handler

// 自定义文件夹：/api/folders（GET 列表 / POST 新建）、/api/folders/{id}（PATCH 重命名 / DELETE）。
// Mail.Folder 对自定义文件夹存键 "c<ID>"；删除文件夹时其邮件移回收件箱。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

type FolderBox struct{ DB *gorm.DB }

var builtinFolders = map[string]bool{"inbox": true, "sent": true, "draft": true, "trash": true, "deleted": true}

func folderKey(id uint) string { return "c" + strconv.FormatUint(uint64(id), 10) }

// FolderValid 判断键是否为该用户可用的文件夹（内置或自有自定义文件夹）。
func FolderValid(db *gorm.DB, uid uint, key string) bool {
	if builtinFolders[key] {
		return true
	}
	if strings.HasPrefix(key, "c") {
		var n int64
		db.Model(&model.MailFolder{}).Where("user_id = ? AND id = ?", uid, strings.TrimPrefix(key, "c")).Count(&n)
		return n > 0
	}
	return false
}

// GET /api/folders   POST /api/folders {name}
func (h *FolderBox) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		var fs []model.MailFolder
		h.DB.Where("user_id = ?", uid).Order("name").Find(&fs)
		if fs == nil {
			fs = []model.MailFolder{}
		}
		writeJSON(w, 200, fs)
	case "POST":
		var in struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		name := strings.TrimSpace(in.Name)
		if name == "" || len([]rune(name)) > 60 {
			writeJSON(w, 400, map[string]string{"error": "文件夹名称不能为空且不超过 60 字"})
			return
		}
		f := model.MailFolder{UserID: uid, Name: name}
		if err := h.DB.Create(&f).Error; err != nil {
			writeJSON(w, 500, map[string]string{"error": "保存失败"})
			return
		}
		writeJSON(w, 201, f)
	default:
		w.WriteHeader(405)
	}
}

// PATCH/DELETE /api/folders/{id}
func (h *FolderBox) One(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/folders/")
	id = strings.Split(id, "/")[0]
	var f model.MailFolder
	if err := h.DB.Where("id = ? AND user_id = ?", id, uid).First(&f).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "DELETE":
		h.DB.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", uid, folderKey(f.ID)).Update("folder", "inbox")
		h.DB.Delete(&f)
		writeJSON(w, 200, map[string]any{"ok": true})
	case "PATCH":
		var in struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		if name := strings.TrimSpace(in.Name); name != "" {
			h.DB.Model(&f).Update("name", name)
		}
		h.DB.First(&f, f.ID)
		writeJSON(w, 200, f)
	default:
		w.WriteHeader(405)
	}
}
