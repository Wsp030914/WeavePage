package service

// 文件说明：这个文件负责某类业务编排逻辑。
// 实现方式：服务层组合 repo、cache、bus 与外部依赖完成业务闭环。
// 这样做的好处是复杂规则集中，跨层协作边界清晰。
import (
	"context"
	"fmt"
	"time"

	"ToDoList/server/cache"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const (
	cacheLoadLockTTL      = 5 * time.Second
	cacheLoadLockWait     = 250 * time.Millisecond
	cacheLoadPollInterval = 25 * time.Millisecond
	cacheLoadReleaseWait  = time.Second
)

type protectedCacheLoadFunc func(ctx context.Context) (interface{}, error)
type protectedCacheReadFunc func(ctx context.Context) (value interface{}, ok bool, err error)

func loadWithCacheProtection(
	ctx context.Context,
	lg *zap.Logger,
	sf *singleflight.Group,
	cacheClient cache.Cache,
	key string,
	load protectedCacheLoadFunc,
	read protectedCacheReadFunc,
) (interface{}, error) {
	if sf == nil {
		return loadWithDistributedCacheLock(ctx, lg, cacheClient, key, load, read)
	}

	value, err, _ := sf.Do(key, func() (interface{}, error) {
		return loadWithDistributedCacheLock(ctx, lg, cacheClient, key, load, read)
	})
	return value, err
}

func loadWithDistributedCacheLock(
	ctx context.Context,
	lg *zap.Logger,
	cacheClient cache.Cache,
	key string,
	load protectedCacheLoadFunc,
	read protectedCacheReadFunc,
) (interface{}, error) {
	if cacheClient == nil {
		return load(ctx)
	}

	lock := cache.NewDistributedLock(cacheClient, "cache_load:"+key, cacheLoadLockTTL)
	acquired, err := acquireCacheLoadLock(ctx, lock)
	if err != nil {
		lg.Warn("cache.load.lock_acquire_failed", zap.String("key", key), zap.Error(err))
		return load(ctx)
	}
	if !acquired {
		if read != nil {
			value, ok, waitErr := waitForProtectedCache(ctx, read)
			if waitErr != nil {
				return nil, waitErr
			}
			if ok {
				return value, nil
			}
		}
		lg.Warn("cache.load.lock_wait_timeout", zap.String("key", key))
		return load(ctx)
	}

	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), cacheLoadReleaseWait)
		defer cancel()
		if err := lock.Release(releaseCtx); err != nil {
			lg.Warn("cache.load.lock_release_failed", zap.String("key", key), zap.Error(err))
		}
	}()

	return load(ctx)
}

func acquireCacheLoadLock(ctx context.Context, lock *cache.DistributedLock) (bool, error) {
	deadline := time.Now().Add(cacheLoadLockWait)
	for {
		acquired, err := lock.Acquire(ctx)
		if acquired || err != nil {
			return acquired, err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, nil
		}
		sleep := cacheLoadPollInterval
		if remaining < sleep {
			sleep = remaining
		}

		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, fmt.Errorf("waiting for cache load lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func waitForProtectedCache(ctx context.Context, read protectedCacheReadFunc) (interface{}, bool, error) {
	waitCtx, cancel := context.WithTimeout(ctx, cacheLoadLockWait)
	defer cancel()

	for {
		value, ok, err := read(waitCtx)
		if err != nil || ok {
			return value, ok, err
		}

		timer := time.NewTimer(cacheLoadPollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return nil, false, fmt.Errorf("waiting for protected cache: %w", ctx.Err())
			}
			return nil, false, nil
		case <-timer.C:
		}
	}
}
