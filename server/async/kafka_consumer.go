package async

// 文件说明：这个文件实现 Kafka 消费端运行时。
// 实现方式：通过 worker 池拉取消息，按类型分发到处理器，并内置重试、panic 保护和 DLQ 兜底。
// 这样做的好处是可靠异步副作用可以独立于主请求链路运行，同时具备基本的故障恢复能力。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const (
	ConsumerMaxRetry    = 3
	ConsumerBaseBackoff = 300 * time.Millisecond
	ConsumerMaxBackoff  = 1500 * time.Millisecond
)

type KafkaJob struct {
	Type      string
	Payload   []byte
	TraceID   string
	Retry     int
	Partition int
	Offset    int64
}

type KafkaHandler func(ctx context.Context, job KafkaJob, lg *zap.Logger) error

type KafkaConsumer struct {
	reader      *kafka.Reader
	handlers    map[string]KafkaHandler
	producer    *KafkaProducer
	dlqTopic    string
	lg          *zap.Logger
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	workers     int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

var ErrDLQNotConfigured = errors.New("dlq not configured")

type KafkaConsumerOption func(*KafkaConsumer)

// WithWorkerCount 配置消费端 worker 数量。
func WithWorkerCount(workers int) KafkaConsumerOption {
	return func(c *KafkaConsumer) {
		c.workers = workers
	}
}

// WithBackoff 配置消费失败后的退避区间。
func WithBackoff(base, max time.Duration) KafkaConsumerOption {
	return func(c *KafkaConsumer) {
		c.baseBackoff = base
		c.maxBackoff = max
	}
}

// WithDeadLetterQueue 为消费者注入 DLQ 发布能力。
func WithDeadLetterQueue(producer *KafkaProducer, dlqTopic string) KafkaConsumerOption {
	return func(c *KafkaConsumer) {
		c.producer = producer
		c.dlqTopic = dlqTopic
	}
}

// NewKafkaConsumer 创建 Kafka 消费者。
// 默认从 last offset 开始消费，是为了避免新实例启动时把历史消息整批重复吃一遍。
func NewKafkaConsumer(brokers []string, topic, groupID string, opts ...KafkaConsumerOption) *KafkaConsumer {
	lg := zap.L()
	ctx, cancel := context.WithCancel(context.Background())

	c := &KafkaConsumer{
		handlers:    make(map[string]KafkaHandler),
		lg:          lg,
		ctx:         ctx,
		cancel:      cancel,
		workers:     4,
		baseBackoff: ConsumerBaseBackoff,
		maxBackoff:  ConsumerMaxBackoff,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		MaxWait:        time.Second,
		CommitInterval: time.Second,
		StartOffset:    kafka.LastOffset,
	})

	return c
}

// Register 注册某种事件类型对应的处理函数。
// 这里对重复注册直接 panic，是为了在启动阶段尽早暴露装配错误。
func (c *KafkaConsumer) Register(handlerType string, h KafkaHandler) {
	if _, exists := c.handlers[handlerType]; exists {
		panic("duplicate handler: " + handlerType)
	}
	c.handlers[handlerType] = h
}

// Start 启动所有消费 worker。
func (c *KafkaConsumer) Start() {
	for i := 0; i < c.workers; i++ {
		c.wg.Add(1)
		go c.worker(i)
	}
	c.lg.Info("kafka.consumer.started", zap.Int("workers", c.workers))
}

// Stop 停止消费者并等待 worker 退出。
func (c *KafkaConsumer) Stop() {
	c.cancel()
	c.wg.Wait()
	c.reader.Close()
	c.lg.Info("kafka.consumer.stopped")
}

// worker 是单个消费协程的主循环。
// 每个 worker 都带 panic 保护，是为了避免某个 handler 崩掉后整个消费者静默退出。
func (c *KafkaConsumer) worker(id int) {
	defer c.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			c.lg.Error("kafka.worker.panic",
				zap.Int("id", id),
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			c.lg.Info("kafka.worker.exit", zap.Int("id", id))
			return
		default:
			msg, err := c.reader.FetchMessage(c.ctx)
			if err != nil {
				if c.ctx.Err() != nil {
					return
				}
				c.lg.Error("kafka.fetch.failed", zap.Int("id", id), zap.Error(err))
				continue
			}

			commit, err := c.processMessage(msg)
			if err != nil {
				c.lg.Error("kafka.process.failed",
					zap.Int("id", id),
					zap.String("topic", msg.Topic),
					zap.Error(err))
			}
			if !commit {
				c.lg.Warn("kafka.commit.skipped",
					zap.Int("id", id),
					zap.String("topic", msg.Topic),
					zap.Int("partition", msg.Partition),
					zap.Int64("offset", msg.Offset))
				continue
			}

			if err := c.reader.CommitMessages(c.ctx, msg); err != nil {
				c.lg.Error("kafka.commit.failed", zap.Error(err))
			}
		}
	}
}

// processMessage 负责反序列化 Kafka 消息、分发处理器并给出是否提交 offset 的结论。
func (c *KafkaConsumer) processMessage(msg kafka.Message) (bool, error) {
	var km KafkaMessage
	if err := json.Unmarshal(msg.Value, &km); err != nil {
		payload, marshalErr := json.Marshal(map[string]string{
			"raw_message": string(msg.Value),
		})
		if marshalErr != nil {
			payload = []byte(`{"raw_message":"<marshal-failed>"}`)
		}
		fallback := KafkaMessage{
			Type:      "decode_error",
			Payload:   payload,
			TraceID:   traceIDFromHeaders(msg.Headers),
			CreatedAt: time.Now(),
			Retry:     ConsumerMaxRetry,
		}
		c.lg.Error("kafka.message.decode_failed",
			zap.String("topic", msg.Topic),
			zap.Error(err))
		return c.commitDecisionAfterFailure(msg, fallback, fmt.Errorf("decode kafka message: %w", err))
	}

	job := KafkaJob{
		Type:      km.Type,
		Payload:   km.Payload,
		TraceID:   km.TraceID,
		Retry:     km.Retry,
		Partition: msg.Partition,
		Offset:    msg.Offset,
	}

	handler, exists := c.handlers[job.Type]
	if !exists {
		noHandlerErr := fmt.Errorf("no handler for type %q", job.Type)
		c.lg.Warn("kafka.no_handler",
			zap.String("type", job.Type),
			zap.String("trace_id", job.TraceID),
			zap.Error(noHandlerErr))
		return c.commitDecisionAfterFailure(msg, km, noHandlerErr)
	}

	lg := c.lg.With(
		zap.String("type", job.Type),
		zap.String("trace_id", job.TraceID),
		zap.Int("partition", job.Partition),
		zap.Int64("offset", job.Offset),
		zap.Int("retry", job.Retry),
	)

	err := c.executeWithRetry(c.ctx, job, handler, lg)

	if err != nil {
		return c.commitDecisionAfterFailure(msg, km, err)
	}

	lg.Info("kafka.job.success")
	return true, nil
}

// commitDecisionAfterFailure 决定失败消息是否转入 DLQ 并提交 offset。
// 只有成功写入 DLQ 后才允许提交 offset，是为了避免坏消息既处理失败又彻底丢失。
func (c *KafkaConsumer) commitDecisionAfterFailure(msg kafka.Message, km KafkaMessage, cause error) (bool, error) {
	if c.producer == nil || c.dlqTopic == "" {
		return false, fmt.Errorf("%w: %v", ErrDLQNotConfigured, cause)
	}

	c.lg.Error("kafka.send_to_dlq",
		zap.String("type", km.Type),
		zap.String("trace_id", km.TraceID),
		zap.String("topic", msg.Topic),
		zap.Int("partition", msg.Partition),
		zap.Int64("offset", msg.Offset),
		zap.Error(cause))

	if err := c.producer.PublishToDLQ(c.ctx, c.dlqTopic, msg.Topic, km, cause); err != nil {
		return false, fmt.Errorf("publish message to dlq: %w", err)
	}
	return true, nil
}

// traceIDFromHeaders 从 Kafka header 里提取 trace_id。
func traceIDFromHeaders(headers []kafka.Header) string {
	for _, h := range headers {
		if h.Key == "trace_id" {
			return string(h.Value)
		}
	}
	return ""
}

// executeWithRetry 带指数退避地执行消息处理器。
// 这里加入 jitter，是为了避免大量失败消息在同一时刻齐刷刷重试。
func (c *KafkaConsumer) executeWithRetry(ctx context.Context, job KafkaJob, handler KafkaHandler, lg *zap.Logger) error {
	var err error
	for retry := 0; retry <= ConsumerMaxRetry; retry++ {
		if retry > 0 {
			backoff := c.baseBackoff * (1 << uint(retry-1))
			if backoff > c.maxBackoff {
				backoff = c.maxBackoff
			}
			jitter := 0.8 + rand.Float64()*0.4
			sleepDuration := time.Duration(float64(backoff) * jitter)

			lg.Info("kafka.retry.wait",
				zap.Int("retry", retry),
				zap.Duration("backoff", backoff),
				zap.Duration("sleep", sleepDuration))

			timer := time.NewTimer(sleepDuration)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		err = c.safeExecute(ctx, job, handler, lg)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		lg.Warn("kafka.job.failed", zap.Int("retry", retry), zap.Error(err))
	}

	return fmt.Errorf("max retries exceeded: %w", err)
}

// safeExecute 在超时和 panic 保护下执行单个 handler。
// 每次 handler 调用都套独立 30 秒超时，是为了避免某个异步任务无限占住消费 worker。
func (c *KafkaConsumer) safeExecute(ctx context.Context, job KafkaJob, handler KafkaHandler, lg *zap.Logger) (err error) {
	defer func() {
		if r := recover(); r != nil {
			lg.Error("kafka.handler.panic",
				zap.Any("panic", r),
				zap.ByteString("stack", debug.Stack()),
				zap.String("type", job.Type))
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return handler(ctx, job, lg)
}
