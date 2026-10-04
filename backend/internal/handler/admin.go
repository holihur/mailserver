package handler

// 域名邮箱管理后台（仅管理员）：账号开/关/删/改密、域名成员概览、系统状态。
// 路由见 main.go：/api/admin/overview|users|users/:id|domains

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/certstore"
	"mailserver/internal/health"
	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/selfupdate"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Admin struct {
	DB          *gorm.DB
	AdminEmails string // ADMIN_EMAILS 逗号分隔，命中即管理员（兜底提权）
	DNS         *DNS   // 复用自托管 DNS 的导出/域名落库能力（可为 nil）
	RT          *runtimecfg.Store
	Cert        *certstore.Store // 动态 TLS 证书
	CertDir     string           // 证书 / ACME 账号缓存目录
	Version     string           // 当前版本（注入自 main）
	Commit      string
	Date        string
	Repo        string // GitHub owner/repo（MAILSERVER_REPO，空则用默认）
	Health      *health.Collector
}

// effectiveAdminEmails 优先使用后台配置，回退环境变量。
func effectiveAdminEmails(rt *runtimecfg.Store, fallback string) string {
	if rt != nil {
		if v := rt.AdminEmails(); v != "" {
			return v
		}
	}
	return fallback
}

// isAdminEmail 命中预置管理员名单（大小写不敏感）
func isAdminEmail(list, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range strings.Split(list, ",") {
		if strings.ToLower(strings.TrimSpace(e)) == email {
			return true
		}
	}
	return false
}

// resolveAdmin: token 有效 + 账号未禁用 + 是管理员。Admin/DNS 等控制面共用。
func resolveAdmin(db *gorm.DB, rt *runtimecfg.Store, adminEmails string, w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	uid, ver, err := auth.Access(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	var u model.User
	if err := db.First(&u, uid).Error; err != nil || u.Disabled || u.TokenVersion != ver {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	// 名单命中顺手提权（改了环境变量无需手动进库）
	if !u.Admin && isAdminEmail(effectiveAdminEmails(rt, adminEmails), u.Email) {
		u.Admin = true
		db.Model(&u).Update("admin", true)
	}
	if !u.Admin {
		writeJSON(w, 403, map[string]string{"error": "需要管理员权限"})
		return nil, false
	}
	return &u, true
}

// mustAdmin: Admin 控制面入口。
func (a *Admin) mustAdmin(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	return resolveAdmin(a.DB, a.RT, a.AdminEmails, w, r)
}

// GET /api/admin/overview {users, domains, mails, pending, storage_bytes}
func (a *Admin) Overview(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	var users, domains, mails, pending, storage int64
	a.DB.Model(&model.User{}).Count(&users)
	a.DB.Model(&model.Domain{}).Count(&domains)
	a.DB.Model(&model.Mail{}).Count(&mails)
	a.DB.Model(&model.Mail{}).Where("folder = ? AND relayed = ?", "sent", false).Count(&pending)
	// 跨数据库（SQLite/PG）均可用；按字符计，仅作展示
	a.DB.Model(&model.Mail{}).
		Select("COALESCE(SUM(LENGTH(subject)+LENGTH(body)),0)").
		Scan(&storage)
	writeJSON(w, 200, map[string]any{
		"users": users, "domains": domains, "mails": mails,
		"pending": pending, "storage_bytes": storage,
	})
}

type adminUser struct {
	model.User
	MailCount int64 `json:"mail_count"`
}

// GET /api/admin/users（带各账号邮件数）  POST /api/admin/users（新建邮箱账号）
func (a *Admin) Users(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method == "POST" {
		a.createUser(w, r)
		return
	}
	var users []model.User
	a.DB.Order("id ASC").Find(&users)
	type cnt struct {
		UserID uint
		N      int64
	}
	var counts []cnt
	a.DB.Model(&model.Mail{}).Select("user_id, COUNT(*) AS n").Group("user_id").Scan(&counts)
	m := make(map[uint]int64, len(counts))
	for _, c := range counts {
		m[c.UserID] = c.N
	}
	out := make([]adminUser, 0, len(users))
	for _, u := range users {
		out = append(out, adminUser{User: u, MailCount: m[u.ID]})
	}
	writeJSON(w, 200, out)
}

// POST /api/admin/users {email, name, password} -> 在托管域名下新建邮箱账号
func (a *Admin) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Pass    string `json:"password"`
		QuotaMB int    `json:"quota_mb"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		writeJSON(w, 400, map[string]string{"error": "邮箱格式不正确"})
		return
	}
	if err := validatePassword(in.Pass, email); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	// 邮箱域名必须已托管，否则建了也收不到信
	var dm model.Domain
	if err := a.DB.Where("LOWER(name) = ?", email[at+1:]).First(&dm).Error; err != nil {
		writeJSON(w, 400, map[string]string{"error": "域名未托管：先去「域名 DNS」页添加 " + email[at+1:]})
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = email[:at]
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Pass), bcrypt.DefaultCost)
	u := model.User{Email: email, Name: name, PassHash: string(hash), QuotaMB: in.QuotaMB}
	if isAdminEmail(effectiveAdminEmails(a.RT, a.AdminEmails), email) {
		u.Admin = true
	}
	if err := a.DB.Create(&u).Error; err != nil {
		writeJSON(w, 409, map[string]string{"error": "邮箱已存在"})
		return
	}
	writeJSON(w, 201, u)
}

// PATCH /api/admin/users/:id {name?, password?, disabled?, admin?}
// DELETE /api/admin/users/:id（删账号及其全部邮件）
func (a *Admin) UserOne(w http.ResponseWriter, r *http.Request) {
	me, ok := a.mustAdmin(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	id, _ := strconv.Atoi(strings.Split(idStr, "/")[0])
	if id < 1 {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	var u model.User
	if err := a.DB.First(&u, id).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	switch r.Method {
	case "PATCH":
		var in struct {
			Name     *string `json:"name"`
			Pass     *string `json:"password"`
			Disabled *bool   `json:"disabled"`
			Admin    *bool   `json:"admin"`
			QuotaMB  *int    `json:"quota_mb"`
			TOTPOff  *bool   `json:"totp_off"` // 重置（关闭并清除）两步验证
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		if in.Pass != nil && *in.Pass != "" {
			if err := validatePassword(*in.Pass, u.Email); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
		}
		// 防锁死：不能动自己的管理员身份和禁用开关
		if uint(id) == me.ID && ((in.Admin != nil && !*in.Admin) || (in.Disabled != nil && *in.Disabled)) {
			writeJSON(w, 400, map[string]string{"error": "不能取消自己的管理员身份或禁用自己"})
			return
		}
		upd := map[string]any{}
		if in.Name != nil {
			upd["name"] = strings.TrimSpace(*in.Name)
		}
		if in.Pass != nil && *in.Pass != "" {
			hash, _ := bcrypt.GenerateFromPassword([]byte(*in.Pass), bcrypt.DefaultCost)
			upd["pass_hash"] = string(hash)
			upd["token_version"] = gorm.Expr("token_version + ?", 1) // 重置密码后旧令牌立即失效
		}
		if in.Disabled != nil {
			upd["disabled"] = *in.Disabled
		}
		if in.Admin != nil {
			if !*in.Admin && u.Admin {
				var n int64
				a.DB.Model(&model.User{}).Where("admin = ? AND id <> ?", true, u.ID).Count(&n)
				if n == 0 {
					writeJSON(w, 400, map[string]string{"error": "至少保留一个管理员"})
					return
				}
			}
			upd["admin"] = *in.Admin
		}
		if in.TOTPOff != nil && *in.TOTPOff {
			upd["totp_enabled"] = false
			upd["totp_secret"] = ""
		}
		if in.QuotaMB != nil {
			upd["quota_mb"] = *in.QuotaMB
		}
		if len(upd) > 0 {
			a.DB.Model(&u).Updates(upd)
		}
		a.DB.First(&u, id)
		writeJSON(w, 200, u)
	case "DELETE":
		if uint(id) == me.ID {
			writeJSON(w, 400, map[string]string{"error": "不能删除自己"})
			return
		}
		if err := a.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_id = ?", u.ID).Delete(&model.Mail{}).Error; err != nil {
				return err
			}
			return tx.Delete(&u).Error
		}); err != nil {
			writeJSON(w, 500, map[string]string{"error": "删除失败"})
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		w.WriteHeader(405)
	}
}

// GET /api/admin/domains：各托管域名的邮箱数 + 解析记录数
func (a *Admin) Domains(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	var domains []model.Domain
	a.DB.Order("id ASC").Find(&domains)
	type item struct {
		model.Domain
		UserCount   int64 `json:"user_count"`
		RecordCount int64 `json:"record_count"`
	}
	out := make([]item, 0, len(domains))
	for _, dm := range domains {
		var uc, rc int64
		a.DB.Model(&model.User{}).Where("email LIKE ?", "%@"+dm.Name).Count(&uc)
		a.DB.Model(&model.DnsRecord{}).Where("domain_id = ?", dm.ID).Count(&rc)
		out = append(out, item{Domain: dm, UserCount: uc, RecordCount: rc})
	}
	writeJSON(w, 200, out)
}

// GET /api/admin/health -> 系统健康（CPU/内存/磁盘 + 80/90 告警）
func (a *Admin) HealthStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if a.Health == nil {
		writeJSON(w, 200, map[string]any{"alerts": []any{}})
		return
	}
	writeJSON(w, 200, a.Health.Current())
}

func (a *Admin) repoName() string {
	if a.Repo != "" {
		return a.Repo
	}
	return selfupdate.DefaultRepo
}

// GET /api/admin/about -> 版本信息 + 仓库
func (a *Admin) About(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{
		"version": a.Version, "commit": a.Commit, "date": a.Date, "repo": a.repoName(),
	})
}

// GET /api/admin/update/check -> 查询最新版本（不安装）
func (a *Admin) UpdateCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	latest, err := selfupdate.Latest(ctx, a.repoName())
	if err != nil {
		writeJSON(w, 200, map[string]any{"current": a.Version, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"current": a.Version, "latest": latest,
		"update_available": selfupdate.Differs(a.Version, latest),
	})
}

// POST /api/admin/update -> 立即更新到最新版（后台执行，服务会重启）
func (a *Admin) Update(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	repo, cur := a.repoName(), a.Version
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := selfupdate.Run(ctx, selfupdate.Options{Repo: repo, Current: cur, Log: log.Printf}); err != nil {
			log.Println("self-update:", err)
		}
	}()
	writeJSON(w, 202, map[string]any{"ok": true, "message": "已开始更新，服务将自动重启；若未重启请手动重启进程"})
}
