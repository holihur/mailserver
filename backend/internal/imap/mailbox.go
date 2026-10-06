package imap

import (
	"strings"

	"mailserver/internal/model"
)

func (s *session) loadBox() {
	var mails []model.Mail
	s.db.Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Order("id").Limit(maxSnap).Find(&mails)
	s.items = s.items[:0]
	for _, m := range mails {
		it := item{id: uint32(m.ID), date: m.CreatedAt, from: m.From, to: m.To, cc: m.Cc,
			subject: m.Subject, body: m.Body, attachments: m.Attachments, read: m.Read, starred: m.Starred}
		it.size = len(it.raw(s.host()))
		s.items = append(s.items, it)
	}
}

func (s *session) uidNext() uint32 { return s.maxUID() + 1 }
func (s *session) maxUID() uint32 {
	var m uint32
	for _, it := range s.items {
		if it.id > m {
			m = it.id
		}
	}
	if m == 0 && s.user != nil && s.folder != "" {
		row := struct{ M uint32 }{0}
		s.db.Model(&model.Mail{}).Select("COALESCE(MAX(id),0) AS m").
			Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Scan(&row)
		m = row.M
	}
	return m
}

func (s *session) uidsToSeqs(uids []uint32) []int {
	var out []int
	for _, u := range uids {
		for i, it := range s.items {
			if it.id == u && !it.deleted {
				out = append(out, i+1)
				break
			}
		}
	}
	return out
}

func flagsOf(it *item) string {
	var f []string
	if it.read {
		f = append(f, "\\Seen")
	}
	if it.starred {
		f = append(f, "\\Flagged")
	}
	if it.deleted {
		f = append(f, "\\Deleted")
	}
	return strings.Join(f, " ")
}
