package utils

// 文件说明：这个文件提供 JWT 生成与解析工具。
// 实现方式：统一从 config 读取签名参数，封装 access token 的 claims 结构、签发和校验逻辑。
// 这样做的好处是认证链路里所有令牌都共享一致的签名规则和声明格式。

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

// jwtConfig 读取当前 JWT 运行时配置。
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

// GenerateAccessToken 生成访问令牌。
// claims 里携带 tokenVersion，是为了支持服务端在密码修改或强制下线后立即让旧 token 失效。
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

// Parse 解析并校验 JWT。
// 这里同时校验签名算法、issuer 和 audience，是为了尽量缩小错误 token 被误接受的空间。
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
