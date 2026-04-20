package cache

// 文件说明：这个文件负责某类缓存或锁能力封装。
// 实现方式：统一封装缓存键与读写语义。
// 这样做的好处是缓存行为更一致，也更方便测试。
import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrCacheMiss = redis.Nil
	ErrCacheNull = errors.New("cache: null object")
)

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	MGet(ctx context.Context, keys ...string) ([]interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	ZAdd(ctx context.Context, key string, members ...redis.Z) error
	ZRem(ctx context.Context, key string, members ...interface{}) error
	ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZCard(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) Cache {
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *RedisCache) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	return c.client.MGet(ctx, keys...).Result()
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	var val string
	switch v := value.(type) {
	case string:
		val = v
	default:
		val = interfaceToString(v)
	}
	return c.client.Set(ctx, key, val, ttl).Err()
}

func (c *RedisCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	var val string
	switch v := value.(type) {
	case string:
		val = v
	default:
		val = interfaceToString(v)
	}
	return c.client.SetNX(ctx, key, val, ttl).Result()
}

func (c *RedisCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *RedisCache) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return c.client.Eval(ctx, script, keys, args...).Result()
}

func (c *RedisCache) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return c.client.ZAdd(ctx, key, members...).Err()
}

func (c *RedisCache) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.ZRem(ctx, key, members...).Err()
}

func (c *RedisCache) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRevRange(ctx, key, start, stop).Result()
}

func (c *RedisCache) ZCard(ctx context.Context, key string) (int64, error) {
	return c.client.ZCard(ctx, key).Result()
}

func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

func interfaceToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}
