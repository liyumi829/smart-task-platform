// internal/model/outbox_message.go
// Package model
// Outbox 消息表 GORM 映射对象

package model

import (
	"time"

	"gorm.io/datatypes"
)

const (
	OutboxMessageTableName = "outbox_messages" // 表名

	OutboxMessageColumnID            = "id"              // Outbox消息ID
	OutboxMessageColumnEventID       = "event_id"        // 事件唯一ID
	OutboxMessageColumnEventType     = "event_type"      // 事件类型
	OutboxMessageColumnExchangeName  = "exchange_name"   // RabbitMQ交换机名称
	OutboxMessageColumnRoutingKey    = "routing_key"     // RabbitMQ路由键
	OutboxMessageColumnPayload       = "payload"         // 消息内容JSON
	OutboxMessageColumnStatus        = "status"          // 消息状态
	OutboxMessageColumnRetryCount    = "retry_count"     // 当前重试次数
	OutboxMessageColumnMaxRetryCount = "max_retry_count" // 最大重试次数
	OutboxMessageColumnNextRetryAt   = "next_retry_at"   // 下次允许重试时间
	OutboxMessageColumnLockedBy      = "locked_by"       // 当前处理Worker标识
	OutboxMessageColumnLockedAt      = "locked_at"       // 锁定时间
	OutboxMessageColumnSentAt        = "sent_at"         // 发送成功时间
	OutboxMessageColumnErrorMessage  = "error_message"   // 最近一次错误信息
	OutboxMessageColumnCreatedAt     = "created_at"      // 创建时间
	OutboxMessageColumnUpdatedAt     = "updated_at"      // 更新时间
)

const (
	OutboxMessageStatusPending    = "pending"    // 待发送
	OutboxMessageStatusProcessing = "processing" // 发送中
	OutboxMessageStatusSent       = "sent"       // 已发送
	OutboxMessageStatusFailed     = "failed"     // 发送失败
)

const (
	OutboxEventTypeNotificationCreated = "notification.created" // 通知创建
	OutboxEventTypeTaskAssigned        = "task.assigned"        // 任务指派
	OutboxEventTypeTaskDeleted         = "task.deleted"         // 任务删除
	OutboxEventTypeTaskStatusChanged   = "task.status_changed"  // 任务状态变更
	OutboxEventTypeCommentCreated      = "comment.created"      // 评论创建
	OutboxEventTypeCommentReply        = "comment.reply"        // 评论回复
)

const (
	DefaultOutboxExchangeName = "smart-task-platform.events"
	DefaultOutboxRoutingKey   = "smart-task-platform.notification"
)

const (
	DefaultOutboxMaxRetryCount = 3 // 默认最大重试次数
)

// OutboxMessage Outbox 消息表
type OutboxMessage struct {
	ID            uint64         `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	EventID       string         `gorm:"column:event_id;type:varchar(64);not null;uniqueIndex:uk_outbox_event_id" json:"event_id"`
	EventType     string         `gorm:"column:event_type;type:varchar(100);not null;index:idx_outbox_event_type" json:"event_type"`
	ExchangeName  string         `gorm:"column:exchange_name;type:varchar(100);not null" json:"exchange_name"`
	Payload       datatypes.JSON `gorm:"column:payload;type:json;not null" json:"payload"`
	RoutingKey    string         `gorm:"column:routing_key;type:varchar(100);not null" json:"routing_key"`
	Status        string         `gorm:"column:status;type:varchar(20);not null;default:pending;index:idx_outbox_status_next_retry,priority:1;index:idx_outbox_status_created,priority:1" json:"status"`
	RetryCount    int            `gorm:"column:retry_count;not null;default:0" json:"retry_count"`
	MaxRetryCount int            `gorm:"column:max_retry_count;not null;default:5" json:"max_retry_count"`
	NextRetryAt   *time.Time     `gorm:"column:next_retry_at;index:idx_outbox_status_next_retry,priority:2" json:"next_retry_at"`
	LockedBy      *string        `gorm:"column:locked_by;type:varchar(100)" json:"locked_by"`
	LockedAt      *time.Time     `gorm:"column:locked_at;index:idx_outbox_locked_at" json:"locked_at"`
	SentAt        *time.Time     `gorm:"column:sent_at" json:"sent_at"`
	ErrorMessage  *string        `gorm:"column:error_message;type:varchar(1000)" json:"error_message"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null;autoCreateTime;index:idx_outbox_status_created,priority:2" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (OutboxMessage) TableName() string {
	return OutboxMessageTableName
}
