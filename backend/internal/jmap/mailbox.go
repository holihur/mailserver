package jmap

import (
	"encoding/json"
	"strings"

	"mailserver/internal/model"
)

// ---------- Mailbox ----------

type mailboxObj struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ParentID      *string         `json:"parentId"`
	Role          *string         `json:"role"`
	SortOrder     int             `json:"sortOrder"`
	TotalEmails   int64           `json:"totalEmails"`
	UnreadEmails  int64           `json:"unreadEmails"`
	TotalThreads  int64           `json:"totalThreads"`
	UnreadThreads int64           `json:"unreadThreads"`
	MyRights      map[string]bool `json:"myRights"`
	IsSubscribed  bool            `json:"isSubscribed"`
}

var roleOf = map[string]string{"inbox": "inbox", "sent": "sent", "draft": "drafts", "trash": "trash"}

func (s *Server) mailboxes(u *model.User) []mailboxObj {
	rights := map[string]bool{"mayReadItems": true, "mayAddItems": true, "mayRemoveItems": true, "maySetSeen": true, "maySetKeywords": true, "mayCreateChild": false, "mayRename": false, "mayDelete": false, "maySubmit": false}
	var out []mailboxObj
	add := func(key, name string) {
		var total, unread int64
		s.DB.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", u.ID, key).Count(&total)
		s.DB.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", u.ID, key).Where(map[string]any{"read": false}).Count(&unread)
		var role *string
		if r, ok := roleOf[key]; ok {
			role = &r
		}
		out = append(out, mailboxObj{ID: key, Name: name, Role: role, TotalEmails: total, UnreadEmails: unread,
			TotalThreads: total, UnreadThreads: unread, MyRights: rights, IsSubscribed: true})
	}
	add("inbox", "Inbox")
	add("sent", "Sent")
	add("draft", "Drafts")
	add("trash", "Trash")
	add("deleted", "Deleted")
	var folders []model.MailFolder
	s.DB.Where("user_id = ?", u.ID).Order("name").Find(&folders)
	for _, f := range folders {
		add("c"+strID(f.ID), f.Name)
	}
	return out
}

func (s *Server) mailboxGet(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		IDs []string `json:"ids"`
	}
	_ = json.Unmarshal(args, &a)
	all := s.mailboxes(u)
	list := all
	if len(a.IDs) > 0 {
		set := map[string]bool{}
		for _, id := range a.IDs {
			set[id] = true
		}
		list = nil
		for _, m := range all {
			if set[m.ID] {
				list = append(list, m)
			}
		}
	}
	return map[string]any{"accountId": acct, "state": "1", "list": list, "notFound": []string{}}, nil
}

func (s *Server) mailboxQuery(u *model.User, acct string) (any, error) {
	ids := []string{}
	for _, m := range s.mailboxes(u) {
		ids = append(ids, m.ID)
	}
	return map[string]any{"accountId": acct, "queryState": "1", "canCalculateChanges": false, "position": 0, "ids": ids, "total": len(ids)}, nil
}

func (s *Server) mailboxSet(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		Create  map[string]map[string]any `json:"create"`
		Update  map[string]map[string]any `json:"update"`
		Destroy []string                  `json:"destroy"`
	}
	_ = json.Unmarshal(args, &a)
	created := map[string]any{}
	updated := map[string]any{}
	destroyed := []string{}
	notFound := []string{}
	for cid, obj := range a.Create {
		name, _ := obj["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		f := model.MailFolder{UserID: u.ID, Name: name}
		if err := s.DB.Create(&f).Error; err != nil {
			continue
		}
		created[cid] = map[string]any{"id": "c" + strID(f.ID)}
	}
	for id, patch := range a.Update {
		if !strings.HasPrefix(id, "c") {
			continue
		}
		upd := map[string]any{}
		if n, ok := patch["name"].(string); ok && strings.TrimSpace(n) != "" {
			upd["name"] = strings.TrimSpace(n)
		}
		if len(upd) > 0 {
			s.DB.Model(&model.MailFolder{}).Where("id = ? AND user_id = ?", strings.TrimPrefix(id, "c"), u.ID).Updates(upd)
		}
		updated[id] = nil
	}
	for _, id := range a.Destroy {
		if !strings.HasPrefix(id, "c") {
			notFound = append(notFound, id)
			continue
		}
		s.DB.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", u.ID, id).Update("folder", "inbox")
		s.DB.Where("id = ? AND user_id = ?", strings.TrimPrefix(id, "c"), u.ID).Delete(&model.MailFolder{})
		destroyed = append(destroyed, id)
	}
	return map[string]any{"accountId": acct, "oldState": "1", "newState": "1",
		"created": created, "updated": updated, "destroyed": destroyed,
		"notCreated": nil, "notUpdated": nil, "notDestroyed": nil, "notFound": notFound}, nil
}
