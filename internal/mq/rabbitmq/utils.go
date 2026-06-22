// internal/mq/rabbitmq/utils.go
// 工具

package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// CloneHeaders 复制 headers，避免修改原始对象
func CloneHeaders(headers amqp.Table) amqp.Table {
	cloned := amqp.Table{}

	for k, v := range headers {
		cloned[k] = v
	}

	return cloned
}

// GetStringHeader 获取 string header
func GetStringHeader(headers amqp.Table, key string) string {
	if headers == nil {
		return ""
	}

	value, ok := headers[key]
	if !ok || value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

// GetIntHeader 获取 int header
func GetIntHeader(headers amqp.Table, key string) int {
	if headers == nil {
		return 0
	}

	value, ok := headers[key]
	if !ok || value == nil {
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	default:
		return 0
	}
}

// buildConsumedMessage 构建消费消息
func buildConsumedMessage(delivery amqp.Delivery) *ConsumedMessage {
	headers := CloneHeaders(delivery.Headers)              // 拷贝 header
	eventID := GetStringHeader(headers, HeaderEventID)     // 获取事件ID
	eventType := GetStringHeader(headers, HeaderEventType) // 获取事件类型
	retryCount := GetIntHeader(headers, HeaderRetryCount)  // 获取重试次数

	if eventID == "" {
		eventID = delivery.MessageId
	}

	return &ConsumedMessage{
		MessageID:   delivery.MessageId,
		EventID:     eventID,
		EventType:   eventType,
		Exchange:    delivery.Exchange,
		RoutingKey:  delivery.RoutingKey,
		ContentType: delivery.ContentType,
		Payload:     delivery.Body,
		Headers:     headers,
		RetryCount:  retryCount,
		Timestamp:   delivery.Timestamp,
		raw:         delivery, // 用于 ack/nack
	}
}
