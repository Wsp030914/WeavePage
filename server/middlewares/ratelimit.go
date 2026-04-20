package middlewares

// 文件说明：这个文件承载 HTTP 横切逻辑。
// 实现方式：把鉴权、日志、跨域和限流等通用逻辑集中到中间件层。
// 这样做的好处是接口层更干净，公共策略也更容易统一维护。
import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"ToDoList/server/utils"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const (
	visitorTTL          = 5 * time.Minute
	cleanupInterval     = time.Minute
	redisLimiterTTL     = 5 * time.Minute
	redisLimiterTimeout = 50 * time.Millisecond
)

const redisTokenBucketScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(data[1])
local ts = tonumber(data[2])

if tokens == nil then
	tokens = capacity
	ts = now
end
if ts == nil then
	ts = now
end

local delta = math.max(0, now - ts)
tokens = math.min(capacity, tokens + (delta * rate / 1000))

local allowed = 0
if tokens >= 1 then
	allowed = 1
	tokens = tokens - 1
end

redis.call("HSET", key, "tokens", tokens, "ts", now)
redis.call("PEXPIRE", key, ttl)
return allowed
`

type rateLimitStore interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type localRateLimitStore struct {
	rps         int
	burst       int
	visitors    map[string]*visitor
	visitorsMu  sync.Mutex
	cleanupOnce sync.Once
}

func newLocalRateLimitStore(rps int, burst int) *localRateLimitStore {
	store := &localRateLimitStore{
		rps:      rps,
		burst:    burst,
		visitors: make(map[string]*visitor),
	}
	store.startCleanup()
	return store
}

// startCleanup 定时清理长时间未访问的 visitor，避免本机限流表无限增长。
func (s *localRateLimitStore) startCleanup() {
	s.cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(cleanupInterval)
			defer ticker.Stop()
			for now := range ticker.C {
				s.visitorsMu.Lock()
				for key, v := range s.visitors {
					if now.Sub(v.lastSeen) > visitorTTL {
						delete(s.visitors, key)
					}
				}
				s.visitorsMu.Unlock()
			}
		}()
	})
}

// Allow 对本机内存里的 limiter 做一次令牌消费。
func (s *localRateLimitStore) Allow(_ context.Context, key string) (bool, error) {
	limiter := s.getVisitor(key)
	return limiter.Allow(), nil
}

// getVisitor 懒加载 limiter，并在命中时刷新最后访问时间。
// 这样做的好处是常用 key 可以复用已有 limiter，冷 key 又能被后续清理掉。
func (s *localRateLimitStore) getVisitor(key string) *rate.Limiter {
	s.visitorsMu.Lock()
	defer s.visitorsMu.Unlock()

	if v, ok := s.visitors[key]; ok {
		v.lastSeen = time.Now()
		return v.limiter
	}
	limiter := rate.NewLimiter(rate.Limit(s.rps), s.burst)
	s.visitors[key] = &visitor{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

type redisRateLimitStore struct {
	client        *redis.Client
	rps           int
	burst         int
	ttl           time.Duration
	timeout       time.Duration
	localFallback *localRateLimitStore
}

func newRedisRateLimitStore(client *redis.Client, rps int, burst int) *redisRateLimitStore {
	return &redisRateLimitStore{
		client:        client,
		rps:           rps,
		burst:         burst,
		ttl:           redisLimiterTTL,
		timeout:       redisLimiterTimeout,
		localFallback: newLocalRateLimitStore(rps, burst),
	}
}

// Allow 优先走 Redis 分布式 token bucket。
// 如果 Redis 超时或报错，就回退到本机 limiter，避免把限流组件故障放大成全站不可用。
func (s *redisRateLimitStore) Allow(ctx context.Context, key string) (bool, error) {
	if s.client == nil {
		return s.localFallback.Allow(ctx, key)
	}

	redisCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.client.Eval(
		redisCtx,
		redisTokenBucketScript,
		[]string{redisRateLimitKey(key)},
		time.Now().UnixMilli(),
		s.rps,
		s.burst,
		s.ttl.Milliseconds(),
	).Result()
	if err != nil {
		allowed, fallbackErr := s.localFallback.Allow(ctx, key)
		if fallbackErr != nil {
			return allowed, fmt.Errorf("redis rate limit failed: %w; local fallback failed: %v", err, fallbackErr)
		}
		return allowed, fmt.Errorf("redis rate limit failed, used local fallback: %w", err)
	}

	return parseRedisAllowed(result)
}

// parseRedisAllowed 兼容 Redis Eval 可能返回的几种基础类型。
// 这样做可以减少不同客户端或不同脚本返回格式带来的耦合。
func parseRedisAllowed(result interface{}) (bool, error) {
	switch v := result.(type) {
	case int64:
		return v == 1, nil
	case int:
		return v == 1, nil
	case string:
		return v == "1", nil
	case []byte:
		return string(v) == "1", nil
	default:
		return false, fmt.Errorf("unexpected redis rate limit result %T", result)
	}
}

func redisRateLimitKey(key string) string {
	return "rate_limit:" + key
}

func RateLimitMiddleware(rps int, burst int) gin.HandlerFunc {
	return rateLimitMiddlewareWithStore(newLocalRateLimitStore(rps, burst))
}

func RedisRateLimitMiddleware(client *redis.Client, rps int, burst int) gin.HandlerFunc {
	if client == nil {
		return RateLimitMiddleware(rps, burst)
	}
	return rateLimitMiddlewareWithStore(newRedisRateLimitStore(client, rps, burst))
}

// rateLimitMiddlewareWithStore 把具体限流存储实现包成 Gin 中间件。
// 中间件本身只关心“是否允许”，把状态细节完全留给 store 实现。
func rateLimitMiddlewareWithStore(store rateLimitStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, err := store.Allow(c.Request.Context(), buildRateLimitKey(c))
		if err != nil {
			utils.CtxLogger(c).Warn("rate_limit_fallback", zap.Error(err))
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "too many requests",
			})
			return
		}
		c.Next()
	}
}

// buildRateLimitKey 优先按登录用户维度限流，未登录时回退到 IP。
// 同时把路由和方法拼进 key，避免不同接口共享同一个配额桶。
func buildRateLimitKey(c *gin.Context) string {
	identity := c.ClientIP()
	if uid := c.GetInt("uid"); uid > 0 {
		identity = "uid:" + strconv.Itoa(uid)
	}
	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}
	return identity + "|" + c.Request.Method + "|" + route
}
