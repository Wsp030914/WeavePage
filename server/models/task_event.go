package models

// 文件说明：这个文件定义业务数据模型。
// 实现方式：统一维护持久化字段与跨层共享结构。
// 这样做的好处是数据语义更稳定。
import (
	"encoding/json"
	"time"
)

const (
	TaskEventTypeCreated = "task.created"
	TaskEventTypeUpdated = "task.updated"
	TaskEventTypeDeleted = "task.deleted"
)

type TaskEvent struct {
	ID          int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID     string          `gorm:"size:36;not null;uniqueIndex" json:"event_id"`
	ProjectID   int             `gorm:"not null;index:idx_task_events_project_cursor,priority:1" json:"project_id"`
	TaskID      int             `gorm:"not null;index:idx_task_events_project_cursor,priority:2" json:"task_id"`
	ActorID     int             `gorm:"not null;index" json:"actor_id"`
	EventType   string          `gorm:"size:32;not null;index" json:"event_type"`
	TaskVersion int             `gorm:"not null;default:0" json:"task_version"`
	Payload     json.RawMessage `gorm:"type:json;not null" json:"payload" swaggertype:"object"`
	CreatedAt   time.Time       `gorm:"index:idx_task_events_project_cursor,priority:3" json:"created_at"`
}

type TaskEventPayload struct {
	Task *Task `json:"task,omitempty"`
}
