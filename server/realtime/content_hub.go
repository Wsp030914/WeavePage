package realtime

// 文件说明：这个文件实现正文协同实时 hub。
// 实现方式：管理任务级连接房间、消息广播、Redis fan-out 与快照初始化。
// 这样做的好处是正文实时协同能够跨节点传播，同时保持单任务粒度隔离。
import (
	"ToDoList/server/models"
	"ToDoList/server/service"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	contentPubSubChannel = "realtime:task_content"

	contentWriteWait      = 10 * time.Second
	contentPongWait       = 60 * time.Second
	contentPingPeriod     = (contentPongWait * 9) / 10
	contentSendBufferSize = 32
	contentMaxMessageSize = 32 << 20
)

type ContentHub struct {
	svc     *service.TaskService
	rdb     *redis.Client
	nodeID  string
	metrics realtimeCounters

	mu    sync.RWMutex
	rooms map[int]map[*contentClient]struct{}
}

type contentClient struct {
	hub     *ContentHub
	conn    *websocket.Conn
	session service.TaskContentSession
	send    chan ContentServerMessage
	lg      *zap.Logger
	sendMu  sync.Mutex
	closed  bool
}

// NewContentHub 创建正文协同 hub。
// 这里按 task 维度建房间，是因为正文协同的广播粒度天然就是单文档。
func NewContentHub(svc *service.TaskService, rdb *redis.Client, nodeID string) *ContentHub {
	return &ContentHub{
		svc:    svc,
		rdb:    rdb,
		nodeID: nodeID,
		rooms:  make(map[int]map[*contentClient]struct{}),
	}
}

// Start 在当前节点启动 Redis fan-out 订阅。
// 这样做的好处是某个实例收到的正文消息可以继续广播到其他实例上的同任务连接。
func (h *ContentHub) Start(ctx context.Context, lg *zap.Logger) {
	if h == nil || h.rdb == nil {
		return
	}

	go h.runPubSub(ctx, lg)
}

// HandleConnection 把一个 WebSocket 连接接入正文房间，并启动读写循环。
// 初始同步会先把游标之后的更新补给客户端，避免断线重连时丢正文增量。
func (h *ContentHub) HandleConnection(ctx context.Context, conn *websocket.Conn, session service.TaskContentSession, initialCursor int64, lg *zap.Logger) {
	client := &contentClient{
		hub:     h,
		conn:    conn,
		session: session,
		send:    make(chan ContentServerMessage, contentSendBufferSize),
		lg:      lg,
	}

	h.register(client)
	defer h.unregister(client)

	go client.writePump()
	client.sendInitialSync(ctx, initialCursor)
	client.readPump(ctx)
}

func (h *ContentHub) register(client *contentClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.session.TaskID]
	if room == nil {
		room = make(map[*contentClient]struct{})
		h.rooms[client.session.TaskID] = room
	}
	room[client] = struct{}{}
	h.metrics.connectionsAccepted.Add(1)
}

func (h *ContentHub) unregister(client *contentClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.session.TaskID]
	if room != nil {
		delete(room, client)
		if len(room) == 0 {
			delete(h.rooms, client.session.TaskID)
		}
	}
	client.sendMu.Lock()
	defer client.sendMu.Unlock()
	if !client.closed {
		client.closed = true
		close(client.send)
		h.metrics.connectionsClosed.Add(1)
	}
}

func (h *ContentHub) MetricsSnapshot() HubMetrics {
	if h == nil {
		return HubMetrics{Hub: "content"}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	rooms := make([]RoomMetrics, 0, len(h.rooms))
	totalConnections := 0
	globalUsers := make(map[int]struct{})
	for taskID, room := range h.rooms {
		roomUsers := make(map[int]struct{})
		for client := range room {
			totalConnections++
			roomUsers[client.session.UserID] = struct{}{}
			globalUsers[client.session.UserID] = struct{}{}
		}
		rooms = append(rooms, RoomMetrics{
			ID:          taskID,
			Connections: len(room),
			Users:       len(roomUsers),
		})
	}
	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].ID < rooms[j].ID
	})

	return HubMetrics{
		NodeID:            h.nodeID,
		Hub:               "content",
		ActiveRooms:       len(h.rooms),
		ActiveConnections: totalConnections,
		ActiveUsers:       len(globalUsers),
		Rooms:             rooms,
		Counters:          h.metrics.snapshot(),
	}
}

// broadcast 只向同一个 task 房间里的连接广播正文消息。
// exclude 用来避免把某个客户端刚提交的 update 原样再回推给自己。
func (h *ContentHub) broadcast(taskID int, msg ContentServerMessage, exclude *contentClient) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	h.metrics.broadcastMessages.Add(1)
	for client := range h.rooms[taskID] {
		if client == exclude {
			continue
		}
		select {
		case client.send <- msg:
		default:
			h.metrics.droppedClients.Add(1)
			go h.unregister(client)
		}
	}
}

// publish 把当前节点收到的正文消息投递到 Redis Pub/Sub。
// 这样其他节点也能收到同一条正文更新，保证多实例部署下的协同一致性。
func (h *ContentHub) publish(ctx context.Context, taskID int, msg ContentServerMessage, lg *zap.Logger) {
	if h.rdb == nil {
		return
	}

	payload, err := json.Marshal(contentPubSubEnvelope{
		OriginNodeID: h.nodeID,
		TaskID:       taskID,
		Message:      msg,
	})
	if err != nil {
		lg.Warn("content.pubsub.marshal_failed", zap.Error(err))
		return
	}
	if err := h.rdb.Publish(ctx, contentPubSubChannel, payload).Err(); err != nil {
		h.metrics.pubSubPublishErrors.Add(1)
		lg.Warn("content.pubsub.publish_failed", zap.Int("task_id", taskID), zap.Error(err))
		return
	}
	h.metrics.pubSubPublished.Add(1)
}

// runPubSub 持续消费来自 Redis 的正文广播，并转发给本节点的本地连接。
func (h *ContentHub) runPubSub(ctx context.Context, lg *zap.Logger) {
	pubsub := h.rdb.Subscribe(ctx, contentPubSubChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var env contentPubSubEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				lg.Warn("content.pubsub.unmarshal_failed", zap.Error(err))
				continue
			}
			if env.OriginNodeID == h.nodeID {
				continue
			}
			h.metrics.pubSubReceived.Add(1)
			h.broadcast(env.TaskID, env.Message, nil)
		}
	}
}

// readPump 持续消费客户端消息，并在连接异常关闭时退出。
func (c *contentClient) readPump(ctx context.Context) {
	defer c.conn.Close()

	c.conn.SetReadLimit(contentMaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(contentPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(contentPongWait))
	})

	for {
		var msg ContentClientMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.lg.Debug("content.ws.read_closed", zap.Int("task_id", c.session.TaskID), zap.Error(err))
			}
			return
		}
		c.handleMessage(ctx, msg)
	}
}

// writePump 负责异步写回正文消息和定期发送 ping 保活。
func (c *contentClient) writePump() {
	ticker := time.NewTicker(contentPingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(contentWriteWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(contentWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *contentClient) sendInitialSync(ctx context.Context, cursor int64) {
	c.sendSync(ctx, cursor)
}

func (c *contentClient) handleMessage(ctx context.Context, msg ContentClientMessage) {
	switch msg.Type {
	case ContentMessageTypePing:
		c.enqueue(ContentServerMessage{Type: ContentMessageTypePong, TaskID: c.session.TaskID})
	case ContentMessageTypeSync:
		c.sendSync(ctx, msg.LastUpdateID)
	case ContentMessageTypeUpdate:
		c.handleUpdate(ctx, msg)
	default:
		c.sendError(fmt.Sprintf("unsupported message type: %s", msg.Type))
	}
}

// sendSync 通过 service 层按游标拉取正文 update log，再把结果封装成协议消息发回客户端。
func (c *contentClient) sendSync(ctx context.Context, cursor int64) {
	result, err := c.hub.svc.SyncTaskContentUpdates(ctx, c.lg, c.session, service.TaskContentSyncInput{
		Cursor: cursor,
		Limit:  100,
	})
	if err != nil {
		c.sendError(err.Error())
		return
	}

	c.enqueue(ContentServerMessage{
		Type:       ContentMessageTypeInit,
		TaskID:     c.session.TaskID,
		Updates:    contentUpdateItems(result.Updates),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (c *contentClient) handleUpdate(ctx context.Context, msg ContentClientMessage) {
	result, err := c.hub.svc.AppendTaskContentUpdate(ctx, c.lg, c.session, service.AppendTaskContentUpdateInput{
		MessageID:       msg.MessageID,
		Update:          msg.Update,
		ContentSnapshot: msg.ContentSnapshot,
	})
	if err != nil {
		c.sendError(err.Error())
		return
	}
	if result == nil || result.Update == nil {
		c.sendError("content update was not saved")
		return
	}

	ack := ContentServerMessage{
		Type:      ContentMessageTypeAck,
		MessageID: result.Update.MessageID,
		TaskID:    result.Update.TaskID,
		ActorID:   result.Update.ActorID,
		UpdateID:  result.Update.ID,
		Duplicate: result.Duplicate,
	}
	c.enqueue(ack)

	if result.Duplicate {
		c.hub.metrics.duplicateUpdates.Add(1)
		return
	}
	c.hub.metrics.contentUpdates.Add(1)

	updateMsg := ContentServerMessage{
		Type:         ContentMessageTypeUpdate,
		MessageID:    result.Update.MessageID,
		TaskID:       result.Update.TaskID,
		ActorID:      result.Update.ActorID,
		UpdateID:     result.Update.ID,
		Update:       result.Update.Update,
		ServerNodeID: c.hub.nodeID,
	}
	c.hub.broadcast(c.session.TaskID, updateMsg, c)
	c.hub.publish(ctx, c.session.TaskID, updateMsg, c.lg)
}

func (c *contentClient) enqueue(msg ContentServerMessage) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.closed {
		return
	}

	select {
	case c.send <- msg:
	default:
		go c.hub.unregister(c)
	}
}

func (c *contentClient) sendError(message string) {
	c.enqueue(ContentServerMessage{
		Type:   ContentMessageTypeError,
		TaskID: c.session.TaskID,
		Error:  message,
	})
}

func contentUpdateItems(updates []models.TaskContentUpdate) []ContentUpdateItem {
	if len(updates) == 0 {
		return nil
	}

	items := make([]ContentUpdateItem, 0, len(updates))
	for _, update := range updates {
		items = append(items, ContentUpdateItem{
			ID:        update.ID,
			MessageID: update.MessageID,
			TaskID:    update.TaskID,
			ActorID:   update.ActorID,
			Update:    update.Update,
		})
	}
	return items
}
