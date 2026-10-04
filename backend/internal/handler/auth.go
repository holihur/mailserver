package handler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/model"
	"mailserver/internal/quota"
	"mailserver/internal/ratelimit"
	"mailserver/internal/runtimecfg"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	loginWindow   = 5 * time.Minute
	loginMaxPerIP = 20
)

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// isHostedEmailDomain 判断邮箱域名是否已在 domains 表托管（唯一依据）。
func (a *Auth) isHostedEmailDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	var dm model.Domain
	return a.DB.Where("LOWER(name) = ?", domain).First(&dm).Error == nil
}

type Auth struct {
	DB          *gorm.DB
	AdminEmails string // 见 handler/admin.go isAdminEmail
	RT          *runtimecfg.Store
	RL          *ratelimit.Limiter
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Pass  string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	if a.RL != nil && !a.RL.Allow("register:"+clientIP(r), 5, time.Hour) {
		writeJSON(w, 429, map[string]string{"error": "注册过于频繁，请稍后再试"})
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
	var n int64
	a.DB.Model(&model.User{}).Count(&n)
	// 默认关闭注册；首个用户始终可注册（否则无人能进）
	if n > 0 && (a.RT == nil || !a.RT.RegistrationEnabled()) {
		writeJSON(w, 403, map[string]string{"error": "注册已关闭，请联系管理员开通账号"})
		return
	}
	// 邮箱后缀必须是托管域名（与管理员建号一致）；首个引导管理员不受限
	if n > 0 && !a.isHostedEmailDomain(email[at+1:]) {
		writeJSON(w, 403, map[string]string{"error": "该邮箱域名未托管，无法注册；请联系管理员"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Pass), bcrypt.DefaultCost)
	u := model.User{Email: email, Name: in.Name, PassHash: string(hash)}
	if n == 0 || isAdminEmail(effectiveAdminEmails(a.RT, a.AdminEmails), email) {
		u.Admin = true // 首个注册用户即管理员；名单命中也提权
	}
	if err := a.DB.Create(&u).Error; err != nil {
		writeJSON(w, 409, map[string]string{"error": "邮箱已注册"})
		return
	}
	tok, _ := auth.Sign(u.ID, u.Email, u.TokenVersion)
	writeJSON(w, 201, map[string]any{"token": tok, "user": u})
}

// GET /api/site -> 公开站点配置（是否需要显示注册入口）
func (a *Auth) Site(w http.ResponseWriter, r *http.Request) {
	var n int64
	a.DB.Model(&model.User{}).Count(&n)
	open := n == 0 || (a.RT != nil && a.RT.RegistrationEnabled())
	oidcOn := a.RT != nil && a.RT.OIDCEnabled() && a.RT.OIDCIssuer() != ""
	writeJSON(w, 200, map[string]any{"registration": open, "has_users": n > 0, "oidc": oidcOn})
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Pass  string `json:"password"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	if a.RL != nil && !a.RL.Allow("login:"+clientIP(r), loginMaxPerIP, loginWindow) {
		writeJSON(w, 429, map[string]string{"error": "尝试过于频繁，请稍后再试"})
		return
	}
	var u model.User
	if err := a.DB.Where("email = ?", in.Email).First(&u).Error; err != nil {
		// 兼容历史大小写不一致的数据
		if err := a.DB.Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(in.Email))).First(&u).Error; err != nil {
			a.recordLogin(0, in.Email, r, false)
			writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
			return
		}
	}
	if u.Disabled {
		a.recordLogin(u.ID, u.Email, r, false)
		writeJSON(w, 401, map[string]string{"error": "账号已禁用，请联系管理员"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(in.Pass)) != nil {
		a.recordLogin(u.ID, u.Email, r, false)
		writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
		return
	}
	// 两步验证：先返回挑战令牌，再由 /api/login/totp 完成登录
	if u.TOTPEnabled {
		ch, _ := auth.SignTOTPChallenge(u.ID, u.Email)
		writeJSON(w, 200, map[string]any{"totp_required": true, "challenge": ch})
		return
	}
	if !u.Admin && isAdminEmail(effectiveAdminEmails(a.RT, a.AdminEmails), u.Email) {
		u.Admin = true
		a.DB.Model(&u).Update("admin", true)
	}
	tok, _ := auth.Sign(u.ID, u.Email, u.TokenVersion)
	a.recordLogin(u.ID, u.Email, r, true)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (a *Auth) Me(w http.ResponseWriter, r *http.Request) {
	uid, err := auth.UserID(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	var u model.User
	a.DB.First(&u, uid)
	if u.ID == 0 || u.Disabled {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	// PATCH：修改昵称 / 邮件签名
	if r.Method == "PATCH" {
		var in struct {
			Name      *string `json:"name"`
			Signature *string `json:"signature"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
		upd := map[string]any{}
		if in.Name != nil {
			upd["name"] = strings.TrimSpace(*in.Name)
		}
		if in.Signature != nil {
			s := *in.Signature
			if len(s) > 1000 {
				s = s[:1000]
			}
			upd["signature"] = s
		}
		if len(upd) > 0 {
			a.DB.Model(&u).Updates(upd)
		}
		a.DB.First(&u, uid)
	}
	u.QuotaUsed = quota.Usage(a.DB, uid)
	writeJSON(w, 200, u)
}

// NotifyLogin 由 main 注入：新 IP 登录时给用户发提醒邮件。
var NotifyLogin func(db *gorm.DB, uid uint, email, ip, ua string)

// 常见弱密码（小写）拦截，配合长度要求。
var weakPasswords = map[string]bool{
	"password": true, "password1": true, "passw0rd": true, "12345678": true,
	"123456789": true, "1234567890": true, "qwertyui": true, "qwerty123": true,
	"11111111": true, "abc12345": true, "iloveyou": true, "admin123": true,
	"letmein1": true, "welcome1": true, "changeme": true,
}

// validatePassword 校验新密码：至少 8 位 + 非常见弱密码 + 不等于账号名。
func validatePassword(pw, email string) error {
	if len([]rune(pw)) < 8 {
		return errors.New("密码至少 8 位")
	}
	if weakPasswords[strings.ToLower(pw)] {
		return errors.New("密码过于常见，请更换")
	}
	if local := strings.SplitN(strings.ToLower(strings.TrimSpace(email)), "@", 2)[0]; local != "" && strings.EqualFold(pw, local) {
		return errors.New("密码不能与账号名相同")
	}
	return nil
}

// POST /api/me/password {old,new}：本人修改密码；成功后 token_version++，旧令牌全部失效。
func (a *Auth) ChangePassword(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(a.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	var u model.User
	if err := a.DB.First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(in.Old)) != nil {
		writeJSON(w, 400, map[string]string{"error": "原密码错误"})
		return
	}
	if err := validatePassword(in.New, u.Email); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	newVer := u.TokenVersion + 1
	if err := a.DB.Model(&model.User{}).Where("id = ?", uid).Updates(map[string]any{
		"pass_hash": string(hash), "token_version": newVer, "must_change_password": false,
	}).Error; err != nil {
		writeJSON(w, 500, map[string]string{"error": "保存失败"})
		return
	}
	// 换发当前会话的新令牌，其余旧令牌因版本不符立即失效
	tok, _ := auth.Sign(u.ID, u.Email, newVer)
	writeJSON(w, 200, map[string]any{"ok": true, "token": tok})
}

// recordLogin 记录登录事件；首次从某 IP 成功登录时触发告警钩子。
func (a *Auth) recordLogin(uid uint, email string, r *http.Request, success bool) {
	ip := clientIP(r)
	ua := r.Header.Get("User-Agent")
	if len(ua) > 200 {
		ua = ua[:200]
	}
	a.DB.Create(&model.LoginEvent{UserID: uid, Email: strings.ToLower(strings.TrimSpace(email)), IP: ip, UserAgent: ua, Success: success})
	if success && uid != 0 && NotifyLogin != nil {
		var n int64
		a.DB.Model(&model.LoginEvent{}).Where("user_id = ? AND success = ? AND ip = ?", uid, true, ip).Count(&n)
		if n == 1 {
			NotifyLogin(a.DB, uid, email, ip, ua)
		}
	}
}

// GET /api/me/logins -> 最近 30 条登录历史
func (a *Auth) Logins(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(a.DB, w, r)
	if !ok {
		return
	}
	var evs []model.LoginEvent
	a.DB.Where("user_id = ?", uid).Order("id DESC").Limit(30).Find(&evs)
	if evs == nil {
		evs = []model.LoginEvent{}
	}
	writeJSON(w, 200, evs)
}

// POST /api/me/logout-all -> 递增 token_version，登出所有设备（当前会话换发新 token）
func (a *Auth) LogoutAll(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(a.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var u model.User
	if err := a.DB.First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	newVer := u.TokenVersion + 1
	a.DB.Model(&model.User{}).Where("id = ?", uid).Update("token_version", newVer)
	tok, _ := auth.Sign(u.ID, u.Email, newVer)
	writeJSON(w, 200, map[string]any{"ok": true, "token": tok})
}
