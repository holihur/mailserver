package jmap

import (
	"encoding/json"
	"strings"

	"mailserver/internal/model"
)

// ---------- Identity ----------

func (s *Server) identityGet(u *model.User, acct string) (any, error) {
	list := []any{map[string]any{
		"id": "primary", "name": u.Name, "email": u.Email, "replyTo": nil, "bcc": nil,
		"textSignature": u.Signature, "htmlSignature": "", "mayDelete": false,
	}}
	var exts []model.ExternalAccount
	s.DB.Where("user_id = ? AND enabled = ?", u.ID, true).Find(&exts)
	for _, e := range exts {
		name := e.Name
		if name == "" {
			name = e.Email
		}
		list = append(list, map[string]any{
			"id": "ext-" + strID(e.ID), "name": name, "email": e.Email, "replyTo": nil, "bcc": nil,
			"textSignature": "", "htmlSignature": "", "mayDelete": false,
		})
	}
	return map[string]any{"accountId": acct, "state": "1", "list": list, "notFound": []string{}}, nil
}

func (s *Server) identitySet(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		Update map[string]map[string]any `json:"update"`
	}
	_ = json.Unmarshal(args, &a)
	updated := map[string]any{}
	for id, patch := range a.Update {
		if id == "primary" {
			upd := map[string]any{}
			if n, ok := patch["name"].(string); ok {
				upd["name"] = strings.TrimSpace(n)
			}
			if sig, ok := patch["textSignature"].(string); ok {
				upd["signature"] = sig
			}
			if len(upd) > 0 {
				s.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(upd)
			}
		}
		updated[id] = nil
	}
	return map[string]any{"accountId": acct, "oldState": "1", "newState": "1", "updated": updated,
		"created": nil, "destroyed": nil, "notCreated": nil, "notUpdated": nil, "notDestroyed": nil}, nil
}
