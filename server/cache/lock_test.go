package cache

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type lockCacheStub struct {
	mu           sync.Mutex
	values       map[string]string
	refreshCount int
	releaseCount int
}

func newLockCacheStub() *lockCacheStub {
	return &lockCacheStub{values: make(map[string]string)}
}

func (s *lockCacheStub) Get(ctx context.Context, key string) (string, error) {
	panic("unexpected call to Get")
}

func (s *lockCacheStub) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	panic("unexpected call to MGet")
}

func (s *lockCacheStub) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	panic("unexpected call to Set")
}

func (s *lockCacheStub) Del(ctx context.Context, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.values, key)
	}
	return nil
}

func (s *lockCacheStub) Exists(ctx context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.values[key]
	return ok, nil
}

func (s *lockCacheStub) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = value.(string)
	return true, nil
}

func (s *lockCacheStub) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := keys[0]
	owner, _ := args[0].(string)
	current, ok := s.values[key]
	if !ok || current != owner {
		return int64(0), nil
	}

	if strings.Contains(script, "pexpire") {
		s.refreshCount++
		return int64(1), nil
	}

	delete(s.values, key)
	s.releaseCount++
	return int64(1), nil
}

func (s *lockCacheStub) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	panic("unexpected call to ZAdd")
}

func (s *lockCacheStub) ZRem(ctx context.Context, key string, members ...interface{}) error {
	panic("unexpected call to ZRem")
}

func (s *lockCacheStub) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	panic("unexpected call to ZRevRange")
}

func (s *lockCacheStub) ZCard(ctx context.Context, key string) (int64, error) {
	panic("unexpected call to ZCard")
}

func (s *lockCacheStub) Expire(ctx context.Context, key string, ttl time.Duration) error {
	panic("unexpected call to Expire")
}

// TestDistributedLockWatchdogRefreshesAndStopsOnRelease 验证看门狗会续租，并且在释放锁后停止续租。
func TestDistributedLockWatchdogRefreshesAndStopsOnRelease(t *testing.T) {
	t.Parallel()

	cache := newLockCacheStub()
	lock := NewDistributedLock(cache, "task:1", 1500*time.Millisecond)

	acquired, err := lock.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	time.Sleep(650 * time.Millisecond)

	cache.mu.Lock()
	refreshes := cache.refreshCount
	cache.mu.Unlock()
	if refreshes == 0 {
		t.Fatal("expected watchdog to refresh lock ttl at least once")
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	cache.mu.Lock()
	afterReleaseRefreshes := cache.refreshCount
	releases := cache.releaseCount
	cache.mu.Unlock()

	if releases != 1 {
		t.Fatalf("expected exactly one release call, got %d", releases)
	}

	time.Sleep(650 * time.Millisecond)

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.refreshCount != afterReleaseRefreshes {
		t.Fatalf("watchdog still refreshed after release: before=%d after=%d", afterReleaseRefreshes, cache.refreshCount)
	}
}
