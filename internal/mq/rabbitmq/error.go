// internal/mq/rabbitmq/errors.go
// package rabbitmq
// 功能：RabbitMQ 模块统一错误定义。

package rabbitmq

import "errors"

var (
	// ErrRabbitMQClosed 表示 RabbitMQ 组件已经关闭
	ErrRabbitMQClosed = errors.New("rabbitmq is closed")

	// ErrRabbitMQInvalidMessage 表示消息参数无效
	ErrRabbitMQInvalidMessage = errors.New("rabbitmq message is invalid")

	// ErrRabbitMQPublishNack 表示 RabbitMQ broker 返回 publish nack
	ErrRabbitMQPublishNack = errors.New("rabbitmq publish nacked")

	// ErrRabbitMQPublishReturned 表示消息无法被路由到目标队列
	ErrRabbitMQPublishReturned = errors.New("rabbitmq publish returned")

	// ErrRabbitMQMaxRetriesExceeded 表示消息已经超过最大重试次数
	ErrRabbitMQMaxRetriesExceeded = errors.New("rabbitmq max retries exceeded")
)
