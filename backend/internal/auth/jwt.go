package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte("dev-secret-change-me-32chars!!")

// 令牌类型：access 用于接口鉴权；totp 仅用于登录第二因子，不能当 access 用。
const (
	typAccess = "access"
	typTOTP   = "totp"
)

func SetSecret(s string) { secret = []byte(s) }

func sign(claims jwt.MapClaims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(secret)
}

// NewJTI 生成随机会话标识（十六进制）。
func NewJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func Sign(userID uint, email string, ver int) (string, error) {
	return SignSession(userID, email, ver, "")
}

// SignSession 如 Sign，但带会话 jti（用于逐会话踢出）。
func SignSession(userID uint, email string, ver int, jti string) (string, error) {
	claims := jwt.MapClaims{
		"uid":   userID,
		"email": email,
		"typ":   typAccess,
		"ver":   ver,
		"exp":   time.Now().Add(72 * time.Hour).Unix(),
	}
	if jti != "" {
		claims["jti"] = jti
	}
	return sign(claims)
}

// SignTOTPChallenge 签发短期挑战令牌，供登录第二因子校验使用。
func SignTOTPChallenge(userID uint, email string) (string, error) {
	return sign(jwt.MapClaims{
		"uid":   userID,
		"email": email,
		"typ":   typTOTP,
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
	})
}

func parse(token string) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, errors.New("bad token")
	}
	m, _ := tok.Claims.(jwt.MapClaims)
	return m, nil
}

func claimUID(m jwt.MapClaims) (uint, error) {
	uid, _ := m["uid"].(float64)
	if uid <= 0 {
		return 0, errors.New("bad uid")
	}
	return uint(uid), nil
}

// UserID 解析 Bearer access 令牌。
func UserID(r *http.Request) (uint, error) {
	uid, _, _, err := Access(r)
	return uid, err
}

// Access 解析 Bearer access 令牌，返回 uid / token_version / jti（旧令牌无 ver/jti 视为 0/空）。
func Access(r *http.Request) (uint, int, string, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return 0, 0, "", errors.New("no token")
	}
	m, err := parse(strings.TrimPrefix(h, "Bearer "))
	if err != nil {
		return 0, 0, "", err
	}
	if t, _ := m["typ"].(string); t != "" && t != typAccess {
		return 0, 0, "", errors.New("wrong token type")
	}
	uid, err := claimUID(m)
	if err != nil {
		return 0, 0, "", err
	}
	jti, _ := m["jti"].(string)
	return uid, claimVersion(m), jti, nil
}

func claimVersion(m jwt.MapClaims) int {
	if v, ok := m["ver"].(float64); ok {
		return int(v)
	}
	return 0
}

// TOTPChallengeUserID 解析登录第二因子的挑战令牌（typ=totp）。
func TOTPChallengeUserID(token string) (uint, error) {
	m, err := parse(token)
	if err != nil {
		return 0, err
	}
	if t, _ := m["typ"].(string); t != typTOTP {
		return 0, errors.New("not a totp challenge")
	}
	return claimUID(m)
}
