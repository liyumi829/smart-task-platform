// internal/worker/outbox_worker.go
// Package worker
// 实现了 Outbox Worker 的核心逻辑：定时扫描数据库中的 Outbox 消息，抢占消息进行处理
package worker

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/mq/rabbitmq"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrInvalidMessage = errors.New("invalid outbox message")
)

// outBoxMessageRepository 定义 Outbox Worker 依赖的仓储接口
type outboxMessageRepository interface {
	// ClaimPending 原子抢占一批待发送 Outbox 消息
	ClaimPending(ctx context.Context, tx *gorm.DB, param *repository.ClaimPendingParam) ([]*model.OutboxMessage, error)

	// MarkAsPublished 标记 Outbox 消息发送成功
	MarkAsPublished(ctx context.Context, param *repository.MarkOutboxSentParam) error

	// MarkAsRetry 标记 Outbox 消息重新进入 pending 状态
	MarkAsRetry(ctx context.Context, param *repository.MarkOutboxRetryParam) error

	// MarkAsFailed 标记 Outbox 消息最终失败
	MarkAsFailed(ctx context.Context, param *repository.MarkOutboxFailedParam) error

	// ResetTimeoutProcessingMessages 重置超时 processing 消息
	ResetTimeoutProcessingMessages(ctx context.Context, param *repository.ResetTimeoutProcessingMessagesParam) error
}

// PublishMessage 定义发布消息所需的数据
type PublishMessage = rabbitmq.PublishMessage

// messagePublisher 定义了 Outbox Worker 依赖的消息发布接口
type messagePublisher interface {
	// Publish 发布消息到消息队列
	Publish(ctx context.Context, message *PublishMessage) error
}

// OutboxWorkerConfig 定义了 Outbox Worker 的配置项
type OutboxWorkerConfig struct {
	WorkerID string // 表示 Worker 实例
	bootstrap.OutboxWorkerConfig
}

// OutboxWorker 定义了 Outbox Worker 的核心结构
type OutboxWorker struct {
	config    OutboxWorkerConfig      // 配置项
	txMgr     *repository.TxManager   // 事务管理器
	repo      outboxMessageRepository // Outbox 消息仓储接口
	publisher messagePublisher        // 消息发布接口
	logger    *zap.Logger             // 日志记录
}

// NewOutboxWorker 创建一个新的 Outbox Worker 实例
func NewOutboxWorker(config OutboxWorkerConfig, txMgr *repository.TxManager, repo outboxMessageRepository, publisher messagePublisher, logger *zap.Logger) *OutboxWorker {
	if config.WorkerID == "" {
		config.WorkerID = fmt.Sprintf("worker-%s", utils.Uuid())
	}

	if logger == nil {
		logger = zap.NewNop() // 使用空日志器，避免 nil 引用
	}

	return &OutboxWorker{
		config:    config,
		txMgr:     txMgr,
		repo:      repo,
		publisher: publisher,
		logger: logger.With(
			zap.String("component", "outbox_worker"),
			zap.String("worker_id", config.WorkerID),
		),
	}
}

// Run 启动 Outbox Worker 定时轮询主流程
func (w *OutboxWorker) Run(ctx context.Context) {
	// 启动
	w.logger.Info("Outbox worker started successfully",
		zap.String("worker_id", w.config.WorkerID),
		zap.Duration("poll_interval", w.config.PollInterval),
		zap.Int("batch_size", w.config.BatchSize),
	)

	if *w.config.ResetTimeoutOnStartup {
		if err := w.resetTimeoutMessages(ctx); err != nil {
			w.logger.Error("Failed to reset timeout messages on startup", zap.Error(err))
		}
	}

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Outbox worker stopped gracefully", zap.Error(ctx.Err()))
			return

		case <-ticker.C:
			// 重置超时消息
			if err := w.resetTimeoutMessages(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("Reset timeout messages failed", zap.Error(err))
			}

			// 执行一轮消息处理
			if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Error("Process outbox messages failed", zap.Error(err))
			}
		}
	}
}

// processOnce 抢占一批待投递消息并逐条处理
func (w *OutboxWorker) processOnce(ctx context.Context) error {
	now := time.Now() // 获取到当前处理信息的时间

	var messages []*model.OutboxMessage
	err := w.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		var err error
		messages, err = w.repo.ClaimPending(ctx, tx, &repository.ClaimPendingParam{
			Limit:    w.config.BatchSize,
			Now:      now,
			WorkerID: w.config.WorkerID,
			LockedAt: now,
		})
		return err
	})
	if err != nil {
		w.logger.Error("Failed to claim pending messages", zap.Error(err))
		return err
	}

	// 无可用消息，直接返回
	if len(messages) == 0 {
		w.logger.Debug("No pending outbox messages found")
		return nil
	}

	w.logger.Info("Outbox messages claimed successfully",
		zap.Int("count", len(messages)),
	)

	// 抢占成功，逐条进行处理
	for _, message := range messages {
		// handlerMessage 内部进行差错处理，这里仅仅记录日志
		if err := w.handleMessage(ctx, message); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("Handle outbox message failed",
				zap.Error(err),
				zap.Uint64("message_id", message.ID),
				zap.String("event_id", message.EventID),
				zap.String("event_type", message.EventType),
			)
		}
	}

	return nil
}

// handleMessage 处理单条已抢占的 Outbox 消息
func (w *OutboxWorker) handleMessage(ctx context.Context, message *model.OutboxMessage) error {
	if message == nil || message.ID == 0 {
		return ErrInvalidMessage
	}

	log := w.logger.With(
		zap.Uint64("message_id", message.ID),
		zap.String("event_id", message.EventID),
		zap.String("event_type", message.EventType),
		zap.String("exchange_name", message.ExchangeName),
		zap.String("routing_key", message.RoutingKey),
	)

	// 构造发布消息
	publishMsg := buildPublishMessage(message)

	// 发布消息到消息队列
	if publishErr := w.publisher.Publish(ctx, publishMsg); publishErr != nil {
		// 判断是否需要标记为最终失败
		if shouldMarkAsFailed(message) {
			if err := w.repo.MarkAsFailed(ctx, &repository.MarkOutboxFailedParam{
				MessageID:    message.ID,
				ErrorMessage: truncateErrorMessage(publishErr.Error()),
				UpdatedAt:    time.Now(),
			}); err != nil {
				log.Error("Mark outbox message as failed failed", zap.Error(err))
				return err
			}

			log.Warn("Outbox message marked as failed",
				zap.Int("retry_count", message.RetryCount+1),
				zap.Int("max_retry_count", message.MaxRetryCount),
			)
		} else {
			if err := w.markMessageAsRetry(ctx, message, publishErr); err != nil {
				log.Error("Mark outbox message as retry failed", zap.Error(err))
				return err
			}

			log.Warn("Outbox message marked as retry",
				zap.Int("retry_count", message.RetryCount+1),
				zap.Int("max_retry_count", message.MaxRetryCount),
			)
		}
		return nil
	}

	if err := w.markMessageAsPublished(ctx, message); err != nil {
		log.Error("Mark outbox message as published failed", zap.Error(err))
		return err
	}

	log.Info("Outbox message published successfully")

	return nil
}

// resetTimeoutMessages 重置处理超时的 processing 消息，恢复为 pending
func (w *OutboxWorker) resetTimeoutMessages(ctx context.Context) error {
	now := time.Now()                              // 现在的时间
	before := now.Add(-w.config.ProcessingTimeout) // 超过这个时间还在 processing 的消息都认为时超时了

	if err := w.repo.ResetTimeoutProcessingMessages(ctx, &repository.ResetTimeoutProcessingMessagesParam{
		Before:    before,
		UpdatedAt: now,
	}); err != nil {
		return err
	}

	w.logger.Debug("Timeout outbox messages reset successfully",
		zap.Time("before", before),
	)

	return nil
}

//==========
// 更新状态
//==========

// markMessageAsPublished 标记消息发送成功
func (w *OutboxWorker) markMessageAsPublished(ctx context.Context, message *model.OutboxMessage) error {
	return w.repo.MarkAsPublished(ctx, &repository.MarkOutboxSentParam{
		MessageID: message.ID,
		SentAt:    time.Now(),
	})
}

// markMessageAsRetry 标记消息等待下次重试
func (w *OutboxWorker) markMessageAsRetry(ctx context.Context, message *model.OutboxMessage, cause error) error {
	now := time.Now()
	nextRetryAt := now.Add(w.config.RetryBackoff)

	return w.repo.MarkAsRetry(ctx, &repository.MarkOutboxRetryParam{
		MessageID:    message.ID,
		NextRetryAt:  nextRetryAt,
		ErrorMessage: truncateErrorMessage(cause.Error()),
		UpdatedAt:    now,
	})
}

// markMessageAsFailed 标记消息最终失败
func (w *OutboxWorker) markMessageAsFailed(ctx context.Context, message *model.OutboxMessage, cause error) error {
	return w.repo.MarkAsFailed(ctx, &repository.MarkOutboxFailedParam{
		MessageID:    message.ID,
		ErrorMessage: truncateErrorMessage(cause.Error()),
		UpdatedAt:    time.Now(),
	})
}

//=============
// helper
//=============

// shouldMarkAsFailed 判断当前消息是否应标记为最终失败
func shouldMarkAsFailed(message *model.OutboxMessage) bool {
	if message == nil {
		return true
	}

	return message.RetryCount+1 >= message.MaxRetryCount
}

// truncateErrorMessage 截断错误信息，避免超过数据库字段长度
func truncateErrorMessage(message string) string {
	const maxLen = 1000

	if len(message) <= maxLen {
		return message
	}

	return message[:maxLen]
}

// buildPublishMessage 从 OutboxMessage 构建 PublishMessage
func buildPublishMessage(outboxMsg *model.OutboxMessage) *PublishMessage {
	return &PublishMessage{
		MessageID:   utils.Uuid() + fmt.Sprintf("-%d", outboxMsg.ID), // 生成全局唯一的消息ID，附加上 Outbox 消息ID 以便追踪
		EventID:     outboxMsg.EventID,
		EventType:   outboxMsg.EventType,
		Exchange:    outboxMsg.ExchangeName,
		RoutingKey:  outboxMsg.RoutingKey,
		ContentType: "application/json", // 消息都是使用 json 格式
		Payload:     outboxMsg.Payload,
		Headers: map[string]any{
			rabbitmq.HeaderEventID:    outboxMsg.EventID,
			rabbitmq.HeaderEventType:  outboxMsg.EventType,
			rabbitmq.HeaderRetryCount: 0, // 这个重试次数是 rabbitmq 进行消费的重试，而不是投递的重试次数
		},
	}
}
