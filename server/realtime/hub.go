package realtime

// 文件说明：这个文件实现项目级实时协同 hub。
// 实现方式：按项目维度管理连接、在线态、锁广播与 Redis Pub/Sub fan-out。
// 这样做的好处是项目级实时事件、presence 和锁状态可以在多实例之间一致传播。
import (
	"ToDoList/server/cache"
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
	projectPubSubChannel = "realtime:project_events"

	projectWriteWait      = 10 * time.Second
	projectPongWait       = 60 * time.Second
	projectPingPeriod     = (projectPongWait * 9) / 10
	projectSendBufferSize = 32
	projectMaxMessageSize = 1 << 20
	projectPresencePeriod = 15 * time.Second
)

type ProjectHub struct {
	svc    *service.TaskService
	rdb    *redis.Client
	nodeID string

	lockManager *ProjectLockManager
	metrics     realtimeCounters

	mu    sync.RWMutex
	rooms map[int]map[*projectClient]struct{}
}

type projectClient struct {
	hub           *ProjectHub
	conn          *websocket.Conn
	session       service.ProjectRealtimeSession
	send          chan ProjectServerMessage
	lg            *zap.Logger
	sendMu        sync.Mutex
	closed        bool
	viewingTaskID int
}

// NewProjectHub 创建项目级实时 hub。
// 这里按 project 维度管理房间，是因为任务事件、presence 和元数据锁天然都归属于项目作用域。
func NewProjectHub(svc *service.TaskService, rdb *redis.Client, nodeID string, lockCache cache.Cache) *ProjectHub {
	return &ProjectHub{
		svc:         svc,
		rdb:         rdb,
		nodeID:      nodeID,
		lockManager: NewProjectLockManager(lockCache, projectLockTTL),
		rooms:       make(map[int]map[*projectClient]struct{}),
	}
}

// Start 启动 Redis 订阅和 presence 心跳两个后台循环。
// 一个负责跨节点转发项目事件，一个负责周期性广播在线快照。
func (h *ProjectHub) Start(ctx context.Context, lg *zap.Logger) {
	if h == nil || h.rdb == nil {
		return
	}

	go h.runPubSub(ctx, lg)
	go h.runPresenceHeartbeat(ctx, lg)
}

// HandleProjectConnection 把一个客户端接入项目房间，并在连接生命周期内维护 presence 与锁释放。
func (h *ProjectHub) HandleProjectConnection(ctx context.Context, conn *websocket.Conn, session service.ProjectRealtimeSession, initialCursor int64, lg *zap.Logger) {
	client := &projectClient{
		hub:     h,
		conn:    conn,
		session: session,
		send:    make(chan ProjectServerMessage, projectSendBufferSize),
		lg:      lg,
	}

	h.register(client)
	h.broadcastPresence(ctx, client.session.ProjectID, client.lg)
	defer func() {
		client.releaseAllLocks()
		h.unregister(client)
		h.broadcastPresence(ctx, client.session.ProjectID, client.lg)
	}()

	go client.writePump()
	client.sendInitialSync(ctx, initialCursor)
	client.readPump(ctx)
}

// BroadcastTaskEvent 既广播给当前节点的本地连接，也会投递到 Redis 供其他节点转发。
// 这样做的好处是“实时广播”和“多节点 fan-out”共用同一条入口。
func (h *ProjectHub) BroadcastTaskEvent(ctx context.Context, event models.TaskEvent) {
	if event.ProjectID <= 0 {
		return
	}

	msg := taskEventServerMessage(event, h.nodeID)
	h.broadcast(event.ProjectID, msg)
	h.publish(ctx, event.ProjectID, msg, zap.L())
}

func (h *ProjectHub) register(client *projectClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.session.ProjectID]
	if room == nil {
		room = make(map[*projectClient]struct{})
		h.rooms[client.session.ProjectID] = room
	}
	room[client] = struct{}{}
	h.metrics.connectionsAccepted.Add(1)
}

func (h *ProjectHub) unregister(client *projectClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.session.ProjectID]
	if room != nil {
		delete(room, client)
		if len(room) == 0 {
			delete(h.rooms, client.session.ProjectID)
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

func (h *ProjectHub) MetricsSnapshot() HubMetrics {
	if h == nil {
		return HubMetrics{Hub: "project"}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	rooms := make([]RoomMetrics, 0, len(h.rooms))
	totalConnections := 0
	globalUsers := make(map[int]struct{})
	for projectID, room := range h.rooms {
		roomUsers := make(map[int]struct{})
		for client := range room {
			totalConnections++
			roomUsers[client.session.UserID] = struct{}{}
			globalUsers[client.session.UserID] = struct{}{}
		}
		rooms = append(rooms, RoomMetrics{
			ID:          projectID,
			Connections: len(room),
			Users:       len(roomUsers),
		})
	}
	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].ID < rooms[j].ID
	})

	return HubMetrics{
		NodeID:            h.nodeID,
		Hub:               "project",
		ActiveRooms:       len(h.rooms),
		ActiveConnections: totalConnections,
		ActiveUsers:       len(globalUsers),
		Rooms:             rooms,
		Counters:          h.metrics.snapshot(),
	}
}

// presenceSnapshot 汇总某个项目房间当前所有连接，折叠成按用户维度聚合的在线快照。
func (h *ProjectHub) presenceSnapshot(projectID int) []ProjectPresence {
	h.mu.RLock()
	defer h.mu.RUnlock()

	room := h.rooms[projectID]
	if len(room) == 0 {
		return []ProjectPresence{}
	}

	byUserID := make(map[int]*ProjectPresence)
	for client := range room {
		item := byUserID[client.session.UserID]
		if item == nil {
			item = &ProjectPresence{
				UserID:   client.session.UserID,
				Username: client.session.Username,
			}
			byUserID[client.session.UserID] = item
		}
		item.Connections++
		if client.viewingTaskID > 0 && !intSliceContains(item.ViewingTaskIDs, client.viewingTaskID) {
			item.ViewingTaskIDs = append(item.ViewingTaskIDs, client.viewingTaskID)
			sort.Ints(item.ViewingTaskIDs)
		}
		if item.Username == "" {
			item.Username = client.session.Username
		}
	}

	presence := make([]ProjectPresence, 0, len(byUserID))
	for _, item := range byUserID {
		presence = append(presence, *item)
	}
	sort.Slice(presence, func(i, j int) bool {
		return presence[i].UserID < presence[j].UserID
	})
	return presence
}

func (h *ProjectHub) activeProjectIDs() []int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ids := make([]int, 0, len(h.rooms))
	for projectID := range h.rooms {
		ids = append(ids, projectID)
	}
	sort.Ints(ids)
	return ids
}

// broadcastPresence 生成并广播 presence 快照。
// 使用快照而不是增量事件，可以让新旧客户端都更容易做状态收敛。
func (h *ProjectHub) broadcastPresence(ctx context.Context, projectID int, lg *zap.Logger) {
	msg := ProjectServerMessage{
		Type:         ProjectMessageTypePresence,
		ProjectID:    projectID,
		Presence:     h.presenceSnapshot(projectID),
		ServerNodeID: h.nodeID,
	}
	h.broadcast(projectID, msg)
	h.publish(ctx, projectID, msg, lg)
}

// runPresenceHeartbeat 周期性给所有活跃项目广播在线快照。
// 即使没有新的业务事件，客户端也能靠心跳快照修正在线态。
func (h *ProjectHub) runPresenceHeartbeat(ctx context.Context, lg *zap.Logger) {
	ticker := time.NewTicker(projectPresencePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, projectID := range h.activeProjectIDs() {
				h.broadcastPresence(ctx, projectID, lg)
			}
		}
	}
}

// broadcast 只向同一项目房间里的本地连接广播消息。
func (h *ProjectHub) broadcast(projectID int, msg ProjectServerMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	h.metrics.broadcastMessages.Add(1)
	for client := range h.rooms[projectID] {
		select {
		case client.send <- msg:
		default:
			h.metrics.droppedClients.Add(1)
			go h.unregister(client)
		}
	}
}

func (h *ProjectHub) publish(ctx context.Context, projectID int, msg ProjectServerMessage, lg *zap.Logger) {
	if h.rdb == nil {
		return
	}

	payload, err := json.Marshal(projectPubSubEnvelope{
		OriginNodeID: h.nodeID,
		ProjectID:    projectID,
		Message:      msg,
	})
	if err != nil {
		lg.Warn("project.pubsub.marshal_failed", zap.Error(err))
		return
	}
	if err := h.rdb.Publish(ctx, projectPubSubChannel, payload).Err(); err != nil {
		h.metrics.pubSubPublishErrors.Add(1)
		lg.Warn("project.pubsub.publish_failed", zap.Int("project_id", projectID), zap.Error(err))
		return
	}
	h.metrics.pubSubPublished.Add(1)
}

func (h *ProjectHub) runPubSub(ctx context.Context, lg *zap.Logger) {
	pubsub := h.rdb.Subscribe(ctx, projectPubSubChannel)
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
			var env projectPubSubEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				lg.Warn("project.pubsub.unmarshal_failed", zap.Error(err))
				continue
			}
			if env.OriginNodeID == h.nodeID {
				continue
			}
			h.metrics.pubSubReceived.Add(1)
			h.broadcast(env.ProjectID, env.Message)
		}
	}
}

func (c *projectClient) readPump(ctx context.Context) {
	defer c.conn.Close()

	c.conn.SetReadLimit(projectMaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(projectPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(projectPongWait))
	})

	for {
		var msg ProjectClientMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.lg.Debug("project.ws.read_closed", zap.Int("project_id", c.session.ProjectID), zap.Error(err))
			}
			return
		}
		c.handleMessage(ctx, msg)
	}
}

func (c *projectClient) writePump() {
	ticker := time.NewTicker(projectPingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(projectWriteWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(projectWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *projectClient) sendInitialSync(ctx context.Context, cursor int64) {
	c.sendSync(ctx, cursor)
}

func (c *projectClient) handleMessage(ctx context.Context, msg ProjectClientMessage) {
	switch msg.Type {
	case ProjectMessageTypePing:
		c.enqueue(ProjectServerMessage{Type: ProjectMessageTypePong, ProjectID: c.session.ProjectID})
	case ProjectMessageTypeSync:
		c.sendSync(ctx, msg.Cursor)
	case ProjectMessageTypeLockRequest:
		c.hub.metrics.lockRequests.Add(1)
		c.handleLockRequest(ctx, msg)
	case ProjectMessageTypeLockRelease:
		c.handleLockRelease(ctx, msg)
	case ProjectMessageTypeViewDocument:
		c.setViewingTask(ctx, msg.TaskID)
	default:
		c.sendError(fmt.Sprintf("unsupported message type: %s", msg.Type))
	}
}

func (c *projectClient) setViewingTask(ctx context.Context, taskID int) {
	if taskID > 0 {
		if err := c.ensureCanLockTask(ctx, taskID); err != nil {
			c.sendError(err.Error())
			return
		}
	}
	c.hub.mu.Lock()
	c.viewingTaskID = taskID
	c.hub.mu.Unlock()
	c.hub.broadcastPresence(ctx, c.session.ProjectID, c.lg)
}

func (c *projectClient) handleLockRequest(ctx context.Context, msg ProjectClientMessage) {
	if c.hub.lockManager == nil {
		c.sendLockError(msg.TaskID, msg.Field, "lock manager is not configured")
		return
	}
	if err := c.ensureCanLockTask(ctx, msg.TaskID); err != nil {
		c.sendLockError(msg.TaskID, msg.Field, err.Error())
		return
	}

	lockMsg, err := c.hub.lockManager.Acquire(ctx, c, msg.TaskID, msg.Field)
	if err != nil {
		c.sendLockError(msg.TaskID, msg.Field, err.Error())
		return
	}
	c.broadcastLock(lockMsg)
}

func (c *projectClient) handleLockRelease(ctx context.Context, msg ProjectClientMessage) {
	lockMsg, err := c.hub.lockManager.Release(ctx, c, msg.TaskID, msg.Field)
	if err != nil {
		c.sendLockError(msg.TaskID, msg.Field, err.Error())
		return
	}
	c.broadcastLock(lockMsg)
}

func (c *projectClient) ensureCanLockTask(ctx context.Context, taskID int) error {
	if c.hub.svc == nil {
		return fmt.Errorf("task service is not configured")
	}
	session, err := c.hub.svc.OpenTaskContentSession(ctx, c.lg, c.session.UserID, taskID)
	if err != nil {
		return err
	}
	if session.ProjectID != c.session.ProjectID {
		return fmt.Errorf("task does not belong to current project")
	}
	if !session.CanEdit {
		return fmt.Errorf("no permission to lock task")
	}
	return nil
}

func (c *projectClient) releaseAllLocks() {
	if c.hub.lockManager == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, msg := range c.hub.lockManager.ReleaseClient(ctx, c) {
		c.broadcastLock(msg)
	}
}

func (c *projectClient) broadcastLock(msg ProjectServerMessage) {
	c.hub.broadcast(c.session.ProjectID, msg)
	c.hub.publish(context.Background(), c.session.ProjectID, msg, c.lg)
}

func (c *projectClient) sendLockError(taskID int, field string, message string) {
	c.hub.metrics.lockErrors.Add(1)
	c.enqueue(ProjectServerMessage{
		Type:      ProjectMessageTypeLockError,
		ProjectID: c.session.ProjectID,
		Error:     message,
		Lock: &ProjectLock{
			TaskID:         taskID,
			Field:          normalizeProjectLockField(field),
			HolderUserID:   c.session.UserID,
			HolderUsername: c.session.Username,
		},
	})
}

func (c *projectClient) sendSync(ctx context.Context, cursor int64) {
	result, err := c.hub.svc.SyncProjectEvents(ctx, c.lg, c.session.UserID, c.session.ProjectID, service.ProjectSyncInput{
		Cursor: cursor,
		Limit:  100,
	})
	if err != nil {
		c.sendError(err.Error())
		return
	}

	c.enqueue(ProjectServerMessage{
		Type:       ProjectMessageTypeInit,
		ProjectID:  c.session.ProjectID,
		Events:     result.Events,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func (c *projectClient) enqueue(msg ProjectServerMessage) {
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

func (c *projectClient) sendError(message string) {
	c.enqueue(ProjectServerMessage{
		Type:      ProjectMessageTypeError,
		ProjectID: c.session.ProjectID,
		Error:     message,
	})
}

func intSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func taskEventServerMessage(event models.TaskEvent, nodeID string) ProjectServerMessage {
	return ProjectServerMessage{
		Type:         taskEventMessageType(event.EventType),
		ProjectID:    event.ProjectID,
		EventID:      event.EventID,
		Cursor:       event.ID,
		Event:        &event,
		ServerNodeID: nodeID,
	}
}
