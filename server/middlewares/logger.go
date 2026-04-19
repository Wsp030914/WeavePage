package middlewares

import (
	"ToDoList/server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set("request_id", reqID)
		c.Header("X-Request-ID", reqID)

		lg := zap.L().With(
			zap.String("request_id", reqID),
			zap.String("client_ip", c.ClientIP()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
		)
		c.Set("logger", lg)

		ctx := utils.WithLogger(c.Request.Context(), lg)
		ctx = utils.WithRequestID(ctx, reqID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		// 收尾：记录耗时/状态码/错误
		latency := time.Since(start)
		status := c.Writer.Status()
		lg.Info("access",
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.Int("size", c.Writer.Size()),
		)
	}
}

func RecoveryWithZap() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				lg, _ := c.Get("logger")
				logger, _ := lg.(*zap.Logger)
				if logger == nil {
					logger = zap.L()
				}

				logger.Error("panic recovered",
					zap.Any("panic", rec),
					zap.Stack("stack"),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 5000, "msg": "系统内部错误"})
			}
		}()
		c.Next()
	}
}
