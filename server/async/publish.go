package async

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func PublishWithTimeout(bus IEventBus, lg *zap.Logger, topic string, payload any, timeout time.Duration, fields ...zap.Field) bool {
	if bus == nil {
		return false
	}

	pubCtx, cancel := context.WithTimeout(context.Background(), timeout)
	ok := bus.Publish(pubCtx, topic, payload)
	cancel()

	if !ok && lg != nil {
		lg.Warn("kafka.publish_failed",
			append([]zap.Field{zap.String("topic", topic)}, fields...)...,
		)
	}
	return ok
}
