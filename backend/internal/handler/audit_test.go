package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"
)

// #10：管理员写操作落审计（脱敏），下游仍能读到 body。
func TestAuditAdmin(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	g.Exec("TRUNCATE users, audit_logs RESTART IDENTITY CASCADE")
	auth.SetSecret("unit-test-secret-0123456789")
	u := model.User{Email: "admin@test.local", Admin: true}
	g.Create(&u)
	tok, _ := auth.Sign(u.ID, u.Email, 0)

	downstream := false
	h := AuditAdmin(g, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstream = true
		b, _ := io.ReadAll(r.Body)
		if r.Method == "PATCH" && !strings.Contains(string(b), "secret-value") {
			t.Errorf("下游应能读到完整 body，得到 %q", string(b))
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest("PATCH", "/api/admin/users/1", strings.NewReader(`{"password":"secret-value","name":"x"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.RemoteAddr = "203.0.113.9:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !downstream {
		t.Fatal("下游 handler 应被调用")
	}
	var logs []model.AuditLog
	g.Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("应记录 1 条审计，得到 %d", len(logs))
	}
	l := logs[0]
	if l.ActorEmail != "admin@test.local" || l.Action != "PATCH" || l.IP != "203.0.113.9" {
		t.Fatalf("审计字段不正确: %+v", l)
	}
	if strings.Contains(l.Detail, "secret-value") || !strings.Contains(l.Detail, `"password":"***"`) {
		t.Fatalf("密码应脱敏: %s", l.Detail)
	}

	// GET 不记录
	req2 := httptest.NewRequest("GET", "/api/admin/users", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(httptest.NewRecorder(), req2)
	g.Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("GET 不应新增审计，得到 %d", len(logs))
	}
}
