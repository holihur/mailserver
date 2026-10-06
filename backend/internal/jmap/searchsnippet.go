package jmap

import (
	"encoding/json"

	"mailserver/internal/model"
)

// ---------- SearchSnippet ----------

func (s *Server) searchSnippet(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		EmailIDs []string `json:"emailIds"`
	}
	_ = json.Unmarshal(args, &a)
	list := []any{}
	notFound := []string{}
	for _, id := range a.EmailIDs {
		var m model.Mail
		if err := s.DB.Where("id = ? AND user_id = ?", id, u.ID).First(&m).Error; err != nil {
			notFound = append(notFound, id)
			continue
		}
		list = append(list, map[string]any{"emailId": id, "subject": nil, "preview": preview(m.Body)})
	}
	return map[string]any{"accountId": acct, "list": list, "notFound": notFound}, nil
}
