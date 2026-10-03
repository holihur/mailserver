package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Auth struct {
	DB          *gorm.DB
	AdminEmails string // 见 handler/admin.go isAdminEmail
	RT          *runtimecfg.Store
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
	if len(in.Pass) < 6 || in.Email == "" {
		writeJSON(w, 400, map[string]string{"error": "email/password 非法"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Pass), bcrypt.MinCost) // 低内存用 MinCost
	u := model.User{Email: email, Name: in.Name, PassHash: string(hash)}
	var n int64
	a.DB.Model(&model.User{}).Count(&n)
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

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Pass  string `json:"password"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
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
	writeJSON(w, 200, u)
}
