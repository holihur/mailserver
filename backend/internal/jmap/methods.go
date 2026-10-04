package jmap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/contacts"
	"mailserver/internal/message"
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

// ---------- Email ----------

func addrList(s string) []map[string]string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []map[string]string
	for _, a := range strings.Split(s, ",") {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, map[string]string{"email": a})
		}
	}
	return out
}

func keywordsOf(m *model.Mail) map[string]bool {
	kw := map[string]bool{}
	if m.Read {
		kw["$seen"] = true
	}
	if m.Starred {
		kw["$flagged"] = true
	}
	if m.Folder == "draft" {
		kw["$draft"] = true
	}
	return kw
}

func (s *Server) emailObject(m *model.Mail) map[string]any {
	id := strID(m.ID)
	attachments := []any{}
	for i, a := range message.ParseAttachments(m.Attachments) {
		attachments = append(attachments, map[string]any{
			"partId": "a" + strconv.Itoa(i), "blobId": "a" + id + "-" + strconv.Itoa(i),
			"type": a.Type, "name": a.Name, "size": a.Size, "disposition": "attachment",
		})
	}
	body := m.Body
	bodyType := "text/plain"
	if strings.TrimSpace(m.BodyHTML) != "" {
		body = m.BodyHTML
		bodyType = "text/html"
	}
	part := map[string]any{"partId": "1", "blobId": "m" + id, "size": len(body), "type": bodyType}
	obj := map[string]any{
		"id": id, "blobId": "m" + id, "threadId": id,
		"mailboxIds":    map[string]bool{m.Folder: true},
		"keywords":      keywordsOf(m),
		"from":          addrList(m.From),
		"to":            addrList(m.To),
		"cc":            addrList(m.Cc),
		"bcc":           addrList(m.Bcc),
		"replyTo":       nil,
		"subject":       m.Subject,
		"sentAt":        m.CreatedAt.UTC().Format(time.RFC3339),
		"receivedAt":    m.CreatedAt.UTC().Format(time.RFC3339),
		"size":          len(body),
		"preview":       preview(m.Body),
		"hasAttachment": len(attachments) > 0,
		"bodyValues":    map[string]any{"1": map[string]any{"value": body, "isEncodingProblem": false, "isTruncated": false}},
		"bodyStructure": map[string]any{"partId": "1", "blobId": "m" + id, "type": bodyType, "size": len(body)},
		"attachments":   attachments,
		"headers": []map[string]string{
			{"name": "From", "value": m.From}, {"name": "To", "value": m.To},
			{"name": "Subject", "value": m.Subject},
		},
	}
	if bodyType == "text/html" {
		obj["htmlBody"] = []any{part}
		obj["textBody"] = []any{}
	} else {
		obj["textBody"] = []any{part}
		obj["htmlBody"] = []any{}
	}
	return obj
}

func preview(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 100 {
		return s[:100]
	}
	return s
}

func (s *Server) emailGet(u *model.User, acct string, args json.RawMessage) (any, error) {
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
		list = append(list, s.emailObject(&m))
	}
	return map[string]any{"accountId": acct, "state": "1", "list": list, "notFound": notFound}, nil
}

func (s *Server) emailQuery(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		Filter   map[string]any   `json:"filter"`
		Sort     []map[string]any `json:"sort"`
		Position int              `json:"position"`
		Limit    int              `json:"limit"`
	}
	_ = json.Unmarshal(args, &a)
	if a.Limit <= 0 || a.Limit > 500 {
		a.Limit = 50
	}
	tx := s.DB.Where("user_id = ?", u.ID)
	f := a.Filter
	if mb, ok := f["inMailbox"].(string); ok && mb != "" {
		tx = tx.Where("folder = ?", mb)
	}
	if txt, ok := f["text"].(string); ok && txt != "" {
		like := "%" + txt + "%"
		tx = tx.Where("subject LIKE ? OR \"from\" LIKE ? OR \"to\" LIKE ? OR body LIKE ?", like, like, like, like)
	}
	if v, ok := f["subject"].(string); ok && v != "" {
		tx = tx.Where("subject LIKE ?", "%"+v+"%")
	}
	if v, ok := f["from"].(string); ok && v != "" {
		tx = tx.Where("\"from\" LIKE ?", "%"+v+"%")
	}
	if v, ok := f["to"].(string); ok && v != "" {
		tx = tx.Where("\"to\" LIKE ?", "%"+v+"%")
	}
	if hk, ok := f["hasKeyword"].(string); ok {
		switch hk {
		case "$seen":
			tx = tx.Where(map[string]any{"read": true})
		case "$flagged":
			tx = tx.Where(map[string]any{"starred": true})
		}
	}
	if nk, ok := f["notKeyword"].(string); ok {
		switch nk {
		case "$seen":
			tx = tx.Where(map[string]any{"read": false})
		case "$flagged":
			tx = tx.Where(map[string]any{"starred": false})
		}
	}
	order := "id DESC"
	for _, srt := range a.Sort {
		if p, _ := srt["property"].(string); p == "subject" {
			order = "subject ASC"
		} else if p == "from" {
			order = "\"from\" ASC"
		} else if p == "size" {
			order = "length(body) DESC"
		}
		if asc, ok := srt["isAscending"].(bool); ok && !asc && order != "id DESC" {
			order = strings.Replace(order, " ASC", " DESC", 1)
		}
	}
	var total int64
	tx.Model(&model.Mail{}).Count(&total)
	var ms []model.Mail
	tx.Order(order).Offset(a.Position).Limit(a.Limit).Find(&ms)
	ids := make([]string, 0, len(ms))
	for _, m := range ms {
		ids = append(ids, strID(m.ID))
	}
	return map[string]any{"accountId": acct, "queryState": "1", "canCalculateChanges": false,
		"position": a.Position, "ids": ids, "total": total}, nil
}

func (s *Server) emailSet(u *model.User, acct string, args json.RawMessage, createdIds map[string]string) (any, error) {
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
		m := s.buildMail(u, obj)
		if err := s.DB.Create(&m).Error; err != nil {
			continue
		}
		createdIds[cid] = strID(m.ID)
		created[cid] = map[string]any{"id": strID(m.ID), "blobId": "m" + strID(m.ID), "threadId": strID(m.ID), "size": len(m.Body)}
	}
	for id, patch := range a.Update {
		id = resolveIDs([]string{id}, createdIds)[0]
		upd := map[string]any{}
		if kw, ok := patch["keywords"].(map[string]any); ok {
			if v, ok := kw["$seen"].(bool); ok {
				upd["read"] = v
			}
			if v, ok := kw["$flagged"].(bool); ok {
				upd["starred"] = v
			}
		}
		if mb, ok := patch["mailboxIds"].(map[string]any); ok {
			for k, v := range mb {
				if b, _ := v.(bool); b {
					upd["folder"] = k
					break
				}
			}
		}
		if len(upd) > 0 {
			if res := s.DB.Model(&model.Mail{}).Where("id = ? AND user_id = ?", id, u.ID).Updates(upd); res.RowsAffected == 0 {
				notFound = append(notFound, id)
				continue
			}
		}
		updated[id] = nil
	}
	for _, id := range a.Destroy {
		id = resolveIDs([]string{id}, createdIds)[0]
		var m model.Mail
		if err := s.DB.Where("id = ? AND user_id = ?", id, u.ID).First(&m).Error; err != nil {
			notFound = append(notFound, id)
			continue
		}
		s.DB.Model(&m).Update("folder", "trash")
		destroyed = append(destroyed, id)
	}
	return map[string]any{"accountId": acct, "oldState": "1", "newState": "1",
		"created": created, "updated": updated, "destroyed": destroyed,
		"notCreated": nil, "notUpdated": nil, "notDestroyed": nil, "notFound": notFound}, nil
}

// buildMail 从 JMAP Email 对象构建待存邮件。
func (s *Server) buildMail(u *model.User, obj map[string]any) model.Mail {
	folder := "draft"
	if mb, ok := obj["mailboxIds"].(map[string]any); ok {
		for k, v := range mb {
			if b, _ := v.(bool); b {
				folder = k
				break
			}
		}
	}
	subject, _ := obj["subject"].(string)
	from := u.Email
	if fl, ok := obj["from"].([]any); ok && len(fl) > 0 {
		if m0, ok := fl[0].(map[string]any); ok {
			if e, ok := m0["email"].(string); ok && e != "" {
				from = e
			}
		}
	}
	joinAddrs := func(v any) string {
		arr, _ := v.([]any)
		var out []string
		for _, x := range arr {
			if m, ok := x.(map[string]any); ok {
				if e, ok := m["email"].(string); ok {
					out = append(out, e)
				}
			}
		}
		return strings.Join(out, ", ")
	}
	body := ""
	if bv, ok := obj["bodyValues"].(map[string]any); ok {
		for _, v := range bv {
			if m, ok := v.(map[string]any); ok {
				if s, ok := m["value"].(string); ok {
					body = s
				}
			}
		}
	}
	read := true
	if kw, ok := obj["keywords"].(map[string]any); ok {
		if v, ok := kw["$seen"].(bool); ok {
			read = v
		}
	}
	m := model.Mail{UserID: u.ID, From: from, To: joinAddrs(obj["to"]), Cc: joinAddrs(obj["cc"]),
		Bcc: joinAddrs(obj["bcc"]), Subject: subject, Body: body, Folder: folder, Read: read}
	if folder == "sent" {
		m.Status = "queued"
	}
	return m
}

func (s *Server) emailImport(u *model.User, acct string, args json.RawMessage) (any, error) {
	var a struct {
		Emails map[string]struct {
			BlobID     string          `json:"blobId"`
			MailboxIDs map[string]bool `json:"mailboxIds"`
			Keywords   map[string]bool `json:"keywords"`
		} `json:"emails"`
	}
	_ = json.Unmarshal(args, &a)
	created := map[string]any{}
	notCreated := map[string]any{}
	for cid, em := range a.Emails {
		raw, err := s.readBlob(em.BlobID)
		if err != nil {
			notCreated[cid] = map[string]any{"type": "blobNotFound"}
			continue
		}
		subject, body, htmlBody, atts := message.ParseInbound(string(raw))
		folder := "inbox"
		for k, v := range em.MailboxIDs {
			if v {
				folder = k
				break
			}
		}
		m := model.Mail{UserID: u.ID, To: u.Email, Subject: subject, Body: body, BodyHTML: htmlBody,
			Attachments: atts, Folder: folder, Read: em.Keywords["$seen"], Starred: em.Keywords["$flagged"]}
		if err := s.DB.Create(&m).Error; err != nil {
			notCreated[cid] = map[string]any{"type": "serverFail"}
			continue
		}
		created[cid] = map[string]any{"id": strID(m.ID), "blobId": "m" + strID(m.ID), "threadId": strID(m.ID), "size": len(raw)}
	}
	return map[string]any{"accountId": acct, "oldState": "1", "newState": "1",
		"created": created, "notCreated": notCreated}, nil
}

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

// ---------- Blob ----------

func (s *Server) readBlob(blobID string) ([]byte, error) {
	switch {
	case strings.HasPrefix(blobID, "m"):
		var m model.Mail
		if err := s.DB.First(&m, strings.TrimPrefix(blobID, "m")).Error; err != nil {
			return nil, err
		}
		return message.Build("mailserver", &m), nil
	case strings.HasPrefix(blobID, "a"):
		rest := strings.TrimPrefix(blobID, "a")
		dash := strings.LastIndex(rest, "-")
		if dash < 0 {
			return nil, fmt.Errorf("bad blob id")
		}
		mailID, idx := rest[:dash], rest[dash+1:]
		var m model.Mail
		if err := s.DB.First(&m, mailID).Error; err != nil {
			return nil, err
		}
		atts := message.ParseAttachments(m.Attachments)
		i, _ := strconv.Atoi(idx)
		if i < 0 || i >= len(atts) {
			return nil, fmt.Errorf("attachment not found")
		}
		return base64.StdEncoding.DecodeString(atts[i].Data)
	case strings.HasPrefix(blobID, "u"):
		return s.readUpload(blobID)
	}
	return nil, fmt.Errorf("unknown blob")
}
