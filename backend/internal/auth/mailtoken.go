package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"mailserver/internal/model"

	"gorm.io/gorm"
)

// MailTokenPrefix 应用专用密码（PAT）的固定前缀，便于识别与排查。
const MailTokenPrefix = "mst_"

// NewMailToken 生成一个新 PAT。返回明文（只应展示一次）与用于界面展示的 prefix。
func NewMailToken() (plain, prefix string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plain = MailTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	prefix = plain[:12]
	return plain, prefix, nil
}

// HashMailToken 计算令牌摘要。令牌是 256bit 随机值，sha256 足以防止库泄露后被反推，
// 且可确定性索引查找（不像 bcrypt 无法按值查询）。
func HashMailToken(plain string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plain)))
	return hex.EncodeToString(sum[:])
}

// AuthenticateMail 校验邮件客户端（IMAP/POP3/SMTP）凭据。
// 出于安全考虑只接受 PAT，不接受网页登录密码；返回令牌归属且未禁用的用户。
func AuthenticateMail(db *gorm.DB, email, secret string) (*model.User, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, MailTokenPrefix) {
		return nil, errors.New("请使用「应用专用密码」(PAT)，不要使用网页登录密码")
	}
	var tok model.MailToken
	if err := db.Where("hash = ?", HashMailToken(secret)).First(&tok).Error; err != nil {
		return nil, errors.New("应用专用密码无效")
	}
	var u model.User
	if err := db.First(&u, tok.UserID).Error; err != nil {
		return nil, errors.New("应用专用密码无效")
	}
	if u.Disabled {
		return nil, errors.New("账号已禁用")
	}
	// 客户端填写的用户名必须与令牌归属一致，避免张冠李戴。
	if e := strings.TrimSpace(email); e != "" && !strings.EqualFold(e, u.Email) {
		return nil, errors.New("应用专用密码与账号不匹配")
	}
	now := time.Now()
	db.Model(&model.MailToken{}).Where("id = ?", tok.ID).Update("last_used", now)
	return &u, nil
}
