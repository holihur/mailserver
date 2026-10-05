package handler

// 管理后台「备份」页：定时策略、立即备份、列表（大小/时间/校验）、下载/删除、
// 以及带二次确认的恢复（恢复前自动把当前状态另存一份）。仅管理员可用。

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mailserver/internal/backup"
)

// backupDir 解析当前备份目录：后台设置 > DATA_DIR/backups。
func (a *Admin) backupDir() string {
	if d := a.RT.BackupDir(); d != "" {
		return d
	}
	if a.DataDir != "" {
		return filepath.Join(a.DataDir, "backups")
	}
	return ""
}

// Backups 处理 /api/admin/backups：
//
//	GET    ?action=download&name=xxx  下载备份
//	GET                              列出备份 + 当前策略
//	POST                             立即备份
//	DELETE ?name=xxx                 删除备份
func (a *Admin) Backups(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	dir := a.backupDir()
	if dir == "" {
		writeJSON(w, 500, map[string]string{"error": "备份目录不可用"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("action") == "download" {
			a.downloadBackup(w, r, dir)
			return
		}
		entries, err := backup.List(dir)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "读取备份目录失败: " + err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{
			"dir":            dir,
			"interval_hours": int(a.RT.BackupIntervalHours() / time.Hour),
			"keep":           a.RT.BackupKeep(),
			"encrypted":      len(a.MasterKey) > 0,
			"backups":        entries,
		})
	case http.MethodPost:
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeJSON(w, 500, map[string]string{"error": "创建备份目录失败: " + err.Error()})
			return
		}
		out, err := backup.Scheduled(a.DB, a.DataDir, dir, a.RT.BackupKeep(), a.MasterKey)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "备份失败: " + err.Error()})
			return
		}
		writeJSON(w, 201, map[string]any{"name": filepath.Base(out), "ok": true})
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		path, ok := a.safeBackupPath(dir, name)
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "备份名不合法"})
			return
		}
		if err := os.Remove(path); err != nil {
			writeJSON(w, 500, map[string]string{"error": "删除失败: " + err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// RestoreBackup 处理 /api/admin/backups/restore，body: {"name":"...","confirm":"RESTORE"}。
// 恢复会覆盖当前数据，恢复前自动另存当前状态（返回 safety 路径）。
func (a *Admin) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Name    string `json:"name"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	if strings.ToUpper(strings.TrimSpace(in.Confirm)) != "RESTORE" {
		writeJSON(w, 400, map[string]string{"error": "需要二次确认（confirm=RESTORE）"})
		return
	}
	dir := a.backupDir()
	path, ok := a.safeBackupPath(dir, in.Name)
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "备份名不合法"})
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeJSON(w, 404, map[string]string{"error": "备份不存在"})
		return
	}
	safety, err := backup.Restore(a.DB, a.DataDir, path, a.MasterKey)
	if err != nil {
		log.Printf("管理员恢复备份失败 file=%s err=%v", in.Name, err)
		writeJSON(w, 500, map[string]string{"error": "恢复失败: " + err.Error()})
		return
	}
	log.Printf("管理员从备份恢复 file=%s safety=%s", in.Name, safety)
	writeJSON(w, 200, map[string]any{"ok": true, "safety": filepath.Base(safety)})
}

func (a *Admin) downloadBackup(w http.ResponseWriter, r *http.Request, dir string) {
	path, ok := a.safeBackupPath(dir, r.URL.Query().Get("name"))
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "备份名不合法"})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "备份不存在"})
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+info.Name()+`"`)
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// safeBackupPath 校验名称并拼接出目录内路径，防路径穿越。
func (a *Admin) safeBackupPath(dir, name string) (string, bool) {
	if dir == "" || !backup.SafeName(name) {
		return "", false
	}
	return filepath.Join(dir, name), true
}
