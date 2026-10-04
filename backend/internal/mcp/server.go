// Package mcp 提供一个最小化的 MCP（Model Context Protocol）HTTP 服务端。
// 传输：Streamable HTTP（POST 单端点，JSON-RPC 2.0）；鉴权：Authorization: Bearer <PAT>。
// 暴露邮件相关工具，供 AI 客户端读写邮箱。
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/contacts"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

type Server struct {
	DB *gorm.DB
	MQ interface{ EnqueueSend(mailID uint) error }
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func respond(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func writeErr(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	var rid any
	if len(id) > 0 {
		rid = json.RawMessage(id)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": rid,
		"error": map[string]any{"code": code, "message": msg},
	})
}

func (s *Server) Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// 可选 SSE 保活（部分客户端会主动打开该流）
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
		return
	case "OPTIONS":
		w.WriteHeader(204)
		return
	case "POST":
	default:
		w.WriteHeader(405)
		return
	}

	u, err := auth.AuthenticateMail(s.DB, "", bearer(r), auth.HostOf(r.RemoteAddr), auth.ScopeMCP)
	if err != nil {
		writeErr(w, nil, -32001, "unauthorized: "+err.Error())
		return
	}

	var req rpcReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		writeErr(w, nil, -32700, "parse error")
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(202) // 通知，无响应
		return
	}
	switch req.Method {
	case "initialize":
		respond(w, req.ID, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sweetcorn", "version": "1.0.0"},
		})
	case "ping":
		respond(w, req.ID, map[string]any{})
	case "tools/list":
		respond(w, req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		s.call(w, req, u)
	default:
		writeErr(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) call(w http.ResponseWriter, req rpcReq, u *model.User) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(req.Params, &p)
	text, err := s.run(u, p.Name, p.Arguments)
	if err != nil {
		respond(w, req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "错误: " + err.Error()}},
			"isError": true,
		})
		return
	}
	respond(w, req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}})
}

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func summaries(ms []model.Mail) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			"id": m.ID, "from": m.From, "to": m.To, "subject": m.Subject,
			"read": m.Read, "starred": m.Starred, "folder": m.Folder,
			"has_attachment": strings.TrimSpace(m.Attachments) != "" && m.Attachments != "[]",
			"created_at":     m.CreatedAt,
		})
	}
	return out
}

func (s *Server) run(u *model.User, name string, args json.RawMessage) (string, error) {
	switch name {
	case "list_mails", "search_mails":
		var a struct {
			Folder string `json:"folder"`
			Q      string `json:"q"`
			Page   int    `json:"page"`
		}
		_ = json.Unmarshal(args, &a)
		if a.Folder == "" {
			a.Folder = "inbox"
		}
		if a.Page < 1 {
			a.Page = 1
		}
		tx := s.DB.Where("user_id = ? AND folder = ?", u.ID, a.Folder)
		if a.Q != "" {
			like := "%" + a.Q + "%"
			tx = tx.Where("subject LIKE ? OR \"from\" LIKE ? OR \"to\" LIKE ?", like, like, like)
		}
		var ms []model.Mail
		tx.Order("id DESC").Offset((a.Page - 1) * 20).Limit(20).Find(&ms)
		return toJSON(summaries(ms)), nil

	case "get_mail":
		var a struct {
			ID uint `json:"id"`
		}
		_ = json.Unmarshal(args, &a)
		var m model.Mail
		if err := s.DB.Where("id = ? AND user_id = ?", a.ID, u.ID).First(&m).Error; err != nil {
			return "", fmt.Errorf("邮件不存在")
		}
		s.DB.Model(&m).Update("read", true)
		return toJSON(m), nil

	case "send_mail":
		var a struct {
			To, Cc, Bcc, Subject, Body string
		}
		_ = json.Unmarshal(args, &a)
		if strings.TrimSpace(a.To) == "" {
			return "", fmt.Errorf("缺少收件人 to")
		}
		m := model.Mail{UserID: u.ID, From: u.Email, To: a.To, Cc: a.Cc, Bcc: a.Bcc,
			Subject: a.Subject, Body: a.Body, Folder: "sent", Read: true, Status: "queued"}
		if err := s.DB.Create(&m).Error; err != nil {
			return "", fmt.Errorf("保存失败")
		}
		contacts.Collect(s.DB, u.ID, m.From, m.To, m.Cc, m.Bcc)
		if s.MQ != nil {
			_ = s.MQ.EnqueueSend(m.ID)
		}
		return toJSON(map[string]any{"ok": true, "id": m.ID, "status": "queued"}), nil

	case "mark_read":
		var a struct {
			ID   uint `json:"id"`
			Read bool `json:"read"`
		}
		_ = json.Unmarshal(args, &a)
		s.DB.Model(&model.Mail{}).Where("id = ? AND user_id = ?", a.ID, u.ID).Update("read", a.Read)
		return toJSON(map[string]any{"ok": true}), nil

	case "move_mail":
		var a struct {
			ID     uint   `json:"id"`
			Folder string `json:"folder"`
		}
		_ = json.Unmarshal(args, &a)
		if a.Folder == "" {
			return "", fmt.Errorf("缺少目标 folder")
		}
		s.DB.Model(&model.Mail{}).Where("id = ? AND user_id = ?", a.ID, u.ID).Update("folder", a.Folder)
		return toJSON(map[string]any{"ok": true}), nil

	case "delete_mail":
		var a struct {
			ID uint `json:"id"`
		}
		_ = json.Unmarshal(args, &a)
		s.DB.Model(&model.Mail{}).Where("id = ? AND user_id = ?", a.ID, u.ID).Update("folder", "trash")
		return toJSON(map[string]any{"ok": true}), nil

	case "list_folders":
		var fs []model.MailFolder
		s.DB.Where("user_id = ?", u.ID).Find(&fs)
		return toJSON(fs), nil

	case "list_contacts":
		var cs []model.Contact
		s.DB.Where("user_id = ?", u.ID).Find(&cs)
		return toJSON(cs), nil
	}
	return "", fmt.Errorf("未知工具: %s", name)
}

func str() map[string]any { return map[string]any{"type": "string"} }

func toolDefs() []map[string]any {
	return []map[string]any{
		{"name": "list_mails", "description": "列出指定文件夹的邮件（默认 inbox，每页 20 封）",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"folder": str(), "page": map[string]any{"type": "integer"},
			}}},
		{"name": "search_mails", "description": "按关键词搜索邮件",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"q": str(), "folder": str(),
			}, "required": []string{"q"}}},
		{"name": "get_mail", "description": "读取一封邮件的完整内容（并标为已读）",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"id": map[string]any{"type": "integer"},
			}, "required": []string{"id"}}},
		{"name": "send_mail", "description": "发送邮件",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"to": str(), "cc": str(), "bcc": str(), "subject": str(), "body": str(),
			}, "required": []string{"to", "subject", "body"}}},
		{"name": "mark_read", "description": "标记邮件已读/未读",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"id": map[string]any{"type": "integer"}, "read": map[string]any{"type": "boolean"},
			}, "required": []string{"id", "read"}}},
		{"name": "move_mail", "description": "把邮件移动到指定文件夹（如 inbox/trash/deleted 或 c<id>）",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"id": map[string]any{"type": "integer"}, "folder": str(),
			}, "required": []string{"id", "folder"}}},
		{"name": "delete_mail", "description": "把邮件移入垃圾箱",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
				"id": map[string]any{"type": "integer"},
			}, "required": []string{"id"}}},
		{"name": "list_folders", "description": "列出文件夹", "inputSchema": map[string]any{"type": "object"}},
		{"name": "list_contacts", "description": "列出联系人", "inputSchema": map[string]any{"type": "object"}},
	}
}
