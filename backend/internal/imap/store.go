package imap

import (
	"strconv"
	"strings"
	"time"

	"mailserver/internal/model"
	"mailserver/internal/quota"
)

// ---- STORE ----

func (s *session) storeSeq(rest string) bool {
	set, flagstr, silent := splitStore(rest)
	seqs := expandSeq(set, len(s.items))
	return s.applyStoreSeqs(seqs, flagstr, silent)
}

func (s *session) storeUID(rest string) bool {
	set, flagstr, silent := splitStore(rest)
	uids := expandUID(set, s.maxUID())
	return s.applyStoreSeqs(s.uidsToSeqs(uids), flagstr, silent)
}

func splitStore(rest string) (string, string, bool) {
	f := strings.Fields(rest)
	if len(f) < 2 {
		return "", "", false
	}
	set := f[0]
	op := strings.ToUpper(f[1])
	silent := strings.HasSuffix(op, ".SILENT")
	op = strings.TrimSuffix(op, ".SILENT")
	flags := strings.TrimSpace(rest[strings.Index(rest, f[1])+len(f[1]):])
	flags = strings.Trim(flags, "()")
	return set, op + " " + flags, silent
}

func (s *session) applyStoreSeqs(seqs []int, flagstr string, silent bool) bool {
	if s.readonly {
		return false
	}
	op := "replace"
	fu := strings.ToUpper(flagstr)
	if strings.HasPrefix(fu, "+FLAGS") {
		op = "add"
	} else if strings.HasPrefix(fu, "-FLAGS") {
		op = "del"
	}
	want := map[string]bool{}
	for _, f := range strings.Fields(flagstr) {
		f = strings.ToUpper(strings.Trim(f, "()"))
		want[f] = true
	}
	for _, seq := range seqs {
		if seq < 1 || seq > len(s.items) {
			continue
		}
		it := &s.items[seq-1]
		if it.deleted && !want["\\DELETED"] && op != "del" {
			continue
		}
		seen := it.read
		flag := it.starred
		del := it.deleted
		set := func(name string, v bool) {
			switch name {
			case "\\SEEN":
				seen = v
			case "\\FLAGGED":
				flag = v
			case "\\DELETED":
				del = v
			}
		}
		switch op {
		case "add":
			for f := range want {
				set(f, true)
			}
		case "del":
			for f := range want {
				set(f, false)
			}
		default:
			seen, flag, del = false, false, false
			for f := range want {
				set(f, true)
			}
		}
		it.read, it.starred, it.deleted = seen, flag, del
		s.db.Model(&model.Mail{}).Where("id = ?", it.id).Updates(map[string]any{"read": seen, "starred": flag})
		if !silent {
			s.untagged("%d FETCH (UID %d FLAGS (%s))", seq, it.id, flagsOf(it))
		}
	}
	return true
}

// ---- EXPUNGE / COPY / APPEND ----

func (s *session) expunge(report bool) {
	seq := 0
	kept := s.items[:0]
	for _, it := range s.items {
		seq++
		if it.deleted {
			if report {
				s.untagged("%d EXPUNGE", seq)
				seq--
			}
			s.db.Model(&model.Mail{}).Where("id = ?", it.id).Update("folder", "trash")
			continue
		}
		kept = append(kept, it)
	}
	s.items = kept
}

func (s *session) copySeq(rest string, uidMode bool) ([]uint32, []uint32, bool) {
	i := strings.LastIndex(rest, " ")
	if i < 0 {
		return nil, nil, false
	}
	set, mbox := strings.TrimSpace(rest[:i]), strings.Trim(strings.TrimSpace(rest[i+1:]), "\"")
	_, folder, good := toFolder(mbox)
	if !good {
		return nil, nil, false
	}
	var seqs []int
	if uidMode {
		seqs = s.uidsToSeqs(expandUID(set, s.maxUID()))
	} else {
		seqs = expandSeq(set, len(s.items))
	}
	var srcUIDs, dstUIDs []uint32
	for _, seq := range seqs {
		if seq < 1 || seq > len(s.items) {
			continue
		}
		it := s.items[seq-1]
		nm := model.Mail{UserID: s.user.ID, From: it.from, To: it.to,
			Subject: it.subject, Body: it.body, Folder: folder, Read: it.read, Starred: it.starred}
		s.db.Create(&nm)
		srcUIDs = append(srcUIDs, it.id)
		dstUIDs = append(dstUIDs, uint32(nm.ID))
	}
	return srcUIDs, dstUIDs, true
}

// moveSeq 实现 MOVE（RFC 6851）：直接改 folder，向客户端发 EXPUNGE，返回源/目标 UID。
func (s *session) moveSeq(rest string, uidMode bool) ([]uint32, []uint32, bool) {
	i := strings.LastIndex(rest, " ")
	if i < 0 {
		return nil, nil, false
	}
	set, mbox := strings.TrimSpace(rest[:i]), strings.Trim(strings.TrimSpace(rest[i+1:]), "\"")
	_, folder, good := toFolder(mbox)
	if !good {
		return nil, nil, false
	}
	var seqs []int
	if uidMode {
		seqs = s.uidsToSeqs(expandUID(set, s.maxUID()))
	} else {
		seqs = expandSeq(set, len(s.items))
	}
	moved := map[uint32]bool{}
	var srcUIDs, dstUIDs []uint32
	for _, seq := range seqs {
		if seq < 1 || seq > len(s.items) {
			continue
		}
		it := s.items[seq-1]
		s.db.Model(&model.Mail{}).Where("id = ?", it.id).Update("folder", folder)
		moved[it.id] = true
		srcUIDs = append(srcUIDs, it.id)
		dstUIDs = append(dstUIDs, it.id)
	}
	var removed []int
	kept := s.items[:0:0]
	for idx, it := range s.items {
		if moved[it.id] {
			removed = append(removed, idx+1)
		} else {
			kept = append(kept, it)
		}
	}
	for j := len(removed) - 1; j >= 0; j-- {
		s.untagged("%d EXPUNGE", removed[j])
	}
	s.items = kept
	s.untagged("%d EXISTS", len(s.items))
	s.w.Flush()
	return srcUIDs, dstUIDs, true
}

// quota 回复 QUOTA "" (STORAGE used limit)，单位 KB（0=不限）。
func (s *session) quota() {
	used := quota.Usage(s.db, s.user.ID) / 1024
	limit := int64(s.user.QuotaMB) * 1024
	s.untagged("QUOTA \"\" (STORAGE %d %d)", used, limit)
	s.w.Flush()
}

// idle 实现真 IDLE：阻塞等待 DONE，同时每 3s 轮询邮箱变化并推送 EXISTS。
func (s *session) idle() {
	if s.conn == nil {
		return
	}
	base := len(s.items)
	deadline := time.Now().Add(29 * time.Minute)
	for time.Now().Before(deadline) {
		_ = s.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		line, err := s.r.ReadString('\n')
		if err == nil {
			_ = strings.TrimSpace(line) // 客户端应回 DONE；其它输入也结束 IDLE
			break
		}
		_ = s.conn.SetReadDeadline(time.Time{})
		if s.mbox != "" {
			var n int64
			s.db.Model(&model.Mail{}).Where("user_id = ? AND folder = ?", s.user.ID, s.folder).Count(&n)
			if int(n) != base {
				s.untagged("%d EXISTS", n)
				s.w.Flush()
				base = int(n)
			}
		}
	}
	_ = s.conn.SetReadDeadline(time.Time{})
}

// APPEND mbox [flags] {n[+]}
func (s *session) doAppend(rest string) (uint, bool) {
	lb := strings.LastIndex(rest, "{")
	rb := strings.LastIndex(rest, "}")
	if lb < 0 || rb < 0 || rb < lb {
		return 0, false
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(rest[lb+1:rb], "+"))
	if n <= 0 || n > 2<<20 {
		return 0, false
	}
	mbox := strings.TrimSpace(rest[:lb])
	flags := ""
	if fi := strings.Index(mbox, "("); fi >= 0 {
		fe := strings.Index(mbox, ")")
		if fe > fi {
			flags = mbox[fi+1 : fe]
			mbox = strings.TrimSpace(mbox[:fi])
		}
	}
	_, folder, good := toFolder(strings.Trim(mbox, "\""))
	if !good {
		return 0, false
	}
	literalPlus := strings.HasSuffix(rest[lb+1:rb], "+")
	if !literalPlus {
		s.w.WriteString("+ ready\r\n")
		s.w.Flush()
	}
	buf := make([]byte, n)
	read := 0
	for read < n {
		k, err := s.r.Read(buf[read:])
		if err != nil || k <= 0 {
			return 0, false
		}
		read += k
	}
	// 吃掉结尾 CRLF
	s.r.ReadString('\n')
	raw := string(buf)
	subject, body := splitRaw(raw)
	from := headField(raw, "from")
	if from == "" {
		from = s.user.Email
	}
	to := headField(raw, "to")
	m := model.Mail{UserID: s.user.ID, From: from, To: to, Subject: subject, Body: body, Folder: folder}
	fu := strings.ToUpper(flags)
	if strings.Contains(fu, "\\SEEN") {
		m.Read = true
	}
	if strings.Contains(fu, "\\FLAGGED") {
		m.Starred = true
	}
	s.db.Create(&m)
	if folder == s.folder && s.mbox != "" {
		s.loadBox()
	}
	return m.ID, true
}
