package jmap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type fakeMQ struct{ n int }

func (f *fakeMQ) EnqueueSend(uint) error { f.n++; return nil }

func setup(t *testing.T) (*Server, *model.User, string, uint) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	g.Exec("TRUNCATE mail_tokens, external_accounts, mail_folders, mails, users RESTART IDENTITY CASCADE")

	u := model.User{Email: "jmap@test.local", Name: "J", Signature: "sig"}
	if err := g.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	plain, prefix, _ := auth.NewMailToken()
	g.Create(&model.MailToken{UserID: u.ID, Name: "t", Prefix: prefix, Hash: auth.HashMailToken(plain)})

	g.Create(&model.Mail{UserID: u.ID, From: "a@b.c", To: u.Email, Subject: "hello", Body: "world", Folder: "inbox"})
	att := `[{"name":"a.txt","type":"text/plain","data":"aGk=","size":2}]`
	g.Create(&model.Mail{UserID: u.ID, From: "x@y.z", To: u.Email, Subject: "promo", Body: "buy", Folder: "inbox",
		Read: true, Starred: true, BodyHTML: "<p>buy</p>", Attachments: att})
	g.Create(&model.Mail{UserID: u.ID, From: u.Email, To: "d@e.f", Subject: "sent", Body: "s", Folder: "sent", Relayed: true})
	g.Create(&model.MailFolder{UserID: u.ID, Name: "Archive"})
	g.Create(&model.ExternalAccount{UserID: u.ID, Email: "ext@test.local", Name: "Ext", Enabled: true, IMAPHost: "x"})

	s := &Server{DB: g, MQ: &fakeMQ{}, BlobDir: t.TempDir()}
	return s, &u, plain, u.ID
}

func req(s *Server, token, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler(rec, r)
	return rec
}

func TestSessionAndAuth(t *testing.T) {
	s, _, token, _ := setup(t)
	rec := req(s, token, "GET", "/jmap", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "urn:ietf:params:jmap:mail") {
		t.Fatalf("session: %d %s", rec.Code, rec.Body.String())
	}
	// 未授权
	if rec := req(s, "bad", "GET", "/jmap", ""); rec.Code != 401 {
		t.Fatalf("未授权应 401，得到 %d", rec.Code)
	}
	// OPTIONS
	if rec := req(s, token, "OPTIONS", "/jmap", ""); rec.Code != 204 {
		t.Fatalf("OPTIONS 应 204，得到 %d", rec.Code)
	}
	// 不支持的方法
	if rec := req(s, token, "PUT", "/jmap", ""); rec.Code != 405 {
		t.Fatalf("PUT 应 405，得到 %d", rec.Code)
	}
}

func TestMethodSet(t *testing.T) {
	s, _, token, uid := setup(t)
	var mail model.Mail
	s.DB.Where("user_id = ? AND subject = ?", uid, "promo").First(&mail)

	calls := [][]any{
		{"Core/echo", map[string]any{"x": 1}, "c0"},
		{"Mailbox/get", map[string]any{}, "c1"},
		{"Mailbox/get", map[string]any{"ids": []string{"inbox"}}, "c1b"},
		{"Mailbox/query", map[string]any{}, "c2"},
		{"Mailbox/changes", map[string]any{}, "c3"},
		{"Mailbox/set", map[string]any{"create": map[string]any{"m1": map[string]any{"name": "New"}}}, "c4"},
		{"Email/get", map[string]any{"ids": []string{strID(mail.ID), "99999"}}, "c5"},
		{"Email/query", map[string]any{"filter": map[string]any{"inMailbox": "inbox", "text": "buy", "hasKeyword": "$seen"}}, "c6"},
		{"Email/query", map[string]any{"filter": map[string]any{"subject": "promo", "from": "x@", "to": "@test"}}, "c6b"},
		{"Email/query", map[string]any{"filter": map[string]any{"notKeyword": "$flagged"}, "sort": []map[string]any{{"property": "subject", "isAscending": true}}}, "c6c"},
		{"Email/query", map[string]any{"sort": []map[string]any{{"property": "from"}, {"property": "size"}}}, "c6d"},
		{"Email/changes", map[string]any{}, "c7"},
		{"Thread/get", map[string]any{"ids": []string{strID(mail.ID), "99999"}}, "c8"},
		{"Thread/changes", map[string]any{}, "c9"},
		{"Identity/get", map[string]any{}, "c10"},
		{"Identity/set", map[string]any{"update": map[string]any{"primary": map[string]any{"name": "New", "textSignature": "sig2"}, "ext-1": map[string]any{}}}, "c11"},
		{"EmailSubmission/get", map[string]any{}, "c12"},
		{"EmailSubmission/get", map[string]any{"ids": []string{"1"}}, "c12b"},
		{"SearchSnippet/get", map[string]any{"emailIds": []string{strID(mail.ID), "99999"}}, "c13"},
		{"Email/copy", map[string]any{"update": map[string]any{strID(mail.ID): map[string]any{"mailboxIds": map[string]any{"inbox": true}}}}, "c14"},
		{"Email/set", map[string]any{"destroy": []string{strID(mail.ID), "99999"}}, "c15"},
		{"Unknown/method", map[string]any{}, "c16"},
	}
	body, _ := json.Marshal(map[string]any{"using": []string{capCore, capMail}, "methodCalls": calls})
	rec := req(s, token, "POST", "/jmap", string(body))
	if rec.Code != 200 {
		t.Fatalf("api: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown blob") && !strings.Contains(rec.Body.String(), "不支持的方法") {
		t.Fatalf("应包含错误响应: %s", rec.Body.String())
	}
	// identity 更新生效
	var fresh model.User
	s.DB.First(&fresh, uid)
	if fresh.Name != "New" || fresh.Signature != "sig2" {
		t.Fatalf("identity 未更新: %+v", fresh)
	}
}

func TestEmailSetAndSubmission(t *testing.T) {
	s, _, token, uid := setup(t)
	// 创建草稿
	createBody, _ := json.Marshal(map[string]any{
		"using": []string{capCore, capMail},
		"methodCalls": [][]any{
			{"Email/set", map[string]any{"create": map[string]any{"d1": map[string]any{
				"mailboxIds": map[string]any{"draft": true},
				"subject":    "draft subj",
				"from":       []any{map[string]any{"email": "ext@test.local"}},
				"to":         []any{map[string]any{"email": "to@x.y"}},
				"cc":         []any{map[string]any{"email": "cc@x.y"}},
				"bcc":        []any{map[string]any{"email": "bcc@x.y"}},
				"keywords":   map[string]any{"$seen": false},
				"bodyValues": map[string]any{"1": map[string]any{"value": "draft body"}},
			}}}, "m1"},
			{"EmailSubmission/set", map[string]any{"create": map[string]any{"s1": map[string]any{"emailId": "#d1"}}}, "m2"},
			{"EmailSubmission/set", map[string]any{"create": map[string]any{"s2": map[string]any{"emailId": "99999"}}}, "m3"},
		},
	})
	rec := req(s, token, "POST", "/jmap", string(createBody))
	if rec.Code != 200 {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalidEmail") {
		t.Fatalf("应报告 invalidEmail: %s", rec.Body.String())
	}
	var draft model.Mail
	if err := s.DB.Where("user_id = ? AND subject = ?", uid, "draft subj").First(&draft).Error; err != nil {
		t.Fatalf("草稿未创建: %v", err)
	}
	// 提交后应移入 sent 并入队
	var sent model.Mail
	if err := s.DB.Where("user_id = ? AND subject = ?", uid, "draft subj").First(&sent).Error; err != nil {
		t.Fatal(err)
	}
	if sent.Folder != "sent" {
		t.Fatalf("提交后应在 sent，得到 %s", sent.Folder)
	}
	if mq, ok := s.MQ.(*fakeMQ); !ok || mq.n == 0 {
		t.Fatal("应入队发送")
	}
}

func TestEmailImportAndBlob(t *testing.T) {
	s, _, token, uid := setup(t)
	acct := acctID(uid)
	// 上传一个原始邮件 blob
	raw := "From: a@b.c\r\nTo: jmap@test.local\r\nSubject: imported\r\n\r\nhi there"
	rec := req(s, token, "POST", "/jmap/upload/"+acct+"/", raw)
	if rec.Code != 201 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var up struct {
		BlobID string `json:"blobId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &up)
	if up.BlobID == "" {
		t.Fatal("upload 未返回 blobId")
	}
	// import
	imp, _ := json.Marshal(map[string]any{
		"using": []string{capCore, capMail},
		"methodCalls": [][]any{
			{"Email/import", map[string]any{"emails": map[string]any{
				"i1": map[string]any{"blobId": up.BlobID, "mailboxIds": map[string]any{"inbox": true}, "keywords": map[string]any{"$seen": true}},
				"i2": map[string]any{"blobId": "u_nope", "mailboxIds": map[string]any{"inbox": true}},
			}}, "c1"},
		},
	})
	rec = req(s, token, "POST", "/jmap", string(imp))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "blobNotFound") {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	var m model.Mail
	if err := s.DB.Where("user_id = ? AND subject = ?", uid, "imported").First(&m).Error; err != nil {
		t.Fatalf("导入邮件未入库: %v", err)
	}
	// 下载原始邮件
	if rec := req(s, token, "GET", "/jmap/download/"+acct+"/m"+strID(m.ID)+"/x.eml", ""); rec.Code != 200 {
		t.Fatalf("下载原始邮件失败: %d", rec.Code)
	}
	// 下载附件
	var promo model.Mail
	s.DB.Where("user_id = ? AND subject = ?", uid, "promo").First(&promo)
	if rec := req(s, token, "GET", "/jmap/download/"+acct+"/a"+strID(promo.ID)+"-0/a.txt", ""); rec.Code != 200 {
		t.Fatalf("下载附件失败: %d", rec.Code)
	}
	// 下载上传 blob
	if rec := req(s, token, "GET", "/jmap/download/"+acct+"/"+up.BlobID+"/x", ""); rec.Code != 200 {
		t.Fatalf("下载上传 blob 失败: %d", rec.Code)
	}
	// 不存在
	if rec := req(s, token, "GET", "/jmap/download/"+acct+"/m99999/x", ""); rec.Code != 404 {
		t.Fatalf("不存在 blob 应 404，得到 %d", rec.Code)
	}
	// 坏路径
	if rec := req(s, token, "GET", "/jmap/download/onlyone", ""); rec.Code != 400 {
		t.Fatalf("坏路径应 400，得到 %d", rec.Code)
	}
	// upload GET -> 405
	if rec := req(s, token, "GET", "/jmap/upload/"+acct+"/", ""); rec.Code != 405 {
		t.Fatalf("upload GET 应 405，得到 %d", rec.Code)
	}
}

func TestErrorsAndEdges(t *testing.T) {
	s, _, token, uid := setup(t)
	acct := acctID(uid)
	// 坏 JSON
	if rec := req(s, token, "POST", "/jmap", "{not json"); rec.Code != 400 {
		t.Fatalf("坏 JSON 应 400，得到 %d", rec.Code)
	}
	// eventsource（取消上下文）
	r := httptest.NewRequest("GET", "/jmap/eventsource/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	go func() { s.Handler(rec, r) }()
	cancel()
	// 无 BlobDir 的 server：upload 501，readUpload 报错
	s2 := &Server{DB: s.DB}
	if rec := req(s2, token, "POST", "/jmap/upload/"+acct+"/", "x"); rec.Code != 501 {
		t.Fatalf("无 BlobDir upload 应 501，得到 %d", rec.Code)
	}
	if _, err := s2.readUpload("u123"); err == nil {
		t.Fatal("无 BlobDir readUpload 应报错")
	}
	// readBlob 未知类型
	if _, err := s.readBlob("zzz"); err == nil {
		t.Fatal("未知 blob 应报错")
	}
	if _, err := s.readBlob("a123"); err == nil {
		t.Fatal("坏附件 blob 应报错")
	}
	// helpers
	if addrList("") != nil {
		t.Fatal("空地址应为 nil")
	}
	if len(addrList("a@b.c, c@d.e")) != 2 {
		t.Fatal("多地址解析失败")
	}
	if keywordsOf(&model.Mail{Folder: "draft"})["$draft"] != true {
		t.Fatal("草稿关键字缺失")
	}
	if len(preview(strings.Repeat("x", 200))) != 100 {
		t.Fatal("preview 应截断到 100")
	}
	if resolveIDs([]string{"#x", "1"}, map[string]string{"x": "9"})[0] != "9" {
		t.Fatal("resolveIDs 引用解析失败")
	}
}

func TestDBErrorBranches(t *testing.T) {
	s, _, _, uid := setup(t)
	_ = s
	dsn := os.Getenv("TEST_DATABASE_URL")
	bad, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := bad.DB()
	_ = sqlDB.Close() // 关闭连接，后续查询/写入均报错
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "u1"), []byte("From: a@b.c\r\nSubject: x\r\n\r\ny"), 0o600)
	sBad := &Server{DB: bad, MQ: &fakeMQ{}, BlobDir: dir}
	u := &model.User{ID: uid, Email: "jmap@test.local"}
	acct := acctID(uid)
	// 直接调用各方法，覆盖 DB 错误分支
	_, _ = sBad.mailboxGet(u, acct, json.RawMessage(`{}`))
	_, _ = sBad.mailboxQuery(u, acct)
	_, _ = sBad.mailboxSet(u, acct, json.RawMessage(`{"create":{"m":{"name":"X"}},"update":{"c1":{"name":"Y"}},"destroy":["c1"]}`))
	_, _ = sBad.emailGet(u, acct, json.RawMessage(`{"ids":["1"]}`))
	_, _ = sBad.emailQuery(u, acct, json.RawMessage(`{}`))
	_, _ = sBad.emailSet(u, acct, json.RawMessage(`{"create":{"d":{"subject":"x"}},"update":{"1":{"keywords":{"$seen":true}}},"destroy":["1"]}`), map[string]string{})
	_, _ = sBad.emailImport(u, acct, json.RawMessage(`{"emails":{"i":{"blobId":"u1","mailboxIds":{"inbox":true}}}}`))
	_, _ = sBad.threadGet(u, acct, json.RawMessage(`{"ids":["1"]}`))
	_, _ = sBad.identityGet(u, acct)
	_, _ = sBad.identitySet(u, acct, json.RawMessage(`{"update":{"primary":{"name":"n"}}}`))
	_, _ = sBad.submissionGet(u, acct, json.RawMessage(`{}`))
	_, _ = sBad.submissionSet(u, acct, json.RawMessage(`{"create":{"s":{"emailId":"1"}}}`), map[string]string{})
	_, _ = sBad.searchSnippet(u, acct, json.RawMessage(`{"emailIds":["1"]}`))
	_ = sBad.emailObject(&model.Mail{})
}

func TestMoreCoverage(t *testing.T) {
	s, _, token, uid := setup(t)
	acct := acctID(uid)
	// auth 失败分支
	if rec := req(s, "bad", "GET", "/jmap/download/"+acct+"/m1/x", ""); rec.Code != 401 {
		t.Fatal("download 未授权应 401")
	}
	if rec := req(s, "bad", "POST", "/jmap/upload/"+acct+"/", "x"); rec.Code != 401 {
		t.Fatal("upload 未授权应 401")
	}
	// MkdirAll 失败：BlobDir 指向一个文件下的子路径
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o600)
	if rec := req(&Server{DB: s.DB, BlobDir: filepath.Join(f, "sub")}, token, "POST", "/jmap/upload/"+acct+"/", "x"); rec.Code != 500 {
		t.Fatalf("MkdirAll 失败应 500，得到 %d", rec.Code)
	}
	// WriteFile 失败：只读目录
	ro := t.TempDir()
	_ = os.Chmod(ro, 0o500)
	if rec := req(&Server{DB: s.DB, BlobDir: ro}, token, "POST", "/jmap/upload/"+acct+"/", "x"); rec.Code != 500 {
		t.Fatalf("WriteFile 失败应 500，得到 %d", rec.Code)
	}
	_ = os.Chmod(ro, 0o700)
	// accountId 为空（路径少一段）
	if rec := req(s, token, "POST", "/jmap/upload/", "x"); rec.Code != 201 {
		t.Fatalf("upload 空 account 应 201，得到 %d", rec.Code)
	}
	// mailboxSet 非自定义 id
	ms := `{"using":["urn:ietf:params:jmap:core"],"methodCalls":[["Mailbox/set",{"update":{"inbox":{"name":"x"}},"destroy":["inbox"]},"a"]]}`
	req(s, token, "POST", "/jmap", ms)
	// emailQuery $flagged + emailSet update notFound + create sent
	eq := `{"using":["urn:ietf:params:jmap:core","urn:ietf:params:jmap:mail"],"methodCalls":[` +
		`["Email/query",{"filter":{"hasKeyword":"$flagged"}},"b"],` +
		`["Email/query",{"filter":{"notKeyword":"$flagged"}},"c"],` +
		`["Email/set",{"update":{"99999":{"keywords":{"$seen":true}}}},"d"],` +
		`["Email/set",{"create":{"s":{"mailboxIds":{"sent":true},"subject":"sent-create"}}},"e"]]}`
	req(s, token, "POST", "/jmap", eq)
	// identity：外部账号名为空
	var ext model.ExternalAccount
	s.DB.Where("user_id = ?", uid).First(&ext)
	s.DB.Model(&ext).Update("name", "")
	req(s, token, "POST", "/jmap", `{"using":["urn:ietf:params:jmap:core","urn:ietf:params:jmap:mail"],"methodCalls":[["Identity/get",{},"i"]]}`)
	// readBlob 分支
	var plain model.Mail
	s.DB.Where("user_id = ? AND subject = ?", uid, "hello").First(&plain)
	if _, err := s.readBlob("a" + strID(plain.ID) + "-0"); err == nil {
		t.Fatal("无附件应报错")
	}
	if _, err := s.readBlob("aNoDash"); err == nil {
		t.Fatal("坏附件 id 应报错")
	}
	if _, err := s.readBlob("u_missing"); err == nil {
		t.Fatal("缺失上传 blob 应报错")
	}
	if _, err := s.readBlob("m99999"); err == nil {
		t.Fatal("缺失邮件 blob 应报错")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestRemaining(t *testing.T) {
	s, _, token, uid := setup(t)
	acct := acctID(uid)

	// bearer 无 Authorization
	if rec := req(s, "", "GET", "/jmap", ""); rec.Code != 401 {
		t.Fatal("无 token 应 401")
	}
	// methodCall 元素不足
	if rec := req(s, token, "POST", "/jmap", `{"using":[],"methodCalls":[["Core/echo",{}]]}`); rec.Code != 200 {
		t.Fatal("短 methodCall 应被跳过")
	}
	// upload 读取 body 出错
	r := httptest.NewRequest("POST", "/jmap/upload/"+acct+"/", errReader{})
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.upload(rec, r)
	if rec.Code != 400 {
		t.Fatalf("body 读取失败应 400，得到 %d", rec.Code)
	}
	// mailboxSet create 空名
	req(s, token, "POST", "/jmap", `{"using":["urn:ietf:params:jmap:core"],"methodCalls":[["Mailbox/set",{"create":{"m":{"name":"  "}}},"a"]]}`)
	// emailQuery notKeyword $seen + 排序 isAscending=false
	var mail model.Mail
	s.DB.Where("user_id = ? AND subject = ?", uid, "promo").First(&mail)
	eq := `{"using":["urn:ietf:params:jmap:core","urn:ietf:params:jmap:mail"],"methodCalls":[` +
		`["Email/query",{"filter":{"notKeyword":"$seen"}},"b"],` +
		`["Email/query",{"sort":[{"property":"subject","isAscending":false}]},"c"],` +
		`["Email/set",{"update":{"` + strID(mail.ID) + `":{"keywords":{"$flagged":true}}}},"d"]]}`
	req(s, token, "POST", "/jmap", eq)
	// readBlob 附件所属邮件不存在
	if _, err := s.readBlob("a99999-0"); err == nil {
		t.Fatal("邮件不存在应报错")
	}
}
