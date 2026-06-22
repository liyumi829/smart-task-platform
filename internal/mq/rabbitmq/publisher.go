// internal/mq/rabbitmq/publisher.go
// package rabbitmq
// 功能：RabbitMQ 生产者实现，支持 durable、persistent、mandatory、publish confirm、自动重连、channel 自动重建

package rabbitmq

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unsafe"

	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/pkg/utils"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	defaultPublisherReconnectAttempts = 3                      // 默认重连次数
	defaultPublisherReconnectBackoff  = 300 * time.Millisecond // 默认重连退避时间
	defaultPublisherReturnWait        = 50 * time.Millisecond  // 默认等待不可路由消息时间
)

// Publisher RabbitMQ 生产者
// 生产者只负责发布消息，不处理消费、不处理业务逻辑
type Publisher struct {
	config PublisherConfig  // 发布配置
	logger *zap.Logger      // 日志器
	conn   *amqp.Connection // RabbitMQ 连接
	ch     *amqp.Channel    // RabbitMQ 信道

	returns <-chan amqp.Return // mandatory=true 时不可路由消息返回通道

	mu      sync.Mutex // 保护 connection / channel / closed / publish
	closed  bool       // 是否关闭
	ownConn bool       // 是否由 Publisher 自己创建连接
}

// NewPublisher 创建 RabbitMQ 生产者
//   - conn == nil：Publisher 自己创建连接，Close 时关闭连接；
//   - conn != nil：Publisher 复用外部连接，Close 时默认只关闭自己的 channel；
//   - 如果外部连接后续失效，Publisher 会使用 config.URL 自建新连接恢复发布能力
func NewPublisher(config PublisherConfig, logger *zap.Logger, conn *amqp.Connection) (*Publisher, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	p := &Publisher{
		config: config,
		logger: logger.With(
			zap.String("component", "rabbitmq_publisher"),
			zap.String("exchange", config.ExchangeName),
		),
	}

	p.mu.Lock()
	err := p.connectLocked(conn)
	p.mu.Unlock()

	if err != nil {
		return nil, err
	}

	p.logger.Info("RabbitMQ publisher created successfully")

	return p, nil
}

// Publish 发布消息
// 注意：发布成功只代表消息成功进入 RabbitMQ，不代表消费者已经处理成功
func (p *Publisher) Publish(ctx context.Context, message *PublishMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if message == nil {
		return ErrRabbitMQInvalidMessage
	}

	p.normalizeMessage(message)

	if err := p.validateMessage(message); err != nil {
		return err
	}

	var lastErr error

	// 最多尝试：首次发布 + 若干次重连重试
	// 需要短期的发布重试
	// 1. 网络不可靠
	// 2. Channel 会被 RabbitMQ 主动关闭
	for attempt := 0; attempt <= defaultPublisherReconnectAttempts; attempt++ {
		if attempt > 0 {
			p.logger.Warn("RabbitMQ publisher retry publish after reconnect",
				zap.String("message_id", message.MessageID),
				zap.String("event_id", message.EventID),
				zap.Int("attempt", attempt),
				zap.Error(lastErr),
			)
		}

		var err error
		if err = p.publishOnce(ctx, message); err == nil {
			return nil // 发布成功返回
		} else {
			lastErr = err // 发布失败记录错误
		}

		// 只有 connection/channel 关闭类错误才触发自动重连
		if !IsClosedError(err) {
			return err
		}

		// 如果是关闭类错误进行重连尝试
		if reconnectErr := p.Reconnect(ctx); reconnectErr != nil {
			return reconnectErr
		}
	}

	return lastErr
}

// Reconnect 主动重连
//   - 可用于外部健康检查发现 RabbitMQ 异常后主动恢复
func (p *Publisher) Reconnect(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrRabbitMQClosed
	}

	return p.reconnectLocked(ctx) // 复用持锁重连
}

// Close 关闭生产者
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	p.closeChannelLocked()
	p.closeConnIfOwnedLocked()

	p.logger.Info("RabbitMQ publisher closed gracefully")

	return nil
}

// IsClosed 判断生产者是否关闭
func (p *Publisher) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.closed
}

//=====
// helper
//=====

// publishOnce 执行单次发布
// 整个发布过程串行化，避免同一个 channel 并发 publish/confirm/return 混乱
//   - amqp.Channel 不是并发安全的
//   - 一旦并发  协议帧错乱 Broker 直接关闭 Channel
func (p *Publisher) publishOnce(ctx context.Context, message *PublishMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrRabbitMQClosed
	}

	// 确保消息能够发送
	if ok := p.checkReadyLocked(); !ok {
		return amqp.ErrClosed // 不返回 ErrRabbitMQClosed 上面做了检查
	}

	// 构造 publish 的消息
	publishing := amqp.Publishing{
		MessageId:    message.MessageID,
		ContentType:  message.ContentType,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Headers:      message.Headers,
		Body:         message.Payload,
	}
	p.drainReturnsLocked() // 在 publish 之前清空历史的不可路由消息
	publishCtx, cancel := context.WithTimeout(ctx, p.config.PublishTimeout)
	defer cancel()
	// 使用 PublishWithDeferredConfirmWithContext 等待当前消息的 confirm，避免和其他消息确认混淆
	confirm, err := p.ch.PublishWithDeferredConfirmWithContext( // 非阻塞调用
		publishCtx,         // 上下文，用于超时控制
		message.Exchange,   // 交换机名称
		message.RoutingKey, // 路由键
		true,               // mandatory 不可路由消息会进入 NotifyReturn
		false,              // 允许消息进入队列等待
		publishing,         // 消息内容结构体
	)
	if err != nil {
		p.logger.Error("RabbitMQ message publish failed",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.String("routing_key", message.RoutingKey),
			zap.Error(err),
		)
		return err
	}

	ack, err := confirm.WaitContext(publishCtx)
	if err != nil {
		p.logger.Error("RabbitMQ publish confirm wait failed",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.Error(err),
		)
		return err
	}

	if !ack {
		p.logger.Error("RabbitMQ publish confirm nacked",
			zap.String("possible_reason", "possible:queue_full_or_reject_publish / exchange_no_perm / msg_too_large / broker_resource"),
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.Uintptr("message_size", unsafe.Sizeof(publishing)),
		)
		return ErrRabbitMQPublishNack
	}

	// 发送完成之后检查是否是不可路由消息
	if err := p.checkReturnLocked(message); err != nil {
		return err
	}

	p.logger.Info("RabbitMQ message published successfully",
		zap.String("message_id", message.MessageID),
		zap.String("event_id", message.EventID),
		zap.String("event_type", message.EventType),
		zap.String("routing_key", message.RoutingKey),
	)

	return nil
}

// connectLocked 建立连接并初始化 channel
// 调用方必须持有 p.mu
func (p *Publisher) connectLocked(conn *amqp.Connection) error {
	if conn == nil {
		newConn, err := amqp.Dial(p.config.URL())
		if err != nil {
			p.logger.Error("RabbitMQ publisher connect failed", zap.Error(err))
			return err
		}
		conn = newConn
		p.ownConn = true
	} else {
		p.ownConn = false
	}
	p.conn = conn

	if err := p.initChannelLocked(); err != nil {
		p.closeConnIfOwnedLocked()
		return err
	}

	p.logger.Info("RabbitMQ publisher connected successfully")

	return nil
}

// reconnectLocked 重连 RabbitMQ
// 调用方必须持有 p.mu
func (p *Publisher) reconnectLocked(ctx context.Context) error {
	p.closeChannelLocked()                  // 重连之前先关闭原通道
	if p.conn == nil || p.conn.IsClosed() { // 如果连接已经关闭，或者连接不可用，则关闭自己拥有的旧连接
		p.closeConnIfOwnedLocked()
		p.conn = nil
	}

	var lastErr error // 记录上一次错误
	for attempt := 1; attempt <= defaultPublisherReconnectAttempts; attempt++ {
		// 重连检查是否超时
		// ctx 发生错误（外部取消/超时）立刻退出，不浪费资源
		if err := ctx.Err(); err != nil {
			return err
		}

		// connection 仍然可用时，只重建 channel
		if p.conn != nil && !p.conn.IsClosed() {
			if err := p.initChannelLocked(); err == nil {
				p.logger.Info("RabbitMQ publisher channel rebuilt successfully",
					zap.Int("attempt", attempt),
				)
				return nil // 重建成功返回（快速返回）
			} else {
				lastErr = err          // 重建失败记录错误
				p.closeChannelLocked() // 关闭新建的 channel
			}
		} else { // connection 不可用
			// 重新建立 connection + 重建 channel
			if conn, err := amqp.Dial(p.config.URL()); err != nil {
				lastErr = err
				p.logger.Warn("RabbitMQ publisher reconnect failed",
					zap.Int("attempt", attempt),
					zap.Error(err),
				)
			} else {
				// 没有发生错误，尝试重建通道
				p.conn = conn
				p.ownConn = true

				if err := p.initChannelLocked(); err == nil {
					p.logger.Info("RabbitMQ publisher reconnected successfully",
						zap.Int("attempt", attempt),
					)
					return nil
				} else {
					lastErr = err
					p.closeChannelLocked()     // 关闭新建通道
					p.closeConnIfOwnedLocked() // 关闭新连接

					p.logger.Warn("RabbitMQ publisher channel rebuild failed",
						zap.Int("attempt", attempt),
						zap.Error(err),
					)
				}
			}
		}

		// 防止重连时 sleep 卡住整个程序
		if !utils.SleepWithContext(ctx, defaultPublisherReconnectBackoff*time.Duration(attempt)) {
			return ctx.Err()
		}
	}

	if lastErr == nil {
		lastErr = amqp.ErrClosed
	}

	return lastErr
}

// initChannelLocked 初始化 channel / topology / confirm / return
// 调用方必须持有 p.mu
// 使用场景
//   - 初始化 publisher
//   - 重连的时候
func (p *Publisher) initChannelLocked() error {
	if p.conn == nil || p.conn.IsClosed() {
		return amqp.ErrClosed
	}

	ch, err := p.conn.Channel()
	if err != nil {
		p.logger.Error("RabbitMQ publisher channel create failed", zap.Error(err))
		return err
	}

	p.ch = ch // 每个 Publisher 独占自己的 channel
	// 声明拓扑是幂等操作，重连后重新声明可以避免 RabbitMQ 重启后拓扑丢失
	if err := bootstrap.DeclareTopology(ch, &p.config); err != nil {
		_ = ch.Close()
		p.ch = nil

		p.logger.Error("RabbitMQ publisher topology declare failed", zap.Error(err))
		return err
	}

	// 开启 confirm 模式
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		p.ch = nil

		p.logger.Error("RabbitMQ publisher confirm mode enable failed", zap.Error(err))
		return err
	}

	p.returns = ch.NotifyReturn(make(chan amqp.Return, 16))

	return nil
}

// checkReadyLocked 仅检查连接/信道是否处于可用状态
// 调用方必须持有 p.mu
func (p *Publisher) checkReadyLocked() bool {
	return !p.closed && p.conn != nil && !p.conn.IsClosed() && p.ch != nil && !p.ch.IsClosed()
}

// checkReturnLocked 检查 mandatory return
// 调用方必须持有 p.mu
func (p *Publisher) checkReturnLocked(message *PublishMessage) error {
	if p.returns == nil {
		return nil
	}

	timer := time.NewTimer(defaultPublisherReturnWait)
	defer timer.Stop()

	select {
	case ret, ok := <-p.returns:
		if !ok {
			return nil
		}

		p.logger.Error("RabbitMQ message publish returned",
			zap.String("message_id", message.MessageID),
			zap.String("event_id", message.EventID),
			zap.String("exchange", ret.Exchange),
			zap.String("routing_key", ret.RoutingKey),
			zap.Uint16("reply_code", ret.ReplyCode),
			zap.String("reply_text", ret.ReplyText),
		)

		return ErrRabbitMQPublishReturned

	case <-timer.C:
		return nil
	}
}

// drainReturnsLocked 清理历史 return，避免污染当前发布结果
// 调用方必须持有 p.mu
func (p *Publisher) drainReturnsLocked() {
	if p.returns == nil {
		return
	}

	for {
		select {
		case <-p.returns:
		default:
			return
		}
	}
}

// closeChannelLocked 只关闭当前 Publisher 自己的 channel
// 调用方必须持有 p.mu
func (p *Publisher) closeChannelLocked() {
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}

	p.returns = nil
}

// closeConnIfOwnedLocked 只关闭 Publisher 自己拥有的连接
// 调用方必须持有 p.mu
func (p *Publisher) closeConnIfOwnedLocked() {
	if p.ownConn && p.conn != nil {
		_ = p.conn.Close()
	}

	if p.ownConn {
		p.conn = nil
	}
}

// IsClosedError 判断是否为 RabbitMQ 连接/信道关闭类错误
//   - 客户端状态错误（not open /closed）表示 channel 已关
//   - 网络错误（reset by peer /broken pipe）表示 TCP 已断：conn + channel 全挂
//   - 服务端 504 表示 服务端主动关 channel
func IsClosedError(err error) bool {
	if err == nil {
		return false
	}

	// 1. 优先判断明确的 closed 错误
	// connect/chan 关闭 或者 publisher 关闭
	if errors.Is(err, amqp.ErrClosed) || errors.Is(err, ErrRabbitMQClosed) {
		return true
	}

	// 2. 定义需要匹配的错误关键词
	closedKeywords := []string{
		"channel/connection is not open",
		"channel is closed",
		"connection is closed",
		"connection reset by peer",
		"broken pipe",
		"exception (504)",
	}

	msg := strings.ToLower(err.Error())
	for _, keyword := range closedKeywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}

	return false
}

// normalizeMessage 标准化发布消息
func (p *Publisher) normalizeMessage(message *PublishMessage) {
	if message.MessageID == "" {
		message.MessageID = uuid.NewString()
	}

	if message.EventID == "" {
		message.EventID = message.MessageID
	}

	if message.Exchange == "" {
		message.Exchange = p.config.ExchangeName
	}

	if message.RoutingKey == "" {
		message.RoutingKey = p.config.RoutingKey
	}

	if message.ContentType == "" {
		message.ContentType = "application/json"
	}

	if message.Headers == nil {
		message.Headers = amqp.Table{}
	}

	message.Headers[HeaderEventID] = message.EventID
	message.Headers[HeaderEventType] = message.EventType

	if _, ok := message.Headers[HeaderRetryCount]; !ok {
		message.Headers[HeaderRetryCount] = 0
	}
}

// validateMessage 校验发布消息
func (p *Publisher) validateMessage(message *PublishMessage) error {
	if message == nil {
		return ErrRabbitMQInvalidMessage
	}

	if message.Exchange == "" || message.RoutingKey == "" {
		return ErrRabbitMQInvalidMessage
	}

	if len(message.Payload) == 0 {
		return ErrRabbitMQInvalidMessage
	}

	return nil
}
