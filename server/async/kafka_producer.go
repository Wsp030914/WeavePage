package async

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type KafkaProducer struct {
	writer *kafka.Writer
	lg     *zap.Logger
	mu     sync.RWMutex
	closed bool
}

type KafkaMessage struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	TraceID   string          `json:"trace_id"`
	CreatedAt time.Time       `json:"created_at"`
	Retry     int             `json:"retry"`
}

func NewKafkaProducer(brokers []string, topic string) *KafkaProducer {
	lg := zap.L()

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		BatchSize:    10,
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		Compression:  kafka.Snappy,
	}

	return &KafkaProducer{
		writer: writer,
		lg:     lg,
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, jobType string, payload any, traceID string) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		p.lg.Error("kafka.publish.marshal_failed",
			zap.String("type", jobType),
			zap.String("trace_id", traceID),
			zap.Error(err))
		return err
	}

	msg := KafkaMessage{
		Type:      jobType,
		Payload:   payloadBytes,
		TraceID:   traceID,
		CreatedAt: time.Now(),
		Retry:     0,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		p.lg.Error("kafka.publish.msg_marshal_failed",
			zap.String("type", jobType),
			zap.Error(err))
		return err
	}

	kafkaMsg := kafka.Message{
		Key:   []byte(traceID),
		Value: msgBytes,
		Headers: []kafka.Header{
			{Key: "type", Value: []byte(jobType)},
			{Key: "trace_id", Value: []byte(traceID)},
		},
	}

	err = p.writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		p.lg.Error("kafka.publish.write_failed",
			zap.String("type", jobType),
			zap.String("trace_id", traceID),
			zap.Error(err))
		return err
	}

	p.lg.Info("kafka.publish.success",
		zap.String("type", jobType),
		zap.String("trace_id", traceID))

	return nil
}

func (p *KafkaProducer) PublishToDLQ(ctx context.Context, dlqTopic string, originalTopic string, msg KafkaMessage, cause error) error {
	msg.Retry++

	dlqMsg := KafkaMessage{
		Type:      "dlq:" + msg.Type,
		Payload:   msg.Payload,
		TraceID:   msg.TraceID,
		CreatedAt: time.Now(),
		Retry:     msg.Retry,
	}
	causeText := "unknown"
	if cause != nil {
		causeText = cause.Error()
	}

	msgBytes, err := json.Marshal(dlqMsg)
	if err != nil {
		return fmt.Errorf("marshal dlq message: %w", err)
	}

	kafkaMsg := kafka.Message{
		Key:   []byte(msg.TraceID),
		Value: msgBytes,
		Headers: []kafka.Header{
			{Key: "original_topic", Value: []byte(originalTopic)},
			{Key: "error", Value: []byte(causeText)},
			{Key: "retry_count", Value: []byte(fmt.Sprintf("%d", msg.Retry))},
		},
	}

	dlqWriter := &kafka.Writer{
		Addr:         p.writer.Addr,
		Topic:        dlqTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	defer dlqWriter.Close()

	if err := dlqWriter.WriteMessages(ctx, kafkaMsg); err != nil {
		p.lg.Error("kafka.dlq.write_failed",
			zap.String("type", msg.Type),
			zap.String("trace_id", msg.TraceID),
			zap.Error(err))
		return err
	}

	p.lg.Warn("kafka.dlq.published",
		zap.String("type", msg.Type),
		zap.String("trace_id", msg.TraceID),
		zap.Int("retry", msg.Retry),
		zap.Error(cause))

	return nil
}

func (p *KafkaProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.writer.Close()
}
