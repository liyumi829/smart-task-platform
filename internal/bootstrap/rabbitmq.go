// internal/bootstrap/rabbitmq.go
// package bootstrap
// 功能：初始化 RabbitMQ 连接池，并将其注入到全局依赖中。

package bootstrap

import (
	"errors"
	"fmt"
	"net/url"
	"smart-task-platform/internal/model"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

var (
	// ErrRabbitMQInvalidConfig 表示 RabbitMQ 配置无效
	ErrRabbitMQInvalidConfig = errors.New("rabbitmq config is invalid")
)

// RabbitMQConfig 定义了 RabbitMQ 连接配置
type RabbitMQConfig struct {
	// RabbitMQ 连接地址，支持 amqp://user:pass@host:port/vhost 格式
	Host     string `yaml:"host"`     // 主机地址
	User     string `yaml:"user"`     // 用户名
	Password string `yaml:"password"` // 密码

	// 连接超时时间
	ConnectTimeout time.Duration `yaml:"connect_timeout"`

	ExchangeName string `yaml:"exchange_name"` // 交换机名称
	ExchangeType string `yaml:"exchange_type"` // 交换机类型（如 direct、topic、fanout）
	RoutingKey   string `yaml:"routing_key"`   // 路由键
	QueueName    string `yaml:"queue_name"`    // 队列名称

	RetryExchangeName string        `yaml:"retry_exchange_name"` // 重试交换机名称
	RetryRoutingKey   string        `yaml:"retry_routing_key"`   // 重试路由键
	RetryQueueName    string        `yaml:"retry_queue_name"`    // 重试队列名称
	RetryDelay        time.Duration `yaml:"retry_delay"`         // 重试延迟时间（秒）

	DeadLetterExchangeName string `yaml:"dead_letter_exchange_name"` // 死信交换机名称
	DeadLetterRoutingKey   string `yaml:"dead_letter_routing_key"`   // 死信路由建
	DeadLetterQueueName    string `yaml:"dead_letter_queue_name"`    // 死信队列名称

	PublishTimeout time.Duration `yaml:"publish_timeout"` // 消息发布超时时间 默认 5s

	ConsumerTag           string        `yaml:"consumer_tag"`            // 消费者标签，用于区分不同的消费者实例
	PrefetchCount         int           `yaml:"prefetch_count"`          // 每次消费拉取的消息数量
	SubscribeCloseTimeout time.Duration `yaml:"subscribe_close_timeout"` // 消费连接关闭超时时间 默认 10s
}

// URL 生成 RabbitMQ 连接字符串
// 格式：amqp://user:password@host:port/
func (config RabbitMQConfig) URL() string {
	if config.User == "" && config.Password == "" {
		return fmt.Sprintf("amqp://%s/", config.Host)
	}
	user := url.QueryEscape(config.User)
	password := url.QueryEscape(config.Password)
	return fmt.Sprintf("amqp://%s:%s@%s/", user, password, config.Host)
}

// setDefault 设置默认值
func (c *RabbitMQConfig) setDefault() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 3 * time.Second
	}

	if c.ExchangeName == "" {
		c.ExchangeName = model.DefaultOutboxExchangeName
	}

	if c.ExchangeType == "" {
		c.ExchangeType = "direct"
	}

	if c.RoutingKey == "" {
		c.RoutingKey = model.DefaultOutboxRoutingKey
	}

	if c.QueueName == "" {
		c.QueueName = "smart-task-platform.notification.queue"
	}

	if c.RetryExchangeName == "" {
		c.RetryExchangeName = c.ExchangeName + ".retry"
	}

	if c.RetryQueueName == "" {
		c.RetryQueueName = c.QueueName + ".retry"
	}

	if c.RetryRoutingKey == "" {
		c.RetryRoutingKey = c.RoutingKey + ".retry"
	}

	if c.RetryDelay <= 0 {
		c.RetryDelay = 30 * time.Second
	}

	if c.DeadLetterExchangeName == "" {
		c.DeadLetterExchangeName = c.ExchangeName + ".dlx"
	}

	if c.DeadLetterQueueName == "" {
		c.DeadLetterQueueName = c.QueueName + ".dlq"
	}

	if c.DeadLetterRoutingKey == "" {
		c.DeadLetterRoutingKey = c.RoutingKey + ".dead"
	}

	if c.ConsumerTag == "" {
		c.ConsumerTag = "smart-task-platform-consumer"
	}

	if c.PrefetchCount <= 0 {
		c.PrefetchCount = 10
	}

	if c.PublishTimeout <= 0 {
		c.PublishTimeout = 5 * time.Second
	}

	if c.SubscribeCloseTimeout <= 0 {
		c.SubscribeCloseTimeout = 10 * time.Second
	}
}

// Validate 验证 RabbitMQ 配置的有效性
func (config RabbitMQConfig) Validate() error {
	if config.User == "" || config.Password == "" {
		return ErrRabbitMQInvalidConfig
	}
	if config.ExchangeName == "" {
		return ErrRabbitMQInvalidConfig
	}
	if config.RoutingKey == "" {
		return ErrRabbitMQInvalidConfig
	}
	return nil
}

// InitRabbitMQ 初始化 RabbitMQ 连接并声明RabbitMQ的拓扑结构
func InitRabbitMQ(config *RabbitMQConfig) *amqp.Connection {
	// 设置默认值
	config.setDefault()

	if err := config.Validate(); err != nil {
		zap.L().Fatal("Invalid RabbitMQ configuration", zap.Error(err))
	}

	// 连接 RabbitMQ
	url := config.URL()
	conn, err := amqp.DialConfig(url, amqp.Config{
		Dial: amqp.DefaultDial(config.ConnectTimeout),
	})

	if err != nil {
		zap.L().Fatal("Failed to connect to RabbitMQ", zap.String("url", url), zap.Error(err))
	}
	zap.L().Info("Successfully connected to RabbitMQ", zap.String("url", url))

	// 声明 RabbitMQ 拓扑结构
	ch, err := conn.Channel()
	if err != nil {
		zap.L().Fatal("Failed to create RabbitMQ channel", zap.Error(err))
	}

	if err := DeclareTopology(ch, config); err != nil {
		zap.L().Fatal("Failed to declare RabbitMQ topology", zap.Error(err))
	}

	zap.L().Info("Successfully declared RabbitMQ topology",
		zap.String("url", url),
		zap.Duration("connect_timeout", config.ConnectTimeout),
		zap.String("exchange_name", config.ExchangeName),
		zap.String("exchange_type", config.ExchangeType),
		zap.String("routing_key", config.RoutingKey),
		zap.String("queue_name", config.QueueName),
		zap.String("retry_exchange", config.RetryExchangeName),
		zap.String("retry_routing_key", config.RetryRoutingKey),
		zap.String("retry_queue", config.RetryQueueName),
		zap.Duration("retry_delay", config.RetryDelay),
		zap.String("dead_letter_exchange", config.DeadLetterExchangeName),
		zap.String("dead_letter_routing_key", config.DeadLetterRoutingKey),
		zap.String("dead_letter_queue", config.DeadLetterQueueName),
		zap.String("consumer_tag", config.ConsumerTag),
		zap.Int("prefetch_count", config.PrefetchCount),
		zap.Duration("publish_timeout", config.PublishTimeout),
		zap.Duration("close_timeout", config.SubscribeCloseTimeout))

	return conn
}

// DeclareTopology 声明 RabbitMQ 拓扑结构
func DeclareTopology(ch *amqp.Channel, config *RabbitMQConfig) error {
	if ch == nil {
		zap.L().Error("AMQP channel is nil, cannot declare topology")
		return errors.New("amqp channel is nil")
	}

	// 声明主 exchange
	if err := ch.ExchangeDeclare(
		config.ExchangeName, // 交换机名称
		config.ExchangeType, // 交换机类型
		true,                // 持久化
		false,               // 自动删除
		false,               // 内部使用
		false,               // 是否不等待声明完成
		nil,                 // 其它参数
	); err != nil {
		return err
	}

	// 声明 retry exchange
	if err := ch.ExchangeDeclare(
		config.RetryExchangeName,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	// 声明 dead letter exchange
	if err := ch.ExchangeDeclare(
		config.DeadLetterExchangeName,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	// 主队列配置 dead-letter，消费端 Nack(false) 后会进入死信队列
	mainQueueArgs := amqp.Table{
		"x-dead-letter-exchange":    config.DeadLetterExchangeName,
		"x-dead-letter-routing-key": config.DeadLetterRoutingKey,
	}

	// 声明主队列
	if _, err := ch.QueueDeclare(
		config.QueueName, // 队列名称
		true,             // 持久化
		false,            // 自动删除
		false,            // 排他性
		false,            // 是否不等待声明完成
		mainQueueArgs,
	); err != nil {
		return err
	}

	// 绑定主队列到主 exchange
	if err := ch.QueueBind(
		config.QueueName,
		config.RoutingKey,
		config.ExchangeName,
		false, // 是否不等待声明完成
		nil,
	); err != nil {
		return err
	}

	// retry 队列通过 TTL 实现固定延迟重试，到期后重新投递回主 exchange
	// "x-message-ttl"            消息存活时间（超时时间）
	// "x-dead-letter-exchange"    消息死了之后发给哪个交换机
	// "x-dead-letter-routing-key" 消息死了之后用什么路由键
	retryQueueArgs := amqp.Table{
		"x-message-ttl":             int64(config.RetryDelay / time.Millisecond),
		"x-dead-letter-exchange":    config.ExchangeName,
		"x-dead-letter-routing-key": config.RoutingKey,
	}

	// 声明重试队列
	if _, err := ch.QueueDeclare(
		config.RetryQueueName,
		true,
		false,
		false,
		false,
		retryQueueArgs,
	); err != nil {
		return err
	}

	if err := ch.QueueBind(
		config.RetryQueueName,
		config.RetryRoutingKey,
		config.RetryExchangeName,
		false,
		nil,
	); err != nil {
		return err
	}

	// 死信队列用于保存最终失败的消息
	if _, err := ch.QueueDeclare(
		config.DeadLetterQueueName,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	if err := ch.QueueBind(
		config.DeadLetterQueueName,
		config.DeadLetterRoutingKey,
		config.DeadLetterExchangeName,
		false,
		nil,
	); err != nil {
		return err
	}

	return nil
}
