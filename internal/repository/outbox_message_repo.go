// internal/repository/outbox_message_repo.go
// Package repository
// 实现 outbox_messages 表的仓储操作

package repository

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidOutboxMessageParam = errors.New("invalid outbox message param")          // 不合法的Outbox参数
	ErrCreateOutboxMessageEmpty  = errors.New("create outbox message params is empty") // 创建Outbox消息参数为空
	ErrOutboxMessageNotFound     = errors.New("outbox message not found")              // Outbox消息不存在/未找到
	ErrOutboxNoRowsUpdated       = errors.New("outbox no rows updated")                // Outbox更新无数据受影响
)

// outboxMessageRepository Outbox 消息仓储
type outboxMessageRepository struct {
	db *gorm.DB
}

// NewOutboxMessageRepository 创建 Outbox 消息仓储
func NewOutboxMessageRepository(db *gorm.DB) *outboxMessageRepository {
	return &outboxMessageRepository{
		db: db,
	}
}

// CreateWithTx 在 Outbox 消息表中插入一条数据
//
// 用于业务事务：创建任务动态 + 创建通知 + 创建 outbox message
func (r *outboxMessageRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, message *model.OutboxMessage) error {
	if message == nil {
		return ErrCreateOutboxMessageEmpty
	}

	return getDB(ctx, r.db, tx).Create(message).Error
}

// OutboxPendingQuery 待发送 Outbox 消息查询参数
//
// 用于 Outbox Worker 扫描待投递消息
type OutboxPendingQuery struct {
	Limit int       // 查询数量
	Now   time.Time // 当前时间，用于判断 next_retry_at
}

// ListPending 查询待发送的 Outbox 消息
//
// 用于 Outbox Worker 扫描待投递消息
func (r *outboxMessageRepository) ListPending(ctx context.Context, query *OutboxPendingQuery) ([]*model.OutboxMessage, error) {
	if query == nil || query.Limit <= 0 || query.Now.IsZero() {
		return nil, ErrInvalidOutboxMessageParam
	}

	messages := make([]*model.OutboxMessage, 0, query.Limit)

	err := getDB(ctx, r.db, nil).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusPending).
		Where(model.OutboxMessageColumnRetryCount+" < "+model.OutboxMessageColumnMaxRetryCount).
		Where("("+model.OutboxMessageColumnNextRetryAt+" IS NULL OR "+model.OutboxMessageColumnNextRetryAt+" <= ?)", query.Now).
		Order(model.OutboxMessageColumnCreatedAt + " ASC").
		Order(model.OutboxMessageColumnID + " ASC").
		Limit(query.Limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}

	return messages, nil
}

// LockMessageParam 锁定 Outbox 消息参数
type LockMessageParam struct {
	MessageID uint64    // Outbox 消息ID
	WorkerID  string    // Worker 标识
	LockedAt  time.Time // 锁定时间
}

// LockMessage 锁定一条 Outbox 消息
//
// 用于防止多个 Outbox Worker 重复发布同一条消息
func (r *outboxMessageRepository) LockMessage(ctx context.Context, tx *gorm.DB, param *LockMessageParam) (bool, error) {
	if param == nil || param.MessageID == 0 || param.WorkerID == "" || param.LockedAt.IsZero() {
		return false, ErrInvalidOutboxMessageParam
	}

	result := getDB(ctx, r.db, tx).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" = ?", param.MessageID).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusPending).
		Updates(map[string]interface{}{
			model.OutboxMessageColumnStatus:    model.OutboxMessageStatusProcessing,
			model.OutboxMessageColumnLockedBy:  param.WorkerID,
			model.OutboxMessageColumnLockedAt:  param.LockedAt,
			model.OutboxMessageColumnUpdatedAt: param.LockedAt,
		})

	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected == 1, nil
}

// ClaimPendingParam 抢占待发送 Outbox 消息参数
//
// 用于多个 Outbox Worker 并发扫描时，原子抢占一批 pending 消息
type ClaimPendingParam struct {
	Limit    int       // 抢占数量
	Now      time.Time // 当前时间，用于判断 next_retry_at
	WorkerID string    // Worker 标识
	LockedAt time.Time // 锁定时间
}

// ClaimPending 抢占一批待发送 Outbox 消息
//
// 逻辑：
//  1. 在事务中查询符合条件的 pending 消息
//  2. 使用 FOR UPDATE SKIP LOCKED 跳过已被其它事务锁定的消息
//  3. 将查询到的消息批量更新为 processing
//  4. 返回当前 Worker 成功抢占的消息列表
func (r *outboxMessageRepository) ClaimPending(ctx context.Context, tx *gorm.DB, param *ClaimPendingParam) ([]*model.OutboxMessage, error) {
	if param == nil || param.Limit <= 0 || param.Now.IsZero() || param.WorkerID == "" || param.LockedAt.IsZero() {
		return nil, ErrInvalidOutboxMessageParam
	}

	messages := make([]*model.OutboxMessage, 0, param.Limit)

	// FOR UPDATE：对查询到的行加写锁
	// SKIP LOCKED：如果行已经被别人锁住，直接跳过，不查询、不等待
	// 锁 = 持有到 事务提交 / 事务回滚 才会释放！
	if err := getDB(ctx, r.db, tx).
		Model(&model.OutboxMessage{}).
		Clauses(clause.Locking{
			Strength: "UPDATE",
			Options:  "SKIP LOCKED",
		}).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusPending).
		Where(model.OutboxMessageColumnRetryCount+" < "+model.OutboxMessageColumnMaxRetryCount).
		Where("("+model.OutboxMessageColumnNextRetryAt+" IS NULL OR "+model.OutboxMessageColumnNextRetryAt+" <= ?)", param.Now).
		Order(model.OutboxMessageColumnCreatedAt + " ASC").
		Order(model.OutboxMessageColumnID + " ASC").
		Limit(param.Limit).
		Find(&messages).Error; err != nil {
		return nil, err
	}

	// 无可用消息，返回空列表
	if len(messages) == 0 {
		return []*model.OutboxMessage{}, nil
	}

	messageIDs := make([]uint64, 0, len(messages))
	for _, message := range messages {
		if message == nil || message.ID == 0 {
			continue
		}
		messageIDs = append(messageIDs, message.ID)
	}

	if len(messageIDs) == 0 {
		messages = messages[:0]
		return []*model.OutboxMessage{}, nil
	}

	// 批量抢占更新，只有更新成功的消息才算被当前 worker 抢占成功
	result := getDB(ctx, r.db, tx).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" IN ?", messageIDs).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusPending).
		Updates(map[string]interface{}{
			model.OutboxMessageColumnStatus:    model.OutboxMessageStatusProcessing,
			model.OutboxMessageColumnLockedBy:  param.WorkerID,
			model.OutboxMessageColumnLockedAt:  param.LockedAt,
			model.OutboxMessageColumnUpdatedAt: param.LockedAt,
		})

	if result.Error != nil {
		return nil, result.Error
	}

	// 再次查询被当前 worker 抢占成功的消息列表
	messages = messages[:0] // 重用切片底层数组
	if err := getDB(ctx, r.db, tx).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" IN ?", messageIDs).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusProcessing).
		Where(model.OutboxMessageColumnLockedBy+" = ?", param.WorkerID).
		Order(model.OutboxMessageColumnCreatedAt + " ASC").
		Order(model.OutboxMessageColumnID + " ASC").
		Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

// GetByID 根据ID获取 Outbox 消息
//
// 用于 Worker 锁定后重新读取最新消息状态
func (r *outboxMessageRepository) GetByID(ctx context.Context, messageID uint64) (*model.OutboxMessage, error) {
	if messageID == 0 {
		return nil, ErrInvalidOutboxMessageParam
	}

	var message model.OutboxMessage

	err := getDB(ctx, r.db, nil).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" = ?", messageID).
		First(&message).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOutboxMessageNotFound
		}
		return nil, err
	}

	return &message, nil
}

// MarkOutboxSentParam 标记 Outbox 消息发送成功参数
type MarkOutboxSentParam struct {
	MessageID uint64    // Outbox 消息ID
	SentAt    time.Time // 发送成功时间
}

// MarkAsPublished 标记 Outbox 消息发送成功
//
// 用于 Outbox Worker 发布 RabbitMQ 成功后更新状态
func (r *outboxMessageRepository) MarkAsPublished(ctx context.Context, param *MarkOutboxSentParam) error {
	if param == nil || param.MessageID == 0 || param.SentAt.IsZero() {
		return ErrInvalidOutboxMessageParam
	}

	result := getDB(ctx, r.db, nil).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" = ?", param.MessageID).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusProcessing).
		Updates(map[string]interface{}{
			model.OutboxMessageColumnStatus:       model.OutboxMessageStatusSent,
			model.OutboxMessageColumnSentAt:       param.SentAt,
			model.OutboxMessageColumnLockedBy:     nil,
			model.OutboxMessageColumnLockedAt:     nil,
			model.OutboxMessageColumnErrorMessage: nil,
			model.OutboxMessageColumnUpdatedAt:    param.SentAt,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrOutboxNoRowsUpdated
	}

	return nil
}

// MarkOutboxRetryParam 标记 Outbox 消息重试参数
type MarkOutboxRetryParam struct {
	MessageID    uint64    // Outbox 消息ID
	NextRetryAt  time.Time // 下次重试时间
	ErrorMessage string    // 错误信息
	UpdatedAt    time.Time // 更新时间
}

// MarkAsRetry 标记 Outbox 消息重新进入 pending 状态
//
// 用于 Outbox Worker 发布 RabbitMQ 失败但未超过最大重试次数时调用
func (r *outboxMessageRepository) MarkAsRetry(ctx context.Context, param *MarkOutboxRetryParam) error {
	if param == nil || param.MessageID == 0 || param.NextRetryAt.IsZero() || param.UpdatedAt.IsZero() {
		return ErrInvalidOutboxMessageParam
	}

	result := getDB(ctx, r.db, nil).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" = ?", param.MessageID).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusProcessing).
		Updates(map[string]interface{}{
			model.OutboxMessageColumnStatus:       model.OutboxMessageStatusPending,
			model.OutboxMessageColumnRetryCount:   gorm.Expr(model.OutboxMessageColumnRetryCount + " + 1"),
			model.OutboxMessageColumnNextRetryAt:  param.NextRetryAt,
			model.OutboxMessageColumnErrorMessage: param.ErrorMessage,
			model.OutboxMessageColumnLockedBy:     nil,
			model.OutboxMessageColumnLockedAt:     nil,
			model.OutboxMessageColumnUpdatedAt:    param.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrOutboxNoRowsUpdated
	}

	return nil
}

// MarkOutboxFailedParam 标记 Outbox 消息最终失败参数
type MarkOutboxFailedParam struct {
	MessageID    uint64    // Outbox 消息ID
	ErrorMessage string    // 错误信息
	UpdatedAt    time.Time // 更新时间
}

// MarkAsFailed 标记 Outbox 消息最终失败
//
// 用于 Outbox Worker 发布 RabbitMQ 失败且超过最大重试次数时调用
func (r *outboxMessageRepository) MarkAsFailed(ctx context.Context, param *MarkOutboxFailedParam) error {
	if param == nil || param.MessageID == 0 || param.UpdatedAt.IsZero() {
		return ErrInvalidOutboxMessageParam
	}

	result := getDB(ctx, r.db, nil).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnID+" = ?", param.MessageID).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusProcessing).
		Updates(map[string]interface{}{
			model.OutboxMessageColumnStatus:       model.OutboxMessageStatusFailed,
			model.OutboxMessageColumnRetryCount:   gorm.Expr(model.OutboxMessageColumnRetryCount + " + 1"),
			model.OutboxMessageColumnErrorMessage: param.ErrorMessage,
			model.OutboxMessageColumnLockedBy:     nil,
			model.OutboxMessageColumnLockedAt:     nil,
			model.OutboxMessageColumnUpdatedAt:    param.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrOutboxNoRowsUpdated
	}

	return nil
}

// ResetTimeoutProcessingMessagesParam 重置超时 processing 消息参数
type ResetTimeoutProcessingMessagesParam struct {
	Before    time.Time // 超时时间边界
	UpdatedAt time.Time // 更新时间
}

// ResetTimeoutProcessingMessages 重置超时 processing 消息
//
// 用于 Worker 启动或定时任务恢复异常中断的消息
func (r *outboxMessageRepository) ResetTimeoutProcessingMessages(ctx context.Context, param *ResetTimeoutProcessingMessagesParam) error {
	if param == nil || param.Before.IsZero() || param.UpdatedAt.IsZero() {
		return ErrInvalidOutboxMessageParam
	}

	return getDB(ctx, r.db, nil).
		Model(&model.OutboxMessage{}).
		Where(model.OutboxMessageColumnStatus+" = ?", model.OutboxMessageStatusProcessing).
		Where(model.OutboxMessageColumnLockedAt+" < ?", param.Before).
		Updates(map[string]interface{}{
			model.OutboxMessageColumnStatus:    model.OutboxMessageStatusPending,
			model.OutboxMessageColumnLockedBy:  nil,
			model.OutboxMessageColumnLockedAt:  nil,
			model.OutboxMessageColumnUpdatedAt: param.UpdatedAt,
		}).Error
}
