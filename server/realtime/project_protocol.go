package realtime

import "ToDoList/server/models"

const (
	ProjectMessageTypeSync  = "PROJECT_SYNC"
	ProjectMessageTypeInit  = "PROJECT_INIT"
	ProjectMessageTypePing  = "PING"
	ProjectMessageTypePong  = "PONG"
	ProjectMessageTypeError = "PROJECT_ERROR"

	ProjectMessageTypeTaskCreated = "TASK_CREATED"
	ProjectMessageTypeTaskUpdated = "TASK_UPDATED"
	ProjectMessageTypeTaskDeleted = "TASK_DELETED"
	ProjectMessageTypePresence    = "PRESENCE_SNAPSHOT"
	ProjectMessageTypeLockRequest = "LOCK_REQUEST"
	ProjectMessageTypeLockRelease = "LOCK_RELEASE"
	ProjectMessageTypeTaskLocked  = "TASK_LOCKED"
	ProjectMessageTypeTaskUnlocked = "TASK_UNLOCKED"
	ProjectMessageTypeLockError   = "LOCK_ERROR"
)

type ProjectClientMessage struct {
	Type   string `json:"type"`
	Cursor int64  `json:"cursor,omitempty"`
	TaskID int    `json:"task_id,omitempty"`
	Field  string `json:"field,omitempty"`
}

type ProjectServerMessage struct {
	Type         string             `json:"type"`
	ProjectID    int                `json:"project_id,omitempty"`
	EventID      string             `json:"event_id,omitempty"`
	Cursor       int64              `json:"cursor,omitempty"`
	Event        *models.TaskEvent  `json:"event,omitempty"`
	Events       []models.TaskEvent `json:"events,omitempty"`
	Presence     []ProjectPresence  `json:"presence,omitempty"`
	Lock         *ProjectLock       `json:"lock,omitempty"`
	NextCursor   int64              `json:"next_cursor,omitempty"`
	HasMore      bool               `json:"has_more,omitempty"`
	Error        string             `json:"error,omitempty"`
	ServerNodeID string             `json:"server_node_id,omitempty"`
}

type ProjectLock struct {
	TaskID         int    `json:"task_id"`
	Field          string `json:"field"`
	HolderUserID   int    `json:"holder_user_id"`
	HolderUsername string `json:"holder_username,omitempty"`
}

type ProjectPresence struct {
	UserID      int    `json:"user_id"`
	Username    string `json:"username,omitempty"`
	Connections int    `json:"connections"`
}

type projectPubSubEnvelope struct {
	OriginNodeID string               `json:"origin_node_id"`
	ProjectID    int                  `json:"project_id"`
	Message      ProjectServerMessage `json:"message"`
}

func taskEventMessageType(eventType string) string {
	switch eventType {
	case models.TaskEventTypeCreated:
		return ProjectMessageTypeTaskCreated
	case models.TaskEventTypeUpdated:
		return ProjectMessageTypeTaskUpdated
	case models.TaskEventTypeDeleted:
		return ProjectMessageTypeTaskDeleted
	default:
		return eventType
	}
}
