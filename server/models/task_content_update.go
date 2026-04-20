package models

// 文件说明：这个文件定义业务数据模型。
// 实现方式：统一维护持久化字段与跨层共享结构。
// 这样做的好处是数据语义更稳定。
import "time"

// TaskContentUpdate stores one persisted Yjs document update for a task body.
type TaskContentUpdate struct {
	ID              int64     `gorm:"primaryKey;autoIncrement;index:idx_task_content_project_task_cursor,priority:3" json:"id"`
	MessageID       string    `gorm:"size:64;not null;uniqueIndex" json:"message_id"`
	ProjectID       int       `gorm:"not null;index:idx_task_content_project_task_cursor,priority:1" json:"project_id"`
	TaskID          int       `gorm:"not null;index:idx_task_content_project_task_cursor,priority:2" json:"task_id"`
	ActorID         int       `gorm:"not null;index" json:"actor_id"`
	Update          []byte    `gorm:"type:longblob;not null" json:"update"`
	ContentSnapshot *string   `gorm:"type:longtext" json:"content_snapshot,omitempty"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
}
