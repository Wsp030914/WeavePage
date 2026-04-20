package utils

// 文件说明：这个文件封装请求上下文里的 logger 和 request_id 工具。
// 实现方式：通过 context key 存取 logger 与 request_id，并提供统一读取函数。
// 这样做的好处是异步链路、缓存保护和服务层都能共享同一套请求级上下文数据。

import (
	"context"

	"go.uber.org/zap"
)

type ctxKeyLogger struct{}

// WithLogger 把 logger 写入 context。
func WithLogger(ctx context.Context, lg *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger{}, lg)
}

// WithRequestID 把 request_id 写入 context。
func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, "request_id", reqID)
}

// LoggerFromContext 从 context 中提取 logger。
func LoggerFromContext(ctx context.Context) *zap.Logger {
	if v := ctx.Value(ctxKeyLogger{}); v != nil {
		if lg, ok := v.(*zap.Logger); ok && lg != nil {
			return lg
		}
	}
	return zap.L()
}

// RequestIDFromCtx 从 context 中提取 request_id。
func RequestIDFromCtx(ctx context.Context) string {
	if v := ctx.Value("request_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
