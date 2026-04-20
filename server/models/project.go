package models

// 文件说明：这个文件定义空间项目模型。
// 实现方式：项目结构只保留空间级最小字段，把更重的业务行为留给 service 和 repo 层处理。
// 这样做的好处是 Spaces 作为顶层信息架构节点时可以保持稳定、简单和可缓存。

import (
	"time"

	"gorm.io/gorm"
)

// Project 是用户拥有的空间实体。
type Project struct {
	ID          int            `gorm:"primaryKey"                               json:"id"`
	UserID      int            `gorm:"index;not null;uniqueIndex:ux_user_name,priority:1" json:"user_id"`
	Name        string         `gorm:"size:128;not null;uniqueIndex:ux_user_name,priority:2" json:"name"`
	DeletedName string         `gorm:"size:128" json:"deleted_name,omitempty"`
	Color       string         `gorm:"size:16;not null;default:'#9b6d6d'" json:"color"`
	SortOrder   int64          `gorm:"not null;default:0"                        json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty" swaggertype:"string" format:"date-time"`
	DeletedBy   *int           `gorm:"index" json:"deleted_by,omitempty"`
}

// ProjectIDScore 是项目列表排序缓存所需的最小字段集合。
type ProjectIDScore struct {
	ID        int
	SortOrder int64
}

// ProjectSummaryCache stores one project list/search page snapshot.
type ProjectSummaryCache struct {
	Projects []Project `json:"projects"`
	Total    int64     `json:"total"`
	UID      int       `json:"uid"`
	Ver      int64     `json:"ver"`
	Name     string    `json:"name"`
	Page     int       `json:"page"`
	Size     int       `json:"size"`
}

// BeforeCreate 在项目首次入库前补齐默认排序值。
func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.SortOrder == 0 {
		p.SortOrder = time.Now().UnixNano()
	}
	return nil
}
