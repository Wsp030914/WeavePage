package cache

// 文件说明：这个文件负责某类缓存或锁能力封装。
// 实现方式：统一封装缓存键与读写语义。
// 这样做的好处是缓存行为更一致，也更方便测试。
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"ToDoList/server/models"
)

var ErrCacheNotFound = errors.New("cache: user not found")

const (
	UserProfileExpire = 5 * time.Minute
	CacheNotFound     = "{}"
)

type UserCache interface {
	GetProfile(ctx context.Context, uid int) (*models.User, error)
	SetProfile(ctx context.Context, uid int, user *models.User) error
	DelProfile(ctx context.Context, uid int) error
	GetVersion(ctx context.Context, uid int) (int, error)
	SetVersion(ctx context.Context, uid int, version int) error
	GetJti(ctx context.Context, jti string) (bool, error)
	SetJti(ctx context.Context, jti string, exp time.Time) error
}

type userCache struct {
	cache Cache
	ttl   time.Duration
}

func NewUserCache(cache Cache) UserCache {
	return &userCache{cache: cache, ttl: UserProfileExpire}
}

func (c *userCache) profileKey(uid int) string {
	return fmt.Sprintf("user:profile:%d", uid)
}

func (c *userCache) versionKey(uid int) string {
	return fmt.Sprintf("user:version:%d", uid)
}

func (c *userCache) jtiKey(jti string) string {
	return fmt.Sprintf("jwt:jti:%s", jti)
}

func (c *userCache) GetProfile(ctx context.Context, uid int) (*models.User, error) {
	key := c.profileKey(uid)
	val, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if val == CacheNotFound {
		return nil, ErrCacheNotFound
	}

	var user models.User
	if err := json.Unmarshal([]byte(val), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *userCache) SetProfile(ctx context.Context, uid int, user *models.User) error {
	key := c.profileKey(uid)

	// 防穿透：缓存空对象
	if user == nil {
		return c.cache.Set(ctx, key, CacheNotFound, 30*time.Second)
	}

	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	// 防雪崩：随机过期时间 (5分钟 + 0~60秒随机)
	ttl := c.ttl + time.Duration(rand.Intn(60))*time.Second
	return c.cache.Set(ctx, key, string(data), ttl)
}

func (c *userCache) DelProfile(ctx context.Context, uid int) error {
	key := c.profileKey(uid)
	return c.cache.Del(ctx, key)
}

func (c *userCache) GetVersion(ctx context.Context, uid int) (int, error) {
	key := c.versionKey(uid)
	val, err := c.cache.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	var version int
	fmt.Sscanf(val, "%d", &version)
	return version, nil
}

func (c *userCache) SetVersion(ctx context.Context, uid int, version int) error {
	key := c.versionKey(uid)
	return c.cache.Set(ctx, key, fmt.Sprintf("%d", version), c.ttl)
}

func (c *userCache) GetJti(ctx context.Context, jti string) (bool, error) {
	key := c.jtiKey(jti)
	return c.cache.Exists(ctx, key)
}

func (c *userCache) SetJti(ctx context.Context, jti string, exp time.Time) error {
	key := c.jtiKey(jti)
	ttl := time.Until(exp)
	if ttl <= 0 {
		return nil
	}
	return c.cache.Set(ctx, key, "1", ttl)
}
