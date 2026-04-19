package models

import (
	"time"
)

type User struct {
	ID           int       `gorm:"primaryKey"                 json:"id"`
	Email        string    `gorm:"size:255;not null;uniqueIndex"                  json:"email"`
	Password     string    `gorm:"size:255;not null"          json:"-"`
	Username     string    `gorm:"size:64;not null;uniqueIndex"           json:"username"`
	AvatarURL    string    `gorm:"size:512"                   json:"avatar_url"`
	Timezone     string    `gorm:"size:64;default:Asia/Shanghai" json:"timezone"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	TokenVersion int       `gorm:"not null;default:1"  json:"-"`
}
