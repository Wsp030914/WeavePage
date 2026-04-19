package middlewares

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

func (s *localRateLimitStore) Allow(_ context.Context, key string) (bool, error) {
	limiter := s.getVisitor(key)
	return limiter.Allow(), nil
}

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
