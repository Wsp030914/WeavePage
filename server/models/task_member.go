package models

// 文件说明：这个文件定义业务数据模型。
// 实现方式：统一维护持久化字段与跨层共享结构。
// 这样做的好处是数据语义更稳定。
import "time"

const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

type TaskMember struct {
	ID       int       `gorm:"primaryKey" json:"id"`
	TaskID   int       `gorm:"index;index:idx_task_members_user_task,priority:2;not null;uniqueIndex:ux_task_user" json:"task_id"`
	UserID   int       `gorm:"index;index:idx_task_members_user_task,priority:1;not null;uniqueIndex:ux_task_user" json:"user_id"`
	Role     string    `gorm:"size:20;not null;default:'viewer'" json:"role"` // owner, editor, viewer
	JoinedAt time.Time `json:"joined_at"`
}

type TaskMemberInfo struct {
	UserID   int       `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	User     UserInfo  `json:"user" gorm:"embedded;embeddedPrefix:user_"`
}

type UserInfo struct {
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}
