package utils

import (
	"context"

	"go.uber.org/zap"
)

type ctxKeyLogger struct{}

func WithLogger(ctx context.Context, lg *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger{}, lg)
}

func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, "request_id", reqID)
}

func LoggerFromContext(ctx context.Context) *zap.Logger {
	if v := ctx.Value(ctxKeyLogger{}); v != nil {
		if lg, ok := v.(*zap.Logger); ok && lg != nil {
			return lg
		}
	}
	return zap.L()
}

func RequestIDFromCtx(ctx context.Context) string {
	if v := ctx.Value("request_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
