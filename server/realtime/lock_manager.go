package realtime

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
