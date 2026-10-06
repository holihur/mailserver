package jmap

import (
	"encoding/json"

	"mailserver/internal/model"
)

// ---------- Thread ----------

func (s *Server) threadGet(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		IDs []string `json:"ids"`
	}
	_ = json.Unmarshal(args, &a)
	list := []any{}
	notFound := []string{}
	for _, id := range a.IDs {
		var m model.Mail
		if err := s.DB.Where("id = ? AND user_id = ?", id, u.ID).First(&m).Error; err != nil {
			notFound = append(notFound, id)
			continue
		}
		list = append(list, map[string]any{"id": strID(m.ID), "emailIds": []string{strID(m.ID)}})
	}
	return map[string]any{"accountId": acct, "state": "1", "list": list, "notFound": notFound}, nil
}
