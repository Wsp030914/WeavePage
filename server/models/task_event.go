package models

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
