package async

// 文件说明：这个文件封装异步事件总线抽象及其 Kafka 实现。
// 实现方式：通过 IEventBus 屏蔽具体消息系统，再由 EventBus 把 trace_id 和日志语义统一补齐。
// 这样做的好处是主业务链路只依赖发布能力本身，不需要直接耦合 Kafka producer 细节。

import (
	"ToDoList/server/utils"
	"context"

	"go.uber.org/zap"
)

// IEventBus is the plugin interface for all EventBus backends.
// Switch between Kafka and Redis Stream by providing a different implementation.
type IEventBus interface {
	// Publish sends payload to the given topic. Returns false on failure.
	Publish(ctx context.Context, topic string, payload any) bool
	// Close releases underlying resources gracefully.
	Close() error
}

// EventBus is the Kafka-backed implementation of IEventBus.
type EventBus struct {
	producer *KafkaProducer
	lg       *zap.Logger
}

// NewEventBus 创建 Kafka 事件总线适配器。
func NewEventBus(producer *KafkaProducer) *EventBus {
	return &EventBus{
		producer: producer,
		lg:       zap.L(),
	}
}

// Publish 向指定 topic 发布异步事件。
// 这里主动从上下文里补 trace_id，是为了让异步副作用链路在日志里仍然能和原始请求串起来。
func (b *EventBus) Publish(ctx context.Context, topic string, payload any) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return false
	default:
	}

	traceID := utils.RequestIDFromCtx(ctx)
	if traceID == "" {
		if v := ctx.Value("trace_id"); v != nil {
			if s, ok := v.(string); ok {
				traceID = s
			}
		}
	}

	err := b.producer.Publish(ctx, topic, payload, traceID)
	if err != nil {
		b.lg.Warn("kafka.publish_failed",
			zap.String("topic", topic),
			zap.String("trace_id", traceID),
			zap.Error(err))
		return false
	}
	return true
}

// Close 关闭底层 Kafka producer。
func (b *EventBus) Close() error {
	return b.producer.Close()
}
