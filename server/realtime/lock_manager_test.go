package realtime

import (
	"ToDoList/server/service"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type projectLockCacheStub struct {
	mu     sync.Mutex
	values map[string]string
}

func newProjectLockCacheStub() *projectLockCacheStub {
	return &projectLockCacheStub{values: make(map[string]string)}
}

func (s *projectLockCacheStub) Get(ctx context.Context, key string) (string, error) {
	panic("unexpected call to Get")
}

func (s *projectLockCacheStub) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	panic("unexpected call to MGet")
}

func (s *projectLockCacheStub) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	panic("unexpected call to Set")
}

func (s *projectLockCacheStub) Del(ctx context.Context, keys ...string) error {
	panic("unexpected call to Del")
}

func (s *projectLockCacheStub) Exists(ctx context.Context, key string) (bool, error) {
	panic("unexpected call to Exists")
}

func (s *projectLockCacheStub) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = value.(string)
	return true, nil
}

func (s *projectLockCacheStub) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := keys[0]
	owner, _ := args[0].(string)
	current, ok := s.values[key]
	if !ok || current != owner {
		return int64(0), nil
	}

	if strings.Contains(script, "pexpire") {
		return int64(1), nil
	}

	delete(s.values, key)
	return int64(1), nil
}

func (s *projectLockCacheStub) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	panic("unexpected call to ZAdd")
}

func (s *projectLockCacheStub) ZRem(ctx context.Context, key string, members ...interface{}) error {
	panic("unexpected call to ZRem")
}

func (s *projectLockCacheStub) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	panic("unexpected call to ZRevRange")
}

func (s *projectLockCacheStub) ZCard(ctx context.Context, key string) (int64, error) {
	panic("unexpected call to ZCard")
}

func (s *projectLockCacheStub) Expire(ctx context.Context, key string, ttl time.Duration) error {
	panic("unexpected call to Expire")
}

func TestProjectLockManager_AcquireRejectsConcurrentHolderAndReleases(t *testing.T) {
	t.Parallel()

	lockCache := newProjectLockCacheStub()
	hub := NewProjectHub(nil, nil, "node-a", lockCache)
	clientA := projectLockClient(hub, 7, "alice")
	clientB := projectLockClient(hub, 8, "bob")

	msg, err := hub.lockManager.Acquire(context.Background(), clientA, 12, "")
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	if msg.Type != ProjectMessageTypeTaskLocked {
		t.Fatalf("expected TASK_LOCKED, got %s", msg.Type)
	}
	if msg.Lock == nil || msg.Lock.Field != projectLockDefaultField || msg.Lock.HolderUserID != 7 {
		t.Fatalf("unexpected lock payload: %+v", msg.Lock)
	}

	_, err = hub.lockManager.Acquire(context.Background(), clientB, 12, projectLockDefaultField)
	if !errors.Is(err, ErrProjectLockHeld) {
		t.Fatalf("expected ErrProjectLockHeld, got %v", err)
	}

	msg, err = hub.lockManager.Release(context.Background(), clientA, 12, "")
	if err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if msg.Type != ProjectMessageTypeTaskUnlocked {
		t.Fatalf("expected TASK_UNLOCKED, got %s", msg.Type)
	}

	lockCache.mu.Lock()
	defer lockCache.mu.Unlock()
	if len(lockCache.values) != 0 {
		t.Fatalf("expected lock cache to be empty after release, got %d", len(lockCache.values))
	}
}

func TestProjectLockManager_ReleaseClientUnlocksAllHeldLocks(t *testing.T) {
	t.Parallel()

	lockCache := newProjectLockCacheStub()
	hub := NewProjectHub(nil, nil, "node-a", lockCache)
	client := projectLockClient(hub, 7, "alice")

	if _, err := hub.lockManager.Acquire(context.Background(), client, 12, "title"); err != nil {
		t.Fatalf("Acquire title returned error: %v", err)
	}
	if _, err := hub.lockManager.Acquire(context.Background(), client, 13, "status"); err != nil {
		t.Fatalf("Acquire status returned error: %v", err)
	}

	messages := hub.lockManager.ReleaseClient(context.Background(), client)
	if len(messages) != 2 {
		t.Fatalf("expected 2 unlock messages, got %d", len(messages))
	}
	for _, msg := range messages {
		if msg.Type != ProjectMessageTypeTaskUnlocked {
			t.Fatalf("expected TASK_UNLOCKED, got %s", msg.Type)
		}
	}
}

func projectLockClient(hub *ProjectHub, userID int, username string) *projectClient {
	return &projectClient{
		hub: hub,
		session: service.ProjectRealtimeSession{
			UserID:    userID,
			Username:  username,
			ProjectID: 9,
		},
		send: make(chan ProjectServerMessage, 1),
	}
}
