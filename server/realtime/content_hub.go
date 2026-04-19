package realtime

import (
	"ToDoList/server/models"
	"ToDoList/server/service"
	"context"
	"encoding/json"
	"fmt"
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
	svc    *service.TaskService
	rdb    *redis.Client
	nodeID string

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

// NewContentHub creates a task body collaboration hub.
func NewContentHub(svc *service.TaskService, rdb *redis.Client, nodeID string) *ContentHub {
	return &ContentHub{
		svc:    svc,
		rdb:    rdb,
		nodeID: nodeID,
		rooms:  make(map[int]map[*contentClient]struct{}),
	}
}

// Start subscribes to Redis fan-out messages until ctx is canceled.
func (h *ContentHub) Start(ctx context.Context, lg *zap.Logger) {
	if h == nil || h.rdb == nil {
		return
	}

	go h.runPubSub(ctx, lg)
}

// HandleConnection registers a WebSocket connection into a task content room.
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
	}
}

func (h *ContentHub) broadcast(taskID int, msg ContentServerMessage, exclude *contentClient) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.rooms[taskID] {
		if client == exclude {
			continue
		}
		select {
		case client.send <- msg:
		default:
			go h.unregister(client)
		}
	}
}

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
		lg.Warn("content.pubsub.publish_failed", zap.Int("task_id", taskID), zap.Error(err))
	}
}

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
			h.broadcast(env.TaskID, env.Message, nil)
		}
	}
}

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
		return
	}

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
