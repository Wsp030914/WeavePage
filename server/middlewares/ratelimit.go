package middlewares

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	visitors    = make(map[string]*visitor)
	visitorsMu  sync.Mutex
	cleanupOnce sync.Once
)

const (
	visitorTTL      = 5 * time.Minute
	cleanupInterval = time.Minute
)

func startVisitorCleanup() {
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(cleanupInterval)
			defer ticker.Stop()
			for now := range ticker.C {
				visitorsMu.Lock()
				for key, v := range visitors {
					if now.Sub(v.lastSeen) > visitorTTL {
						delete(visitors, key)
					}
				}
				visitorsMu.Unlock()
			}
		}()
	})
}

func getVisitor(key string, rps int, burst int) *rate.Limiter {
	visitorsMu.Lock()
	defer visitorsMu.Unlock()

	if v, ok := visitors[key]; ok {
		v.lastSeen = time.Now()
		return v.limiter
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	visitors[key] = &visitor{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

func RateLimitMiddleware(rps int, burst int) gin.HandlerFunc {
	startVisitorCleanup()
	return func(c *gin.Context) {
		key := c.ClientIP()
		if uid := c.GetInt("uid"); uid > 0 {
			key = "uid:" + strconv.Itoa(uid)
		}
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		limiter := getVisitor(key+"|"+route, rps, burst)
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}
