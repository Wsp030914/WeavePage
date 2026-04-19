package async

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

func NewEventBus(producer *KafkaProducer) *EventBus {
	return &EventBus{
		producer: producer,
		lg:       zap.L(),
	}
}

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

// Close closes the underlying Kafka producer.
func (b *EventBus) Close() error {
	return b.producer.Close()
}
