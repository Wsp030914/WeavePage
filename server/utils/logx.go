package utils

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func CtxLogger(c *gin.Context) *zap.Logger {
	if v, ok := c.Get("logger"); ok {
		if lg, ok := v.(*zap.Logger); ok && lg != nil {
			return lg
		}
	}
	return zap.L()
}
