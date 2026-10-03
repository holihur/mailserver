package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte("dev-secret-change-me-32chars!!")

func SetSecret(s string) { secret = []byte(s) }

func Sign(userID uint, email string) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":   userID,
		"email": email,
		"exp":   time.Now().Add(72 * time.Hour).Unix(),
	})
	return t.SignedString(secret)
}

func UserID(r *http.Request) (uint, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return 0, errors.New("no token")
	}
	tok, err := jwt.Parse(strings.TrimPrefix(h, "Bearer "), func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return 0, errors.New("bad token")
	}
	m, _ := tok.Claims.(jwt.MapClaims)
	uid, _ := m["uid"].(float64)
	return uint(uid), nil
}
