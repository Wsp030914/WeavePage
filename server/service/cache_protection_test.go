package service

import (
	"context"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

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
