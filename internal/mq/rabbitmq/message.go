// internal/mq/rabbitmq/message.go
// package rabbitmq
// 功能：RabbitMQ 发布消息和消费消息结构定义。

package rabbitmq

import (
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	HeaderEventID    = "event_id"
	HeaderEventType  = "event_type"
	HeaderRetryCount = "x-retry-count"
)

// PublishMessage 发布消息
type PublishMessage struct {
	MessageID   string     // 消息ID
	EventID     string     // 事件ID
	EventType   string     // 事件类型
	Exchange    string     // 交换机名称
	RoutingKey  string     // 路由键键
	ContentType string     // 消息内容类型，例如 "application/json"
	Payload     []byte     // 消息内容
	Headers     amqp.Table // 消息头，可以包含额外的元数据
}

// ConsumedMessage 消费消息
type ConsumedMessage struct {
	MessageID   string        // 消息ID
	EventID     string        // 事件ID
	EventType   string        // 事件类型
	Exchange    string        // 交换机名称
	RoutingKey  string        // 路由键
	ContentType string        // 消息内容类型，例如 "application/json"
	Payload     []byte        // 消息内容
	Headers     amqp.Table    // 消息头，可以包含额外的元数据
	RetryCount  int           // 当前重试次数
	Timestamp   time.Time     // 消息发布事件
	raw         amqp.Delivery // 原始消息对象，包含 RabbitMQ 传递的所有信息
}

// Ack 确认消息消费成功
func (m *ConsumedMessage) Ack() error {
	return m.raw.Ack(false)
}

// Nack 拒绝消息
func (m *ConsumedMessage) Nack(requeue bool) error {
	return m.raw.Nack(false, requeue)
}

// Reject 拒绝消息
func (m *ConsumedMessage) Reject(requeue bool) error {
	return m.raw.Reject(requeue)
}
