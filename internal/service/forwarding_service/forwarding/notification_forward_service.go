// internal/service/forwarding_service/forwarding/notification_forward_service.go
// package forwarding
// 实现转发服务
package forwarding

import (
	"context"
	"encoding/json"
	"errors"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/pkg/codec"
	"smart-task-platform/internal/service/forwarding_service/websocket"
	"smart-task-platform/internal/service/forwarding_service/worker"
	"sync"

	gws "github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var (
	ErrNotificationServiceStarted    = errors.New("notification forward service already started") // ErrNotificationServiceStarted 表示服务已经启动
	ErrNotificationServiceNotStarted = errors.New("notification forward service not started")     // ErrNotificationServiceNotStarted 表示服务尚未启动
	ErrNotificationServiceStopping   = errors.New("notification forward service is stopping")     // ErrNotificationServiceStopping 表示服务正在关闭
	ErrNilWebSocketManager           = errors.New("websocket manager is nil")                     // ErrNilWebSocketManager 表示 websocket manager 为空
	ErrInvalidForwardMessage         = errors.New("invalid notification forward message")         // ErrInvalidForwardMessage 表示转发消息格式非法
	ErrNilClient                     = errors.New("user websocket connection is nil")             // ErrNilClient 表示传入的用户 websocket 连接为 nil，通常意味着用户离线或者连接异常
)

const (
	// DefaultForwardMaxPayloadSize 默认最大转发消息大小
	DefaultForwardMaxPayloadSize = 64 * 1024
)

// rabbitmq 中消费的数据
type ForwardMessage = dto.ForwardMessage

// NotificationConsumer 通知消费者接口
type NotificationConsumer interface {
	// 启动消费者
	Start(ctx context.Context, handleFunc worker.HandleMessage) error

	// 关闭消费者
	Close(ctx context.Context) error
}

// NotificationProducer 通知生产者接口
type NotificationProducer interface {
	// 启动生产者
	Start(ctx context.Context) error

	// 关闭生产者
	Close(ctx context.Context) error
}

// NotificationForwardService 通知转发服务
// 职责：
//  1. 处理 RabbitMQ 消费到的通知消息；
//  2. 提供 Gin WebSocket 升级入口；
//  3. 启动 RabbitMQ 消费者；
//  4. 优雅关闭消费者和 websocket 连接
type NotificationForwardService struct {
	manager     *websocket.Manager // websocket 连接管理器
	sendBufSize int                // websocket 发送缓冲区大小

	DropOfflineUser bool                 // 用户离线时是否直接 ACK 丢弃消息
	consumer        NotificationConsumer // 通知消费者
	producer        NotificationProducer // 通知生产者
	MaxPayloadSize  int                  // RabbitMQ 转发消息最大大小

	mu       sync.Mutex
	started  bool
	stopping bool
	wg       sync.WaitGroup
}

// NotificationForwardServiceOption 表示服务配置项
type NotificationForwardServiceOption func(*NotificationForwardService)

// WithWebSocketSendBufferSize 设置 websocket 发送队列大小
func WithWebSocketSendBufferSize(size int) NotificationForwardServiceOption {
	return func(s *NotificationForwardService) {
		if size > 0 {
			s.sendBufSize = size
		}
	}
}

// WithDropOfflineUser 设置离线用户消息处理策略
func WithDropOfflineUser(drop bool) NotificationForwardServiceOption {
	return func(s *NotificationForwardService) {
		s.DropOfflineUser = drop
	}
}

// WithMaxPayloadSize 设置最大消息大小
func WithMaxPayloadSize(size int) NotificationForwardServiceOption {
	return func(s *NotificationForwardService) {
		if size > 0 {
			s.MaxPayloadSize = size
		}
	}
}

// NewNotificationForwardService 创建通知转发服务
func NewNotificationForwardService(manager *websocket.Manager,
	consumer NotificationConsumer,
	producer NotificationProducer,
	opts ...NotificationForwardServiceOption,
) (*NotificationForwardService, error) {
	if manager == nil {
		return nil, ErrNilWebSocketManager
	}
	s := &NotificationForwardService{
		manager:         manager,
		consumer:        consumer,
		sendBufSize:     websocket.DefaultSendBufferSize,
		DropOfflineUser: true,
		producer:        producer,
		MaxPayloadSize:  DefaultForwardMaxPayloadSize,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}

	return s, nil
}

// HandleWebSocket 管理 websocket 的长连接
func (s *NotificationForwardService) HandleWebSocket(userID uint64, sessionID string, conn *gws.Conn) {
	if conn == nil {
		return
	}

	client, err := websocket.NewClient(userID, sessionID, conn, s.sendBufSize, zap.L())
	if err != nil {
		_ = conn.Close()
		return
	}

	if err = s.manager.Register(client); err != nil {
		client.Close()
		return
	}

	// Register 成功后再启动读写协程，避免注册失败导致 goroutine 泄漏
	client.Start(s.manager)
}

//	HandleForwardMessage 处理 RabbitMQ 消费到的通知转发消息
//
// RabbitMQ 消费者适配层负责将 ConsumedMessage.Body 以 []byte 形式传入该方法
func (s *NotificationForwardService) HandleForwardMessage(payload []byte) worker.HandlerResult {
	if len(payload) == 0 || len(payload) > s.MaxPayloadSize || !json.Valid(payload) {
		return worker.HandlerResult{
			Action: worker.ConsumeActionReject,
			Reason: ErrInvalidForwardMessage.Error(),
		}
	}

	var msg ForwardMessage
	if err := codec.Unmarshal(payload, &msg); err != nil {
		return worker.HandlerResult{
			Action: worker.ConsumeActionReject,
			Reason: ErrInvalidForwardMessage.Error(),
		}
	}

	if msg.NotificationID == 0 || msg.UserID == 0 {
		return worker.HandlerResult{
			Action: worker.ConsumeActionReject,
			Reason: ErrInvalidForwardMessage.Error(),
		}
	}

	// 将通知实时推送给目标用户
	client, exists := s.manager.GetByUserID(msg.UserID)
	if !exists || client == nil {
		return worker.HandlerResult{
			Action: worker.ConsumeActionAck, // 通知已经落库，我们这里直接确认即可
			Reason: ErrNilClient.Error(),
		}
	}
	if err := client.TrySend(payload); err != nil {
		// 用户离线时通常直接 ACK 丢弃，避免 RabbitMQ 反复重试离线实时推送消息
		// Notification 已经落库，用户之后仍然可以通过通知列表接口获取
		if errors.Is(err, websocket.ErrUserOffline) && s.DropOfflineUser {
			return worker.HandlerResult{
				Action: worker.ConsumeActionAck,
				Reason: websocket.ErrUserOffline.Error(),
			}
		}

		// 其他错误交给 RabbitMQ 重试机制处理
		return worker.HandlerResult{
			Action: worker.ConsumeActionRetry,
			Reason: err.Error(),
		}
	}

	return worker.HandlerResult{
		Action: worker.ConsumeActionAck,
		Reason: "notification forwarded successfully",
	}
}

// Start 启动通知转发服务
// 当前主要负责启动 RabbitMQ 消费者
//   - 这是一个非阻塞调用
func (s *NotificationForwardService) Start() error {
	s.mu.Lock()

	if s.started {
		s.mu.Unlock()
		return ErrNotificationServiceStarted
	}

	if s.stopping {
		s.mu.Unlock()
		return ErrNotificationServiceStopping
	}

	s.started = true
	s.stopping = false
	consumer := s.consumer
	producer := s.producer
	// 先计算需要启动的组件数量，必须在解锁前完成 wg.Add，
	// 避免与 Close 中的 wg.Wait 并发交错。
	startCount := 0
	if consumer != nil {
		startCount++
	}
	if producer != nil {
		startCount++
	}
	if startCount > 0 {
		s.wg.Add(startCount)
	}

	s.started = true
	s.stopping = false

	s.mu.Unlock()

	if consumer != nil {
		go func() {
			defer s.wg.Done()

			// consumer.Start 是阻塞方法，退出时如果只是上下文取消，不作为异常错误处理
			if err := consumer.Start(context.Background(), func(message *worker.ConsumedMessage) worker.HandlerResult {
				return s.HandleForwardMessage(message.Payload)
			}); err != nil && !isNotificationServiceStopError(err) {
				zap.L().Error("Failed to start notification consumer", zap.Error(err))
			}
		}()
	}

	if producer != nil {
		go func() {
			defer s.wg.Done()

			// producer.Start 是阻塞方法，退出时如果只是上下文取消，不作为异常错误处理
			if err := producer.Start(context.Background()); err != nil && !isNotificationServiceStopError(err) {
				zap.L().Error("Failed to start notification producer", zap.Error(err))
			}
		}()
	}

	return nil
}

// Close 优雅关闭通知转发服务
func (s *NotificationForwardService) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()

	if !s.started {
		s.mu.Unlock()
		return ErrNotificationServiceNotStarted
	}
	if s.stopping {
		s.mu.Unlock()
		return ErrNotificationServiceStopping
	}
	s.stopping = true

	consumer := s.consumer
	producer := s.producer
	s.mu.Unlock()

	var closeErr error

	// 不持锁调用外部 Close，避免死锁
	// 先停 consumer，再停 producer
	// 原因：
	// 1. 先停止消费入口，避免关闭过程中继续拉取新消息
	// 2. producer 后停，可以给正在收尾的消费逻辑保留更完整的资源窗口
	if consumer != nil {
		if err := consumer.Close(ctx); err != nil && !isNotificationServiceStopError(err) {
			closeErr = errors.Join(closeErr, err)
		}
	}
	if producer != nil {
		if err := producer.Close(ctx); err != nil && !isNotificationServiceStopError(err) {
			closeErr = errors.Join(closeErr, err)
		}
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 所有 RabbitMQ worker 退出后，再关闭 websocket 连接
		s.manager.UnregisterAll()

		s.mu.Lock()
		s.started = false
		s.stopping = false
		s.mu.Unlock()

		return closeErr

	case <-ctx.Done():
		// 超时时保持 started/stopping 状态不回滚
		// 避免底层组件尚未真正退出时服务被错误地再次启动
		return errors.Join(closeErr, ctx.Err())
	}
}

// IsStarted 返回服务是否已启动
func (s *NotificationForwardService) IsStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.started
}

// isNotificationServiceStopError 判断是否为正常停止导致的错误
func isNotificationServiceStopError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
