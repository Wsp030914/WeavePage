package realtime

// 文件说明：这个文件定义项目级实时协同协议结构。
// 实现方式：集中声明消息类型、客户端/服务端消息体、presence 结构和锁结构。
// 这样做的好处是项目实时链路的协议边界清晰，handler、hub 和前端都能围绕同一份消息模型演进。

import "ToDoList/server/models"

const (
	ProjectMessageTypeSync  = "PROJECT_SYNC"
	ProjectMessageTypeInit  = "PROJECT_INIT"
	ProjectMessageTypePing  = "PING"
	ProjectMessageTypePong  = "PONG"
	ProjectMessageTypeError = "PROJECT_ERROR"

	ProjectMessageTypeTaskCreated  = "TASK_CREATED"
	ProjectMessageTypeTaskUpdated  = "TASK_UPDATED"
	ProjectMessageTypeTaskDeleted  = "TASK_DELETED"
	ProjectMessageTypePresence     = "PRESENCE_SNAPSHOT"
	ProjectMessageTypeLockRequest  = "LOCK_REQUEST"
	ProjectMessageTypeLockRelease  = "LOCK_RELEASE"
	ProjectMessageTypeViewDocument = "VIEW_DOCUMENT"
	ProjectMessageTypeTaskLocked   = "TASK_LOCKED"
	ProjectMessageTypeTaskUnlocked = "TASK_UNLOCKED"
	ProjectMessageTypeLockError    = "LOCK_ERROR"
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
	UserID         int    `json:"user_id"`
	Username       string `json:"username,omitempty"`
	Connections    int    `json:"connections"`
	ViewingTaskIDs []int  `json:"viewing_task_ids,omitempty"`
}

type projectPubSubEnvelope struct {
	OriginNodeID string               `json:"origin_node_id"`
	ProjectID    int                  `json:"project_id"`
	Message      ProjectServerMessage `json:"message"`
}

// taskEventMessageType 把持久化事件类型映射成前端实时消息类型。
// 这里单独映射一层，是为了让数据库事件命名和前端协议命名可以按需解耦。
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
