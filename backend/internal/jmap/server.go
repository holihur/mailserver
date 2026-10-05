// Package jmap 实现 JMAP（RFC 8620 核心 / RFC 8621 邮件与提交）的标准方法集：
//   - GET  /jmap 或 /.well-known/jmap -> Session
//   - POST /jmap                      -> API（methodCalls）
//   - GET  /jmap/download/...         -> Blob 下载（原始邮件 / 附件）
//   - POST /jmap/upload/{accountId}/  -> Blob 上传
//
// 鉴权：Authorization: Bearer <PAT>。
// 支持方法：Core/echo、Mailbox/get|query|changes|set、Email/get|query|changes|set|copy|import、
// Thread/get|changes、Identity/get|set、EmailSubmission/get|set、SearchSnippet/get。
// 说明：Push（EventSource）在邮件变更时推送 StateChange（进程内 push 中枢）；
// Email/changes 仍返回 cannotCalculateChanges，客户端收到 state 后做全量同步。
package jmap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/model"
	"mailserver/internal/push"

	"gorm.io/gorm"
)

const (
	capCore       = "urn:ietf:params:jmap:core"
	capMail       = "urn:ietf:params:jmap:mail"
	capSubmission = "urn:ietf:params:jmap:submission"
)

type Server struct {
	DB      *gorm.DB
	MQ      interface{ EnqueueSend(mailID uint) error }
	BlobDir string
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func acctID(uid uint) string { return "u" + strconv.FormatUint(uint64(uid), 10) }

func (s *Server) authUser(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	u, err := auth.AuthenticateMail(s.DB, "", bearer(r), auth.HostOf(r.RemoteAddr), auth.ScopeJMAP)
	if err != nil {
		writeJSON(w, 401, map[string]any{"type": "about:blank", "status": 401, "detail": "unauthorized"})
		return nil, false
	}
	return u, true
}

func (s *Server) Handler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/jmap/download/"):
		s.download(w, r)
		return
	case strings.HasPrefix(p, "/jmap/upload/"):
		s.upload(w, r)
		return
	case strings.HasPrefix(p, "/jmap/eventsource"):
		s.eventSource(w, r)
		return
	}
	u, ok := s.authUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case "GET":
		s.session(w, u)
	case "POST":
		s.api(w, r, u)
	case "OPTIONS":
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

// eventSource 实现 JMAP Push（RFC 8620 §7）：客户端建立 SSE 长连接，
// 邮件变更时推送 state 事件（active 类型支持 closeafter/ping）。
func (s *Server) eventSource(w http.ResponseWriter, r *http.Request) {
	u, ok := s.authUser(w, r)
	if !ok {
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	ping, _ := strconv.Atoi(r.URL.Query().Get("ping"))
	closeafter := strings.ToLower(r.URL.Query().Get("closeafter"))
	acct := acctID(u.ID)

	sendPing := func() {
		_, _ = fmt.Fprint(w, "event: ping\ndata: {\"@type\":\"Ping\"}\n\n")
		fl.Flush()
	}
	sendState := func(state string) {
		data, _ := json.Marshal(map[string]any{
			"@type":   "StateChange",
			"changed": map[string]any{acct: map[string]any{"Email": state}},
		})
		_, _ = fmt.Fprintf(w, "event: state\ndata: %s\n\n", data)
		fl.Flush()
	}
	sendPing() // 建连即发一次，客户端可据此确认连接

	ch := push.Subscribe(u.ID)
	defer push.Unsubscribe(u.ID, ch)

	var pingC <-chan time.Time
	if ping > 0 {
		t := time.NewTicker(time.Duration(ping) * time.Second)
		defer t.Stop()
		pingC = t.C
	}
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pingC:
			sendPing()
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		case state := <-ch:
			sendState(state)
			if closeafter == "state" {
				return
			}
		}
	}
}

func (s *Server) session(w http.ResponseWriter, u *model.User) {
	acct := acctID(u.ID)
	writeJSON(w, 200, map[string]any{
		"capabilities": map[string]any{
			capCore: map[string]any{
				"maxSizeUpload": 50 << 20, "maxConcurrentUpload": 4, "maxSizeRequest": 10 << 20,
				"maxConcurrentRequests": 4, "maxCallsInRequest": 64, "maxObjectsInGet": 500,
				"maxObjectsInSet": 500, "collationAlgorithms": []string{"i;ascii-casemap"},
			},
			capMail: map[string]any{
				"maxMailboxesPerEmail": 10, "maxMailboxDepth": 10, "maxSizeAttachmentsPerEmail": 50 << 20,
				"emailQuerySortOptions":    []string{"receivedAt", "sentAt", "subject", "from", "size"},
				"mayCreateTopLevelMailbox": true,
			},
			capSubmission: map[string]any{"maxDelayedSend": 0},
		},
		"accounts": map[string]any{
			acct: map[string]any{
				"name": u.Email, "isPersonal": true, "isReadOnly": false,
				"accountCapabilities": map[string]any{capMail: map[string]any{}, capSubmission: map[string]any{}},
			},
		},
		"primaryAccounts": map[string]any{capMail: acct, capSubmission: acct},
		"username":        u.Email,
		"apiUrl":          "/jmap",
		"downloadUrl":     "/jmap/download/{accountId}/{blobId}/{name}",
		"uploadUrl":       "/jmap/upload/{accountId}/",
		"eventSourceUrl":  "/jmap/eventsource/?types={types}&closeafter={closeafter}&ping={ping}",
		"state":           "1",
	})
}

type methodCall struct {
	Name   string
	Args   json.RawMessage
	CallID string
}

func (s *Server) api(w http.ResponseWriter, r *http.Request, u *model.User) {
	var req struct {
		Using       []string            `json:"using"`
		MethodCalls [][]json.RawMessage `json:"methodCalls"`
		CreatedIds  map[string]string   `json:"createdIds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"type": "urn:ietf:params:jmap:error:notRequest", "status": 400, "detail": err.Error()})
		return
	}
	if req.CreatedIds == nil {
		req.CreatedIds = map[string]string{}
	}
	acct := acctID(u.ID)
	var responses [][]any
	for _, mc := range req.MethodCalls {
		if len(mc) < 3 {
			continue
		}
		var name, callID string
		_ = json.Unmarshal(mc[0], &name)
		_ = json.Unmarshal(mc[2], &callID)
		args := mc[1]
		result, err := s.dispatch(u, acct, name, args, req.CreatedIds)
		if err != nil {
			responses = append(responses, []any{"error", map[string]any{
				"type": "urn:ietf:params:jmap:error:invalidArguments", "status": 400, "detail": err.Error(),
			}, callID})
			continue
		}
		responses = append(responses, []any{name, result, callID})
	}
	writeJSON(w, 200, map[string]any{"methodResponses": responses, "sessionState": "1", "createdIds": req.CreatedIds})
}

func (s *Server) dispatch(u *model.User, acct, name string, args json.RawMessage, created map[string]string) (any, error) {
	switch name {
	case "Core/echo":
		var m map[string]any
		_ = json.Unmarshal(args, &m)
		return m, nil
	case "Mailbox/get":
		return s.mailboxGet(u, acct, args)
	case "Mailbox/query":
		return s.mailboxQuery(u, acct)
	case "Mailbox/changes":
		return changesResult(acct), nil
	case "Mailbox/set":
		return s.mailboxSet(u, acct, args)
	case "Email/get":
		return s.emailGet(u, acct, args)
	case "Email/query":
		return s.emailQuery(u, acct, args)
	case "Email/changes":
		return changesResult(acct), nil
	case "Email/set":
		return s.emailSet(u, acct, args, created)
	case "Email/copy":
		return s.emailSet(u, acct, args, created)
	case "Email/import":
		return s.emailImport(u, acct, args)
	case "Thread/get":
		return s.threadGet(u, acct, args)
	case "Thread/changes":
		return changesResult(acct), nil
	case "Identity/get":
		return s.identityGet(u, acct)
	case "Identity/set":
		return s.identitySet(u, acct, args)
	case "EmailSubmission/get":
		return s.submissionGet(u, acct, args)
	case "EmailSubmission/set":
		return s.submissionSet(u, acct, args, created)
	case "SearchSnippet/get":
		return s.searchSnippet(u, acct, args)
	}
	return nil, fmt.Errorf("不支持的方法: %s", name)
}

func changesResult(acct string) map[string]any {
	return map[string]any{"accountId": acct, "oldState": "1", "newState": "1", "cannotCalculateChanges": true}
}

// ---------- 公共辅助 ----------

func strID(u uint) string { return strconv.FormatUint(uint64(u), 10) }

func resolveIDs(ids []string, created map[string]string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.HasPrefix(id, "#") {
			if v, ok := created[id[1:]]; ok {
				out = append(out, v)
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
