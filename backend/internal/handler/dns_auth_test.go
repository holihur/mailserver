package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mailserver/internal/auth"
	"mailserver/internal/db"
	"mailserver/internal/model"
)

// setupDNSTest 需要 TEST_DATABASE_URL（与 db 集成测试一致），否则跳过。
func setupDNSTest(t *testing.T) (d *DNS, adminTok, userTok string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过")
	}
	g, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := g.Exec("TRUNCATE users, mails, domains, dns_records, dns_providers, acme_configs, settings RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	auth.SetSecret("test-secret-0123456789abcdef")
	admin := model.User{Email: "admin@test.local", Name: "admin", Admin: true}
	user := model.User{Email: "user@test.local", Name: "user"}
	if err := g.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := g.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	adminTok, _ = auth.Sign(admin.ID, admin.Email, admin.TokenVersion)
	userTok, _ = auth.Sign(user.ID, user.Email, user.TokenVersion)
	d = &DNS{DB: g, ZonesPath: filepath.Join(t.TempDir(), "zones.json")}
	return d, adminTok, userTok
}

func doReq(h http.HandlerFunc, method, path, body, token string) *httptest.ResponseRecorder {
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
	h(rec, r)
	return rec
}

// 非管理员不能读取/创建/删除自托管 DNS。
func TestDNSForbiddenForNonAdmin(t *testing.T) {
	d, _, userTok := setupDNSTest(t)

	if rec := doReq(d.Domains, "GET", "/api/domains", "", userTok); rec.Code != 403 {
		t.Fatalf("非管理员 GET /api/domains 应 403，得到 %d", rec.Code)
	}
	if rec := doReq(d.Domains, "POST", "/api/domains", `{"name":"evil.com","ip":"1.2.3.4"}`, userTok); rec.Code != 403 {
		t.Fatalf("非管理员 POST /api/domains 应 403，得到 %d", rec.Code)
	}
	if rec := doReq(d.DKIM, "GET", "/api/dkim", "", userTok); rec.Code != 403 {
		t.Fatalf("非管理员 GET /api/dkim 应 403，得到 %d", rec.Code)
	}
	if rec := doReq(d.DKIM, "POST", "/api/dkim", `{"domain":"evil.com"}`, userTok); rec.Code != 403 {
		t.Fatalf("非管理员 POST /api/dkim 应 403，得到 %d", rec.Code)
	}

	// 无 token 应 401
	if rec := doReq(d.Domains, "GET", "/api/domains", "", ""); rec.Code != 401 {
		t.Fatalf("无 token 应 401，得到 %d", rec.Code)
	}

	// 确认非管理员的操作没有落库
	var n int64
	d.DB.Model(&model.Domain{}).Count(&n)
	if n != 0 {
		t.Fatalf("非管理员不应创建域名，当前 %d 条", n)
	}
}

// 管理员可以正常管理自托管 DNS。
func TestDNSAllowedForAdmin(t *testing.T) {
	d, adminTok, _ := setupDNSTest(t)

	rec := doReq(d.Domains, "POST", "/api/domains", `{"name":"Example.com","ip":"1.2.3.4"}`, adminTok)
	if rec.Code != 201 {
		t.Fatalf("管理员 POST /api/domains 应 201，得到 %d: %s", rec.Code, rec.Body.String())
	}
	var dm model.Domain
	if err := d.DB.Where("name = ?", "example.com").First(&dm).Error; err != nil {
		t.Fatalf("域名未落库: %v", err)
	}

	if rec := doReq(d.Domains, "GET", "/api/domains", "", adminTok); rec.Code != 200 {
		t.Fatalf("管理员 GET /api/domains 应 200，得到 %d", rec.Code)
	}
	if rec := doReq(d.DomainOne, "GET", "/api/domains/1", "", adminTok); rec.Code != 200 {
		t.Fatalf("管理员 GET /api/domains/1 应 200，得到 %d", rec.Code)
	}
	if rec := doReq(d.DomainOne, "POST", "/api/domains/1/records", `{"name":"www","type":"A","value":"5.6.7.8"}`, adminTok); rec.Code != 201 {
		t.Fatalf("管理员新增记录应 201，得到 %d", rec.Code)
	}

	// 删域名
	if rec := doReq(d.DomainOne, "DELETE", "/api/domains/1", "", adminTok); rec.Code != 200 {
		t.Fatalf("管理员 DELETE /api/domains/1 应 200，得到 %d", rec.Code)
	}
	var n int64
	d.DB.Model(&model.Domain{}).Count(&n)
	if n != 0 {
		t.Fatalf("域名应已删除，剩余 %d 条", n)
	}
}
