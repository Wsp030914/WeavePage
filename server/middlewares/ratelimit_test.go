package middlewares

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

type capturingRateLimitStore struct {
	allowed bool
	err     error

	mu   sync.Mutex
	keys []string
}

func (s *capturingRateLimitStore) Allow(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append(s.keys, key)
	return s.allowed, s.err
}

func (s *capturingRateLimitStore) firstKey(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys) == 0 {
		t.Fatal("expected rate limit store to receive a key")
	}
	return s.keys[0]
}

func TestRateLimitMiddleware_LocalStoreBlocksBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(rateLimitMiddlewareWithStore(newLocalRateLimitStore(1, 1)))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusNoContent)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddleware_UsesUIDWhenAuthAlreadyRan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &capturingRateLimitStore{allowed: true}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("uid", 42)
		c.Next()
	})
	router.Use(rateLimitMiddlewareWithStore(store))
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if got, want := store.firstKey(t), "uid:42|GET|/protected"; got != want {
		t.Fatalf("rate limit key = %q, want %q", got, want)
	}
}

func TestRateLimitMiddleware_AllowsFallbackSuccessWhenStoreReturnsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &capturingRateLimitStore{
		allowed: true,
		err:     errors.New("redis unavailable"),
	}
	router := gin.New()
	router.Use(rateLimitMiddlewareWithStore(store))
	router.GET("/fallback", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fallback", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestParseRedisAllowedRejectsUnknownResult(t *testing.T) {
	if _, err := parseRedisAllowed(struct{}{}); err == nil {
		t.Fatal("expected error for unexpected redis result type")
	}
}
