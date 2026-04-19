package utils

import (
	"ToDoList/server/config"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UID      int    `json:"uid"`
	Username string `json:"username"`
	Ver      int    `json:"ver"`
	jwt.RegisteredClaims
}

func jwtConfig() (secret []byte, issuer string, audience string, accessTTL time.Duration) {
	secret = []byte(config.Secret)
	issuer = config.Issuer
	audience = config.Audience
	accessTTL = config.AccessTTL
	if accessTTL <= 0 {
		accessTTL = 24 * time.Hour
	}
	return
}

// GenerateAccessToken 生成访问令牌
func GenerateAccessToken(uid int, username string, tokenVersion int) (string, time.Time, error) {
	secret, issuer, audience, accessTTL := jwtConfig()

	now := time.Now().UTC()
	exp := now.Add(accessTTL)
	jti := fmt.Sprintf("acc_%d_%d", uid, now.UnixNano())
	claims := &Claims{
		UID:      uid,
		Username: username,
		Ver:      tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  []string{audience},
			Subject:   "access",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        jti,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	return signed, exp, err
}

func Parse(tokenStr string) (*Claims, error) {
	secret, issuer, audience, _ := jwtConfig()

	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method == nil || t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected alg: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithAudience(audience), jwt.WithIssuer(issuer), jwt.WithStrictDecoding())
	if err != nil {
		return nil, err
	}
	if c, ok := tok.Claims.(*Claims); ok && tok.Valid {
		return c, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}
