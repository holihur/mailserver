package handler

// TOTP 两步验证：/api/totp 状态、/api/totp/setup 生成密钥、/api/totp/enable|disable 启停，
// 以及登录第二因子 /api/login/totp。密钥用 JWT_SECRET 派生的 AES-GCM 加密存储。

import (
	"encoding/json"
	"net/http"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/model"
	"mailserver/internal/secret"
	"mailserver/internal/totp"

	"gorm.io/gorm"
)

const totpIssuer = "Sweetcorn"

type TOTPBox struct {
	DB *gorm.DB
}

// GET /api/totp -> {enabled}
func (h *TOTPBox) Status(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	var u model.User
	if err := h.DB.Select("id", "totp_enabled").First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, 200, map[string]any{"enabled": u.TOTPEnabled})
}

// POST /api/totp/setup -> 生成新密钥（尚未启用），返回 secret 与 otpauth 链接。
func (h *TOTPBox) Setup(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var u model.User
	if err := h.DB.First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	if u.TOTPEnabled {
		writeJSON(w, 400, map[string]string{"error": "已启用两步验证，请先关闭再重新生成"})
		return
	}
	sec, err := totp.GenerateSecret()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "生成失败"})
		return
	}
	enc, err := secret.Encrypt([]byte(sec))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "加密失败"})
		return
	}
	if err := h.DB.Model(&u).Update("totp_secret", enc).Error; err != nil {
		writeJSON(w, 500, map[string]string{"error": "保存失败"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"secret": sec,
		"url":    totp.ProvisioningURL(totpIssuer, u.Email, sec),
	})
}

// POST /api/totp/enable {code} -> 校验通过后启用
func (h *TOTPBox) Enable(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
	var u model.User
	if err := h.DB.First(&u, uid).Error; err != nil || u.TOTPSecret == "" {
		writeJSON(w, 400, map[string]string{"error": "请先生成密钥"})
		return
	}
	raw, err := secret.Decrypt(u.TOTPSecret)
	if err != nil || !totp.Verify(string(raw), in.Code) {
		writeJSON(w, 400, map[string]string{"error": "动态验证码错误"})
		return
	}
	h.DB.Model(&u).Update("totp_enabled", true)
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": true})
}

// POST /api/totp/disable {code} -> 校验通过后关闭并清除密钥
func (h *TOTPBox) Disable(w http.ResponseWriter, r *http.Request) {
	uid, ok := uidOf(h.DB, w, r)
	if !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
	var u model.User
	if err := h.DB.First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	if !u.TOTPEnabled {
		writeJSON(w, 200, map[string]any{"ok": true, "enabled": false})
		return
	}
	raw, err := secret.Decrypt(u.TOTPSecret)
	if err != nil || !totp.Verify(string(raw), in.Code) {
		writeJSON(w, 400, map[string]string{"error": "动态验证码错误"})
		return
	}
	h.DB.Model(&u).Updates(map[string]any{"totp_enabled": false, "totp_secret": ""})
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": false})
}

// POST /api/login/totp {challenge, code} -> 登录第二因子，返回 access 令牌
func (a *Auth) LoginTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad body"})
		return
	}
	if a.RL != nil && !a.RL.Allow("totp:"+clientIP(r), 20, loginWindow) {
		writeJSON(w, 429, map[string]string{"error": "尝试过于频繁，请稍后再试"})
		return
	}
	uid, err := auth.TOTPChallengeUserID(in.Challenge)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "登录已过期，请重新登录"})
		return
	}
	var u model.User
	if err := a.DB.First(&u, uid).Error; err != nil || u.Disabled {
		writeJSON(w, 401, map[string]string{"error": "账号不可用"})
		return
	}
	if !u.TOTPEnabled {
		writeJSON(w, 400, map[string]string{"error": "未启用两步验证"})
		return
	}
	raw, err := secret.Decrypt(u.TOTPSecret)
	if err != nil || !totp.Verify(string(raw), strings.TrimSpace(in.Code)) {
		writeJSON(w, 401, map[string]string{"error": "动态验证码错误"})
		return
	}
	if !u.Admin && isAdminEmail(effectiveAdminEmails(a.RT, a.AdminEmails), u.Email) {
		u.Admin = true
		a.DB.Model(&u).Update("admin", true)
	}
	tok, _ := auth.Sign(u.ID, u.Email)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}
