// internal/service/forwarding_service/worker/forwarding_worker.go
// package worker
// 功能：实现转发 Worker：处理单条 RabbitMQ 消息，并根据处理结果执行 ack、retry 或 dead-letter

package worker

import (
	"context"
	"fmt"
	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/mq/rabbitmq"
	"smart-task-platform/internal/pkg/utils"
	"sync"

	"go.uber.org/zap"
)

//======
// 类型定义
//======

// ConsumeAction 消费结果动作
type ConsumeAction int

const (
	ConsumeActionAck    ConsumeAction = iota // ConsumeActionAck 表示消费成功
	ConsumeActionRetry                       // ConsumeActionRetry 表示可恢复失败，需要重试
	ConsumeActionReject                      // ConsumeActionReject 表示不可恢复失败，直接进入死信
)

// HandlerResult 消费处理结果
type HandlerResult struct {
	Action ConsumeAction
	Reason string
}

// AckResult 返回成功结果
func AckResult() HandlerResult {
	return HandlerResult{Action: ConsumeActionAck}
}

// RetryResult 返回重试结果
func RetryResult(reason string) HandlerResult {
	return HandlerResult{
		Action: ConsumeActionRetry,
		Reason: reason,
	}
}

// RejectResult 返回拒绝结果
func RejectResult(reason string) HandlerResult {
	return HandlerResult{
		Action: ConsumeActionReject,
		Reason: reason,
	}
}

type ConsumedMessage = rabbitmq.ConsumedMessage

// HandleMessageFunc 消费处理函数类型
// 消费 ConsumedMessage 的调用函数
type HandleMessage func(message *ConsumedMessage) HandlerResult

//======
// worker 配置
//======

// HandleWorkerConfig 配置
type HandleWorkerConfig struct {
	WorkerID string // 工作ID
	bootstrap.HandleWorkerConfig
}

//======
// worker 定义
//======

// HandleWorker 消息的消费者，实现消息转发
type HandleWorker struct {
	config         HandleWorkerConfig // 配置
	retryPublisher messagePublisher   // 重试队列发布者

	ackMu sync.Mutex // ack/nack 操作互斥锁，确保同一时间只有一个 goroutine 在操作消息 ack/nack，避免 RabbitMQ channel 错误

	logger *zap.Logger // 日志器
}

// NewHandleWorker 实例化一个消息转发 worker
func NewHandleWorker(
	config HandleWorkerConfig,
	retryPublisher messagePublisher,
	logger *zap.Logger,
) *HandleWorker {
	if config.WorkerID == "" {
		config.WorkerID = fmt.Sprintf("worker-%s", utils.Uuid())
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &HandleWorker{
		config:         config,
		retryPublisher: retryPublisher,
		logger: logger.With(
			zap.String("component", "forwarding_worker"),
			zap.String("worker_id", config.WorkerID),
		),
	}
}

// Run 启动 Handle Worker 主流程
func (w *HandleWorker) Run(ctx context.Context, in <-chan *ConsumedMessage, handle HandleMessage) {
	w.logger.Info("Handle worker started successfully",
		zap.String("worker_id", w.config.WorkerID),
		zap.Int("max_retries", w.config.MaxRetries),
	)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Handle worker stopped gracefully", zap.Error(ctx.Err()))
			return

		case message, ok := <-in:
			if !ok {
				// 这是正常的 我们会优先关闭 subscriber
				w.logger.Info("Handle worker stopped because delivery channel closed")
				return
			}

			w.HandleMessage(ctx, message, handle)
		}
	}
}

// HandleMessage 处理单条消息
func (w *HandleWorker) HandleMessage(ctx context.Context, message *ConsumedMessage, handle HandleMessage) {
	if ctx == nil {
		ctx = context.Background()
	}
	if message == nil {
		w.logger.Error("RabbitMQ consumed message is nil")
		return
	}
	if handle == nil {
		w.logger.Error("RabbitMQ consume handler is nil",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
		)

		w.deadLetterMessage(message, "consume handler is nil")
		return
	}

	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("RabbitMQ consumer handle panic recovered",
				zap.String("message_id", message.MessageID),
				zap.String("event_id", message.EventID),
				zap.Any("panic", r),
			)

			// panic 视为可恢复错误，进入 retry 流程
			w.retryOrDeadLetter(ctx, message, "handle panic recovered")
		}
	}()

	result := handle(message) // 调用回调处理消息

	// 根据处理结果，选择不同处理流程
	switch result.Action {
	case ConsumeActionAck:
		w.ackMessage(message) // ack

	case ConsumeActionRetry:
		w.retryOrDeadLetter(ctx, message, result.Reason) // 重试或者进入死信队列

	case ConsumeActionReject:
		w.deadLetterMessage(message, result.Reason) // 进入私信队列

	default:
		w.retryOrDeadLetter(ctx, message, "unknown consume result")
	}
}

// ackMessage ack 消息
func (w *HandleWorker) ackMessage(message *ConsumedMessage) {
	w.ackMu.Lock()
	err := message.Ack()
	w.ackMu.Unlock()

	if err != nil {
		w.logger.Error("RabbitMQ message ack failed",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.Error(err),
		)
		return
	}

	w.logger.Info("RabbitMQ message consumed successfully",
		zap.String("message_id", message.MessageID),
		zap.String("event_id", message.EventID),
		zap.String("event_type", message.EventType),
		zap.Int("retry_count", message.RetryCount),
	)
}

// retryOrDeadLetter 重试或进入死信
func (w *HandleWorker) retryOrDeadLetter(ctx context.Context, message *ConsumedMessage, reason string) {
	if message == nil {
		w.logger.Error("RabbitMQ retry skipped because message is nil")
		return
	}
	reason = normalizeReason(reason, "consume failed")
	if message.RetryCount >= w.config.MaxRetries { // 大于最大重试次数直接进入死信队列
		w.deadLetterMessage(message, reason)
		return
	}
	if w.retryPublisher == nil {
		w.logger.Error("RabbitMQ retry publisher is nil",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.String("reason", reason),
		)

		// retry publisher 不可用时，不能 ack 原消息，否则消息会丢失
		w.deadLetterMessage(message, "retry publisher is nil: "+reason)
		return
	}

	// 下面进入重试队列
	// 构造重试消息
	nextRetryCount := message.RetryCount + 1
	newHeaders := rabbitmq.CloneHeaders(message.Headers)
	newHeaders[rabbitmq.HeaderRetryCount] = nextRetryCount

	retryMessage := &PublishMessage{
		MessageID:   message.MessageID,
		EventID:     message.EventID,
		EventType:   message.EventType,
		Exchange:    w.config.RetryExchangeName,
		RoutingKey:  w.config.RetryRoutingKey,
		ContentType: message.ContentType,
		Payload:     message.Payload,
		Headers:     newHeaders,
	}

	// 调用重试队列的发布
	if err := w.retryPublisher.Publish(ctx, retryMessage); err != nil {
		w.logger.Error("RabbitMQ retry message publish failed",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.Int("retry_count", nextRetryCount),
			zap.String("reason", reason),
			zap.Error(err),
		)

		// retry 发布失败说明重试链路不可用
		// 这里不能 Ack 原消息，否则会造成消息丢失
		// 也不建议 Nack(true)，否则消息会立即回到主队列并可能形成空转风暴

		// retry 发布失败直接进入死信队列
		w.deadLetterMessage(message, "retry publish failed: "+reason)
		return
	}

	// retry 消息发布成功后 ack 原消息
	// 注意：publish retry 和 ack original 不是原子操作，需要业务幂等兜底
	if err := w.ack(message); err != nil {
		w.logger.Error("RabbitMQ original message ack failed after retry published",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.Int("next_retry_count", nextRetryCount),
			zap.Error(err),
		)
		return
	}

	w.logger.Warn("RabbitMQ message retry scheduled",
		zap.String("message_id", message.MessageID),
		zap.String("event_id", message.EventID),
		zap.String("event_type", message.EventType),
		zap.Int("retry_count", nextRetryCount),
		zap.String("reason", reason),
	)
}

// deadLetterMessage 将消息送入死信
func (w *HandleWorker) deadLetterMessage(message *ConsumedMessage, reason string) {
	if message == nil {
		w.logger.Error("RabbitMQ dead letter skipped because message is nil")
		return
	}

	reason = normalizeReason(reason, "message rejected")

	if err := w.nack(message, false); err != nil {
		w.logger.Error("RabbitMQ message dead letter failed",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.String("reason", reason),
			zap.Error(err),
		)
		return
	}

	w.logger.Warn("RabbitMQ message moved to dead letter",
		zap.String("message_id", message.MessageID),
		zap.String("event_id", message.EventID),
		zap.String("event_type", message.EventType),
		zap.Int("retry_count", message.RetryCount),
		zap.String("reason", reason),
	)
}

//=========
// helper
//=========

// ack 串行化执行 ack，避免 RabbitMQ channel 并发操作问题
func (w *HandleWorker) ack(message *ConsumedMessage) error {
	w.ackMu.Lock()
	defer w.ackMu.Unlock()

	return message.Ack()
}

// nack 串行化执行 nack，避免 RabbitMQ channel 并发操作问题
func (w *HandleWorker) nack(message *ConsumedMessage, requeue bool) error {
	w.ackMu.Lock()
	defer w.ackMu.Unlock()

	return message.Nack(requeue)
}

// normalizeReason 规范化原因
func normalizeReason(reason string, fallback string) string {
	if reason != "" {
		return reason
	}

	return fallback
}
