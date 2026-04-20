package utils

// 文件说明：这个文件提供请求上下文中的 logger 读取工具。
// 实现方式：优先从 Gin context 里读取注入好的 logger，缺失时退回全局 logger。
// 这样做的好处是 handler 和 service 可以尽量复用同一份带请求字段的日志上下文。

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CtxLogger 从 Gin 上下文里提取 logger。
func CtxLogger(c *gin.Context) *zap.Logger {
	if v, ok := c.Get("logger"); ok {
		if lg, ok := v.(*zap.Logger); ok && lg != nil {
			return lg
		}
	}
	return zap.L()
}
