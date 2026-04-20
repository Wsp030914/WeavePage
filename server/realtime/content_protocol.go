package realtime

// 文件说明：这个文件定义正文协同实时协议结构。
// 实现方式：集中声明正文消息类型、增量更新载荷和跨节点广播信封结构。
// 这样做的好处是正文协同的协议层独立于 hub 运行时，后续扩字段或改前端 provider 时更容易收敛影响面。

const (
	ContentMessageTypeSync   = "CONTENT_SYNC"
	ContentMessageTypeInit   = "CONTENT_INIT"
	ContentMessageTypeUpdate = "CONTENT_UPDATE"
	ContentMessageTypeAck    = "CONTENT_ACK"
	ContentMessageTypeError  = "CONTENT_ERROR"
	ContentMessageTypePing   = "PING"
	ContentMessageTypePong   = "PONG"
)

type ContentClientMessage struct {
	Type            string  `json:"type"`
	MessageID       string  `json:"message_id,omitempty"`
	LastUpdateID    int64   `json:"last_update_id,omitempty"`
	Update          []byte  `json:"update,omitempty"`
	ContentSnapshot *string `json:"content_snapshot,omitempty"`
}

type ContentServerMessage struct {
	Type         string              `json:"type"`
	MessageID    string              `json:"message_id,omitempty"`
	TaskID       int                 `json:"task_id,omitempty"`
	ActorID      int                 `json:"actor_id,omitempty"`
	UpdateID     int64               `json:"update_id,omitempty"`
	Update       []byte              `json:"update,omitempty"`
	Updates      []ContentUpdateItem `json:"updates,omitempty"`
	NextCursor   int64               `json:"next_cursor,omitempty"`
	HasMore      bool                `json:"has_more,omitempty"`
	Error        string              `json:"error,omitempty"`
	Duplicate    bool                `json:"duplicate,omitempty"`
	ServerNodeID string              `json:"server_node_id,omitempty"`
}

type ContentUpdateItem struct {
	ID        int64  `json:"id"`
	MessageID string `json:"message_id"`
	TaskID    int    `json:"task_id"`
	ActorID   int    `json:"actor_id"`
	Update    []byte `json:"update"`
}

type contentPubSubEnvelope struct {
	OriginNodeID string               `json:"origin_node_id"`
	TaskID       int                  `json:"task_id"`
	Message      ContentServerMessage `json:"message"`
}
