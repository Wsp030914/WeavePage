package models

// 文件说明：这个文件定义业务数据模型。
// 实现方式：统一维护持久化字段与跨层共享结构。
// 这样做的好处是数据语义更稳定。
import "time"

type TaskComment struct {
	ID         int        `gorm:"primaryKey" json:"id"`
	ProjectID  int        `gorm:"not null;index:idx_task_comments_task_created,priority:1;index" json:"project_id"`
	TaskID     int        `gorm:"not null;index:idx_task_comments_task_created,priority:2;index" json:"task_id"`
	UserID     int        `gorm:"not null;index" json:"user_id"`
	ContentMD  string     `gorm:"type:text;not null" json:"content_md"`
	AnchorType string     `gorm:"size:32;not null;default:'document';index" json:"anchor_type"`
	AnchorText string     `gorm:"type:text" json:"anchor_text,omitempty"`
	Resolved   bool       `gorm:"not null;default:false;index" json:"resolved"`
	ResolvedBy *int       `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `gorm:"index:idx_task_comments_task_created,priority:3" json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type TaskCommentInfo struct {
	TaskComment
	User UserInfo `gorm:"embedded;embeddedPrefix:user_" json:"user"`
}
