package config

// 文件说明：这个文件负责初始化 JWT 运行时配置。
// 实现方式：把配置对象中的 secret、issuer、audience 和 TTL 提升为运行时全局变量。
// 这样做的好处是令牌工具函数可以直接读取稳定配置，而不必层层传参。

import (
	"time"
)

var (
	Secret    string
	Issuer    string
	Audience  string
	AccessTTL time.Duration
)

// InitJWT 初始化 JWT 运行时参数。
// 这里对 secret 长度做硬校验，是为了避免用过短密钥签发令牌。
func InitJWT(cfg *JWTSettings) {
	Secret = cfg.Secret
	if len(Secret) < 32 {
		panic("JWT_SECRET too short, need >= 32 bytes")
	}
	Issuer = cfg.Issuer
	Audience = cfg.Audience
	AccessTTL = cfg.AccessTTL
}
