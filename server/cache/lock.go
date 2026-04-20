package cache

// 文件说明：这个文件负责某类缓存或锁能力封装。
// 实现方式：统一封装缓存键与读写语义。
// 这样做的好处是缓存行为更一致，也更方便测试。
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

	// watchdog 用来在业务执行时间超过锁 TTL 时自动续期，
	// 避免长事务尚未完成时锁先过期，导致其他实例误以为可以进入临界区。
	// mu 用来保护 watchdog 的启动、停止与 token 切换，防止多个 goroutine 并发操作内部状态。
	mu             sync.Mutex
	watchdogCancel context.CancelFunc
	watchdogDone   chan struct{}
	watchdogToken  uint64
}

// NewDistributedLock 创建一个带唯一持有者标识的分布式锁实例。
// 这里给每把锁生成独立 identifier，是为了在释放和续期时校验“当前进程仍然是锁持有者”，
// 避免误删其他实例已经重新拿到的锁。
func NewDistributedLock(cache Cache, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		cache:      cache,
		key:        "lock:" + key,
		identifier: uuid.New().String(),
		ttl:        ttl,
	}
}

// Acquire 尝试获取锁；只有真正拿到锁时才启动 watchdog。
// 这样做的好处是把续期成本严格限制在持锁路径上，未获取成功的请求不会带来额外后台任务。
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

// Release 在释放锁前先停止 watchdog，避免后台续期协程和释放操作并发交错。
// 释放时使用 Lua 做“值相等再删除”，可以保证只有锁持有者本人能删除锁。
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

// startWatchdog 为当前锁启动自动续期协程。
// 续期频率使用 TTL 的三分之一，是为了在不过度刷 Redis 的前提下，
// 给网络抖动和调度延迟留出足够缓冲。
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

// finishWatchdog 只在 token 匹配时清空 watchdog 状态，防止旧协程退出时把新协程状态误清掉。
func (l *DistributedLock) finishWatchdog(token uint64, done chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.watchdogToken == token && l.watchdogDone == done {
		l.watchdogCancel = nil
		l.watchdogDone = nil
	}
}

// stopWatchdog 负责取消续期协程，并在上下文允许的情况下等待其退出。
// 这样可以减少续期和释放交错造成的时序问题。
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

// refresh 通过 Lua 校验“当前值仍然是自己”之后再续期。
// 这样做的好处是锁一旦被别人接管，旧持有者不会继续错误地延长别人的锁。
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
