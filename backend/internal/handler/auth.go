package handler

import (
	"encoding/json"
	"net/http"

	"mailserver/internal/auth"
	"mailserver/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Auth struct{ DB *gorm.DB }

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
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Pass), bcrypt.MinCost) // 低内存用 MinCost
	u := model.User{Email: in.Email, Name: in.Name, PassHash: string(hash)}
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
		writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PassHash), []byte(in.Pass)) != nil {
		writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
		return
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
	writeJSON(w, 200, u)
}
