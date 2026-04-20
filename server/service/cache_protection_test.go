package service

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import (
	"context"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

// TestLoadWithCacheProtection_AcquiredLockLoadsAndReleases 验证拿到保护锁时会执行加载并在结束后释放锁。
func TestLoadWithCacheProtection_AcquiredLockLoadsAndReleases(t *testing.T) {
	t.Parallel()

	lockCache := newTaskLockCacheStub()
	var loads atomic.Int32

	got, err := loadWithCacheProtection(
		context.Background(),
		zap.NewNop(),
		nil,
		lockCache,
		"unit:load",
		func(ctx context.Context) (interface{}, error) {
			loads.Add(1)
			return "loaded", nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("loadWithCacheProtection returned error: %v", err)
	}
	if got != "loaded" {
		t.Fatalf("unexpected result: got %#v", got)
	}
	if loads.Load() != 1 {
		t.Fatalf("expected one load, got %d", loads.Load())
	}

	lockCache.mu.Lock()
	defer lockCache.mu.Unlock()
	if lockCache.setNXCalls != 1 {
		t.Fatalf("expected one lock acquisition, got %d", lockCache.setNXCalls)
	}
	if lockCache.releaseCalls != 1 {
		t.Fatalf("expected one lock release, got %d", lockCache.releaseCalls)
	}
}

// TestLoadWithCacheProtection_WaitsForCacheWhenLockHeld 验证拿不到锁时会等待缓存回填，而不是直接重复回源。
func TestLoadWithCacheProtection_WaitsForCacheWhenLockHeld(t *testing.T) {
	t.Parallel()

	lockCache := newTaskLockCacheStub()
	lockCache.values["lock:cache_load:unit:wait"] = "other-owner"

	var reads atomic.Int32
	var loads atomic.Int32
	got, err := loadWithCacheProtection(
		context.Background(),
		zap.NewNop(),
		nil,
		lockCache,
		"unit:wait",
		func(ctx context.Context) (interface{}, error) {
			loads.Add(1)
			return "loaded", nil
		},
		func(ctx context.Context) (interface{}, bool, error) {
			if reads.Add(1) >= 2 {
				return "cached", true, nil
			}
			return nil, false, nil
		},
	)
	if err != nil {
		t.Fatalf("loadWithCacheProtection returned error: %v", err)
	}
	if got != "cached" {
		t.Fatalf("unexpected result: got %#v", got)
	}
	if loads.Load() != 0 {
		t.Fatalf("expected no direct load while waiting for cache, got %d", loads.Load())
	}
	if reads.Load() < 2 {
		t.Fatalf("expected cache polling, got %d reads", reads.Load())
	}
}
