package cache

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

const minWatchdogInterval = 500 * time.Millisecond

type DistributedLock struct {
	cache      Cache
	key        string
	identifier string
	ttl        time.Duration

	//增加watchdog来确保持有锁的进程任务执行
	//mu防止多个goroutine去执行对watchdog的操作
	mu             sync.Mutex
	watchdogCancel context.CancelFunc
	watchdogDone   chan struct{}
	watchdogToken  uint64
}

func NewDistributedLock(cache Cache, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		cache:      cache,
		key:        "lock:" + key,
		identifier: uuid.New().String(),
		ttl:        ttl,
	}
}

func (l *DistributedLock) Acquire(ctx context.Context) (bool, error) {
	success, err := l.cache.SetNX(ctx, l.key, l.identifier, l.ttl)
	if err != nil {
		return false, err
	}
	if success {
		l.startWatchdog()
	}
	return success, nil
}

func (l *DistributedLock) Release(ctx context.Context) error {
	l.stopWatchdog(ctx)

	const script = `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	_, err := l.cache.Eval(ctx, script, []string{l.key}, l.identifier)
	if err != nil {
		return err
	}
	return nil
}

func (l *DistributedLock) startWatchdog() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.watchdogCancel != nil {
		return
	}

	watchdogCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	token := l.watchdogToken + 1
	l.watchdogToken = token
	l.watchdogCancel = cancel
	l.watchdogDone = done

	go func(token uint64) {
		defer close(done)
		defer l.finishWatchdog(token, done)

		ticker := time.NewTicker(watchdogInterval(l.ttl))
		defer ticker.Stop()

		for {
			select {
			case <-watchdogCtx.Done():
				return
			case <-ticker.C:
				ok, err := l.refresh(watchdogCtx)
				if err != nil || !ok {
					return
				}
			}
		}
	}(token)
}

func (l *DistributedLock) finishWatchdog(token uint64, done chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.watchdogToken == token && l.watchdogDone == done {
		l.watchdogCancel = nil
		l.watchdogDone = nil
	}
}

func (l *DistributedLock) stopWatchdog(ctx context.Context) {
	l.mu.Lock()
	cancel := l.watchdogCancel
	done := l.watchdogDone
	l.watchdogCancel = nil
	l.watchdogDone = nil
	l.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()

	if done == nil {
		return
	}

	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (l *DistributedLock) refresh(ctx context.Context) (bool, error) {
	const script = `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	res, err := l.cache.Eval(ctx, script, []string{l.key}, l.identifier, ttlMilliseconds(l.ttl))
	if err != nil {
		return false, err
	}

	switch v := res.(type) {
	case int64:
		return v == 1, nil
	case int:
		return v == 1, nil
	case uint64:
		return v == 1, nil
	default:
		return false, nil
	}
}

func watchdogInterval(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return minWatchdogInterval
	}
	interval := ttl / 3
	if interval < minWatchdogInterval {
		return minWatchdogInterval
	}
	return interval
}

func ttlMilliseconds(ttl time.Duration) int64 {
	if ttl <= 0 {
		return 1
	}
	ms := ttl.Milliseconds()
	if ms <= 0 {
		return 1
	}
	return ms
}
