package realtime

// 文件说明：这个文件实现项目元数据锁管理。
// 实现方式：基于缓存锁按任务字段维度做占用、释放和状态查询。
// 这样做的好处是多人同时编辑元数据时能显式暴露冲突，减少静默覆盖。
import (
	"ToDoList/server/cache"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	projectLockDefaultField = "metadata"
	projectLockTTL          = 30 * time.Second
)

var (
	ErrProjectLockUnavailable = errors.New("project lock manager unavailable")
	ErrProjectLockHeld        = errors.New("project lock already held")
	ErrProjectLockNotHeld     = errors.New("project lock not held by connection")
)

type ProjectLockManager struct {
	cache cache.Cache
	ttl   time.Duration

	mu    sync.Mutex
	locks map[*projectClient]map[string]*heldProjectLock
}

type heldProjectLock struct {
	key       string
	projectID int
	taskID    int
	field     string
	lock      *cache.DistributedLock
	holderID  int
	holder    string
	createdAt time.Time
}

// NewProjectLockManager 创建项目元数据锁管理器。
// 这里把锁状态同时保存在本机内存和分布式锁里：前者便于快速判断“当前连接是否已持有锁”，
// 后者负责多实例之间的互斥。
func NewProjectLockManager(cacheClient cache.Cache, ttl time.Duration) *ProjectLockManager {
	if ttl <= 0 {
		ttl = projectLockTTL
	}
	return &ProjectLockManager{
		cache: cacheClient,
		ttl:   ttl,
		locks: make(map[*projectClient]map[string]*heldProjectLock),
	}
}

// Acquire 为某个任务字段申请元数据锁。
// 如果当前连接已经持有同一把锁，直接返回已持有状态，避免重复加锁导致自身冲突。
func (m *ProjectLockManager) Acquire(ctx context.Context, client *projectClient, taskID int, field string) (ProjectServerMessage, error) {
	if m == nil || m.cache == nil {
		return ProjectServerMessage{}, ErrProjectLockUnavailable
	}
	if taskID <= 0 {
		return ProjectServerMessage{}, fmt.Errorf("invalid task id: %d", taskID)
	}

	field = normalizeProjectLockField(field)
	key := projectLockKey(taskID, field)

	m.mu.Lock()
	if existing := m.heldByClientLocked(client, key); existing != nil {
		msg := projectLockMessage(ProjectMessageTypeTaskLocked, existing, client.hub.nodeID)
		m.mu.Unlock()
		return msg, nil
	}
	m.mu.Unlock()

	lock := cache.NewDistributedLock(m.cache, key, m.ttl)
	acquired, err := lock.Acquire(ctx)
	if err != nil {
		return ProjectServerMessage{}, fmt.Errorf("acquire project lock: %w", err)
	}
	if !acquired {
		return ProjectServerMessage{}, ErrProjectLockHeld
	}

	held := &heldProjectLock{
		key:       key,
		projectID: client.session.ProjectID,
		taskID:    taskID,
		field:     field,
		lock:      lock,
		holderID:  client.session.UserID,
		holder:    client.session.Username,
		createdAt: time.Now(),
	}

	m.mu.Lock()
	if m.locks[client] == nil {
		m.locks[client] = make(map[string]*heldProjectLock)
	}
	m.locks[client][key] = held
	m.mu.Unlock()

	return projectLockMessage(ProjectMessageTypeTaskLocked, held, client.hub.nodeID), nil
}

// Release 释放某个连接持有的字段锁。
// 只有本连接自己持有的锁才能释放，避免别人误释放当前编辑者的锁。
func (m *ProjectLockManager) Release(ctx context.Context, client *projectClient, taskID int, field string) (ProjectServerMessage, error) {
	if m == nil || m.cache == nil {
		return ProjectServerMessage{}, ErrProjectLockUnavailable
	}
	field = normalizeProjectLockField(field)
	key := projectLockKey(taskID, field)

	m.mu.Lock()
	held := m.heldByClientLocked(client, key)
	if held == nil {
		m.mu.Unlock()
		return ProjectServerMessage{}, ErrProjectLockNotHeld
	}
	delete(m.locks[client], key)
	if len(m.locks[client]) == 0 {
		delete(m.locks, client)
	}
	m.mu.Unlock()

	if err := held.lock.Release(ctx); err != nil {
		return ProjectServerMessage{}, fmt.Errorf("release project lock: %w", err)
	}
	return projectLockMessage(ProjectMessageTypeTaskUnlocked, held, client.hub.nodeID), nil
}

// ReleaseClient 在连接关闭时释放该连接持有的全部锁。
// 这样做的好处是浏览器关闭、网络断开或页面跳转时不会留下脏锁。
func (m *ProjectLockManager) ReleaseClient(ctx context.Context, client *projectClient) []ProjectServerMessage {
	if m == nil || m.cache == nil {
		return nil
	}

	m.mu.Lock()
	heldLocks := m.locks[client]
	if len(heldLocks) == 0 {
		m.mu.Unlock()
		return nil
	}
	delete(m.locks, client)

	release := make([]*heldProjectLock, 0, len(heldLocks))
	for _, held := range heldLocks {
		release = append(release, held)
	}
	m.mu.Unlock()

	messages := make([]ProjectServerMessage, 0, len(release))
	for _, held := range release {
		if err := held.lock.Release(ctx); err != nil {
			continue
		}
		messages = append(messages, projectLockMessage(ProjectMessageTypeTaskUnlocked, held, client.hub.nodeID))
	}
	return messages
}

func (m *ProjectLockManager) heldByClientLocked(client *projectClient, key string) *heldProjectLock {
	if client == nil {
		return nil
	}
	return m.locks[client][key]
}

func normalizeProjectLockField(field string) string {
	field = strings.TrimSpace(strings.ToLower(field))
	if field == "" {
		return projectLockDefaultField
	}
	return strings.ReplaceAll(field, ":", "_")
}

func projectLockKey(taskID int, field string) string {
	return fmt.Sprintf("collab:task:%d:%s", taskID, field)
}

func projectLockMessage(messageType string, held *heldProjectLock, nodeID string) ProjectServerMessage {
	return ProjectServerMessage{
		Type:         messageType,
		ProjectID:    held.projectID,
		ServerNodeID: nodeID,
		Lock: &ProjectLock{
			TaskID:         held.taskID,
			Field:          held.field,
			HolderUserID:   held.holderID,
			HolderUsername: held.holder,
		},
	}
}
