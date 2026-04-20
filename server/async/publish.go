package async

// 文件说明：这个文件提供异步事件发布的便捷辅助方法。
// 实现方式：在总线发布外再包一层超时控制和统一日志输出。
// 这样做的好处是业务层可以快速发出短时副作用，而不必在每个调用点重复写 context 超时和失败日志。

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// PublishWithTimeout 在固定超时内发布一条异步事件。
// 这里统一从后台 context 派生超时上下文，是为了避免主请求已经结束后长时间阻塞在异步发布上。
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
