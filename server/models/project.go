package models

import (
	"time"

	"gorm.io/gorm"
)

type Project struct {
	ID        int       `gorm:"primaryKey"                               json:"id"`
	UserID    int       `gorm:"index;not null;uniqueIndex:ux_user_name,priority:1" json:"user_id"`
	Name      string    `gorm:"size:128;not null;uniqueIndex:ux_user_name,priority:2" json:"name"`
	Color     string    `gorm:"size:16;not null;default:'#9b6d6d'" json:"color"`
	SortOrder int64     `gorm:"not null;default:0"                        json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProjectIDScore struct {
	ID        int
	SortOrder int64
}

func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.SortOrder == 0 {
		p.SortOrder = time.Now().UnixNano()
	}
	return nil
}
