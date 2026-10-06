package jmap

import (
	"encoding/json"
	"time"

	"mailserver/internal/contacts"
	"mailserver/internal/model"
)

// ---------- EmailSubmission ----------

func (s *Server) submissionGet(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		IDs []string `json:"ids"`
	}
	_ = json.Unmarshal(args, &a)
	tx := s.DB.Where("user_id = ? AND folder = ?", u.ID, "sent")
	if len(a.IDs) > 0 {
		tx = tx.Where("id IN ?", a.IDs)
	}
	var ms []model.Mail
	tx.Order("id DESC").Limit(200).Find(&ms)
	list := []any{}
	for _, m := range ms {
		list = append(list, map[string]any{
			"id": "sub" + strID(m.ID), "identityId": "primary", "emailId": strID(m.ID),
			"threadId": strID(m.ID), "envelope": nil,
			"sendAt":     m.CreatedAt.UTC().Format(time.RFC3339),
			"undoStatus": map[bool]string{true: "final", false: "pending"}[m.Relayed],
		})
	}
	return map[string]any{"accountId": acct, "state": "1", "list": list, "notFound": []string{}}, nil
}

func (s *Server) submissionSet(u *model.User, acct string, args json.RawMessage, createdIds map[string]string) (any, error) {
	var a struct {
		Create map[string]struct {
			EmailID    string `json:"emailId"`
			IdentityID string `json:"identityId"`
		} `json:"create"`
	}
	_ = json.Unmarshal(args, &a)
	created := map[string]any{}
	notCreated := map[string]any{}
	for cid, sub := range a.Create {
		emailID := resolveIDs([]string{sub.EmailID}, createdIds)[0]
		var m model.Mail
		if err := s.DB.Where("id = ? AND user_id = ?", emailID, u.ID).First(&m).Error; err != nil {
			notCreated[cid] = map[string]any{"type": "invalidEmail", "description": "邮件不存在"}
			continue
		}
		s.DB.Model(&m).Updates(map[string]any{"folder": "sent", "status": "queued"})
		contacts.Collect(s.DB, u.ID, m.From, m.To, m.Cc, m.Bcc)
		if s.MQ != nil {
			_ = s.MQ.EnqueueSend(m.ID)
		}
		created[cid] = map[string]any{"id": "sub" + strID(m.ID), "sendAt": time.Now().UTC().Format(time.RFC3339)}
	}
	return map[string]any{"accountId": acct, "oldState": "1", "newState": "1",
		"created": created, "notCreated": notCreated}, nil
}
