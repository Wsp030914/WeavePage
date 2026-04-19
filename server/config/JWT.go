package config

import (
	"time"
)

var (
	Secret    string
	Issuer    string
	Audience  string
	AccessTTL time.Duration
)

func InitJWT(cfg *JWTSettings) {
	Secret = cfg.Secret
	if len(Secret) < 32 {
		panic("JWT_SECRET too short, need >= 32 bytes")
	}
	Issuer = cfg.Issuer
	Audience = cfg.Audience
	AccessTTL = cfg.AccessTTL
}
