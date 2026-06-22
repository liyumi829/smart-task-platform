// internal/mq/rabbitmq/config.go
// package rabbitmq
// 功能：RabbitMQ 配置定义和默认值处理。

package rabbitmq

import "smart-task-platform/internal/bootstrap"

// PublisherConfig 定义了 RabbitMQ 消息发布器的配置项
// 直接使用 bootstrap.RabbitMQConfig 以保持一致性
type PublisherConfig = bootstrap.RabbitMQConfig

// ConsumerConfig 定义了 RabbitMQ 消息消费者的配置项
// 直接使用 bootstrap.RabbitMQConfig 以保持一致性
type ConsumerConfig = bootstrap.RabbitMQConfig
