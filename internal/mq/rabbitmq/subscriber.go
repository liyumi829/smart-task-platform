// internal/mq/rabbitmq/subscriber.go
// package rabbitmq
// 功能：RabbitMQ 消费者实现，支持手动 ack、nack、retry queue、dead letter queue、prefetch、优雅关闭。

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/pkg/utils"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	defaultSubscriberReconnectAttempts = 3                      // 默认重连次数
	defaultSubscriberReconnectBackoff  = 300 * time.Millisecond // 默认重连退避时间
)

// Subscriber RabbitMQ 消费者
// 消费者只负责订阅消息，不处理业务逻辑
type Subscriber struct {
	config ConsumerConfig   // 消费者配置
	logger *zap.Logger      // 日志器
	conn   *amqp.Connection // RabbitMQ 连接
	ch     *amqp.Channel    // RabbitMQ 信道

	mu      sync.Mutex // 保护 connection / channel / closed / started / subscribe
	closed  bool       // 是否关闭
	started bool       // 是否已经开始订阅
	ownConn bool       // 是否由 Subscriber 自己创建连接

	ctx    context.Context    // 内部运行上下文
	cancel context.CancelFunc // 内部取消函数
	wg     sync.WaitGroup     // 等待后台协程退出
}

// NewSubscriber 创建 RabbitMQ 消费者
//   - conn == nil：Subscriber 自己创建连接，Close 时关闭连接；
//   - conn != nil：Subscriber 复用外部连接，Close 时默认只关闭自己的 channel；
func NewSubscriber(config ConsumerConfig, logger *zap.Logger, conn *amqp.Connection) (*Subscriber, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	s := &Subscriber{
		config: config,
		logger: logger.With(
			zap.String("component", "rabbitmq_subscriber"),
			zap.String("queue", config.QueueName),
		),
	}

	s.mu.Lock()
	err := s.connectLocked(conn)
	s.mu.Unlock()

	if err != nil {
		return nil, err
	}

	s.logger.Info("RabbitMQ subscriber created successfully")

	return s, nil
}

// Subscribe 订阅消息
//   - Subscribe 返回消费数据，不负责处理结果；
//   - 上层业务收到消息后，自行决定 Ack / Nack；
//   - 当前实现单个 Subscriber 只允许启动一次订阅
func (s *Subscriber) Subscribe(ctx context.Context) (<-chan *ConsumedMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrRabbitMQClosed
	}

	if s.started {
		return nil, fmt.Errorf("rabbitmq subscriber already started")
	}

	runCtx, cancel := context.WithCancel(ctx)

	s.ctx = runCtx
	s.cancel = cancel
	s.started = true

	out := make(chan *ConsumedMessage, s.config.PrefetchCount)
	s.wg.Add(1)
	go s.consumeLoop(runCtx, out)

	s.logger.Info("RabbitMQ subscriber started successfully",
		zap.String("queue", s.config.QueueName),
		zap.String("consumer_tag", s.config.ConsumerTag),
	)

	return out, nil
}

// Reconnect 主动重连
//   - 可用于外部健康检查发现 RabbitMQ 异常后主动恢复
func (s *Subscriber) Reconnect(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrRabbitMQClosed
	}

	return s.reconnectLocked(ctx)
}

// Close 关闭消费者
func (s *Subscriber) Close() error {

	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return nil
	}

	s.closed = true

	// 先取消上下文
	// 取消上下文之后，消费循环退出
	if s.cancel != nil {
		s.cancel()
	}

	closeTimeout := s.config.SubscribeCloseTimeout

	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(closeTimeout):
		s.logger.Warn("RabbitMQ subscriber close timed out",
			zap.Duration("close_timeout", closeTimeout),
		)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeChannelLocked()
	s.closeConnIfOwnedLocked()

	s.logger.Info("RabbitMQ subscriber closed gracefully")

	return nil
}

// IsClosed 判断消费者是否关闭
func (s *Subscriber) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closed
}

//=====
// helper
//=====

// consumeLoop 消费循环
//   - 持续从 RabbitMQ 获取 delivery；
//   - 出现 channel / connection 异常时自动恢复；
//   - 把消息包装为 ConsumedMessage 返回给上层业务
func (s *Subscriber) consumeLoop(ctx context.Context, out chan<- *ConsumedMessage) {
	defer s.wg.Done()
	defer close(out)

	for {
		if err := ctx.Err(); err != nil { // 先判断是否超时/取消
			s.logger.Info("RabbitMQ subscriber consume loop stopped", zap.Error(err))
			return
		}

		var reason error
		// 一旦 channel 断开，旧的 deliveries <-chan amqp.Delivery 就永久关闭、永久作废、永远不会再收到消息
		// channel 关闭后，deliveries 会被自动 close，永远无法恢复
		// 所以这里需要在循环中创建 channel
		deliveries, err := s.consumeChannel(ctx)
		if err != nil { // 创建 channel 发生错误
			if ctx.Err() != nil {
				return
			}

			if !IsClosedError(err) {
				s.logger.Error("RabbitMQ subscriber consume create failed", zap.Error(err))
				return
			}
			reason = errors.New("subscriber consume create failed")
		} else {
			// 创建成功
			// forwardDeliveries 服务端一直向 deliveries 发送消息
			// 一般来说，这是一个死循环
			// 如果该调用返回 false，那么说明 channel/connection 出现问题
			if ok := s.forwardDeliveries(ctx, deliveries, out); ok {
				return
			}
			reason = errors.New("subscriber delivery closed")
		}
		// 两种情况：
		// 1. consumeChannel 错误，需要重连，尝试自动恢复
		// 2. forwardDeliveries 返回 false 说明 delivery channel 异常关闭，尝试自动恢复
		if reconnectErr := s.Reconnect(ctx); reconnectErr != nil {
			if ctx.Err() != nil {
				return
			}

			s.logger.Error("RabbitMQ subscriber reconnect failed",
				zap.NamedError("reason", reason),
				zap.Error(reconnectErr))
			return
		}
	}
	//
}

// consumeChannel 创建 Consume channel
func (s *Subscriber) consumeChannel(ctx context.Context) (<-chan amqp.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrRabbitMQClosed
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if ok := s.checkReadyLocked(); !ok {
		return nil, amqp.ErrClosed
	}

	deliveries, err := s.ch.Consume(
		s.config.QueueName,
		s.config.ConsumerTag, // 消费标签
		false,                // 是否自动确认，当前使用手动 ACK
		false,                // 是否排他消费者
		false,                // 不接收同一连接发出的消息
		false,                // 是否不等待服务器确认，直接开始消费
		nil,                  // 扩展参数
	)
	if err != nil {
		s.logger.Error("RabbitMQ subscriber consume failed", zap.Error(err))
		return nil, err
	}

	return deliveries, nil
}

// forwardDeliveries 转发 delivery 到业务层
// 返回值：
//   - true：正常结束，不需要重连
//   - false：连接中断 / delivery channel 关闭，需要重连
func (s *Subscriber) forwardDeliveries(ctx context.Context, deliveries <-chan amqp.Delivery, out chan<- *ConsumedMessage) bool {
	// 双重定优先级
	for {
		select {
		case <-ctx.Done():
			return true

		case delivery, ok := <-deliveries:
			if !ok {
				s.logger.Warn("RabbitMQ delivery channel closed")
				return false
			}

			message := buildConsumedMessage(delivery)

			select {
			case out <- message:
			case <-ctx.Done():
				return true
			}
		}
	}
}

// connectLocked 建立连接并初始化 channel
// 调用方必须持有 s.mu
func (s *Subscriber) connectLocked(conn *amqp.Connection) error {
	if conn == nil {
		newConn, err := amqp.Dial(s.config.URL())
		if err != nil {
			s.logger.Error("RabbitMQ subscriber connect failed", zap.Error(err))
			return err
		}
		conn = newConn
		s.ownConn = true
	} else {
		s.ownConn = false
	}
	s.conn = conn

	if err := s.initChannelLocked(); err != nil {
		s.closeConnIfOwnedLocked()
		return err
	}

	s.logger.Info("RabbitMQ subscriber connected successfully",
		zap.Int("prefetch_count", s.config.PrefetchCount),
	)

	return nil
}

// reconnectLocked 重连 RabbitMQ
// 调用方必须持有 s.mu
func (s *Subscriber) reconnectLocked(ctx context.Context) error {
	s.closeChannelLocked()
	if s.conn == nil || s.conn.IsClosed() {
		s.closeConnIfOwnedLocked()
		s.conn = nil
	}

	var lastErr error
	for attempt := 1; attempt <= defaultSubscriberReconnectAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		// connection 可用时只重建 channel
		if s.conn != nil && !s.conn.IsClosed() {
			if err := s.initChannelLocked(); err == nil {
				s.logger.Info("RabbitMQ subscriber channel rebuilt successfully",
					zap.Int("attempt", attempt),
				)
				return nil
			} else {
				lastErr = err
				s.closeChannelLocked()
			}
		} else {
			// connection 不可用时重建 connection + channel
			if conn, err := amqp.Dial(s.config.URL()); err != nil {
				lastErr = err
				s.logger.Warn("RabbitMQ subscriber reconnect failed",
					zap.Int("attempt", attempt),
					zap.Error(err),
				)
			} else {
				s.conn = conn
				s.ownConn = true

				if err := s.initChannelLocked(); err == nil {
					s.logger.Info("RabbitMQ subscriber reconnected successfully",
						zap.Int("attempt", attempt),
					)
					return nil
				} else {
					lastErr = err
					s.closeChannelLocked()
					s.closeConnIfOwnedLocked()

					s.logger.Warn("RabbitMQ subscriber channel rebuild failed",
						zap.Int("attempt", attempt),
						zap.Error(err),
					)
				}
			}
		}

		if !utils.SleepWithContext(ctx, defaultSubscriberReconnectBackoff*time.Duration(attempt)) {
			return ctx.Err()
		}
	}

	if lastErr == nil {
		lastErr = amqp.ErrClosed
	}

	return lastErr
}

// initChannelLocked 初始化 channel / topology / qos
// 调用方必须持有 s.mu
func (s *Subscriber) initChannelLocked() error {
	if s.conn == nil || s.conn.IsClosed() {
		return amqp.ErrClosed
	}

	ch, err := s.conn.Channel()
	if err != nil {
		s.logger.Error("RabbitMQ subscriber channel create failed", zap.Error(err))
		return err
	}

	s.ch = ch

	// 声明拓扑是幂等操作，重连后重新声明可以避免 RabbitMQ 重启后拓扑丢失
	if err := bootstrap.DeclareTopology(ch, &s.config); err != nil {
		_ = ch.Close()
		s.ch = nil

		s.logger.Error("RabbitMQ subscriber topology declare failed", zap.Error(err))
		return err
	}

	// prefetch 控制消费者一次最多获取多少未 ack 消息，避免服务被消息压垮
	if err := ch.Qos(
		s.config.PrefetchCount, // 一次性从 RabbitMQ 预取多少条消息
		0,                      // 预取消息大小（字节）
		false,                  // 限流作用范围 false = 每个消费者单独限流
	); err != nil {
		_ = ch.Close()
		s.ch = nil

		s.logger.Error("RabbitMQ subscriber qos set failed", zap.Error(err))
		return err
	}

	return nil
}

// checkReadyLocked 仅检查连接/信道是否处于可用状态
// 调用方必须持有 s.mu
func (s *Subscriber) checkReadyLocked() bool {
	return !s.closed && s.conn != nil && !s.conn.IsClosed() && s.ch != nil && !s.ch.IsClosed()
}

// closeChannelLocked 只关闭当前 Subscriber 自己的 channel
// 调用方必须持有 s.mu
func (s *Subscriber) closeChannelLocked() {
	if s.ch != nil {
		_ = s.ch.Cancel(s.config.ConsumerTag, false) // 优雅停止当前 consumer
		_ = s.ch.Close()
		s.ch = nil
	}
}

// closeConnIfOwnedLocked 只关闭 Subscriber 自己拥有的连接
// 调用方必须持有 s.mu
func (s *Subscriber) closeConnIfOwnedLocked() {
	if s.ownConn && s.conn != nil {
		_ = s.conn.Close()
	}

	if s.ownConn {
		s.conn = nil
	}
}
