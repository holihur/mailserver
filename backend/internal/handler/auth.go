package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/model"
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
	if len(in.Pass) < 6 || in.Email == "" {
		writeJSON(w, 400, map[string]string{"error": "email/password 非法"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	var n int64
	a.DB.Model(&model.User{}).Count(&n)
	// 默认关闭注册；首个用户始终可注册（否则无人能进）
	if n > 0 && (a.RT == nil || !a.RT.RegistrationEnabled()) {
		writeJSON(w, 403, map[string]string{"error": "注册已关闭，请联系管理员开通账号"})
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
	tok, _ := auth.Sign(u.ID, u.Email)
	writeJSON(w, 201, map[string]any{"token": tok, "user": u})
}

// GET /api/site -> 公开站点配置（是否需要显示注册入口）
func (a *Auth) Site(w http.ResponseWriter, r *http.Request) {
	var n int64
	a.DB.Model(&model.User{}).Count(&n)
	open := n == 0 || (a.RT != nil && a.RT.RegistrationEnabled())
	writeJSON(w, 200, map[string]any{"registration": open, "has_users": n > 0})
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
			writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
			return
		}
	}
	if u.Disabled {
		writeJSON(w, 401, map[string]string{"error": "账号已禁用，请联系管理员"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(in.Pass)) != nil {
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
	tok, _ := auth.Sign(u.ID, u.Email)
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
	writeJSON(w, 200, u)
}
