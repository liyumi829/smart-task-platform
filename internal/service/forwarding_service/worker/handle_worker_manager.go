// internal/service/forwarding_service/worker/handle_worker_manager.go
// package worker
// 管理 handle worker 的生命周期
package worker

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/bootstrap"
	"sync"

	"go.uber.org/zap"
)

var (
	ErrHandleWorkerManagerAlreadyStarted       = errors.New("handle worker manager already started")           // Manager 已启动
	ErrHandleWorkerManagerNotStarted           = errors.New("handle worker manager not started")               // Manager 未启动
	ErrHandleWorkerManagerNilHandleMessage     = errors.New("handle worker manager handle is nil")             // Handle 为空
	ErrHandleWorkerManagerNilSubscriberFactory = errors.New("handle worker manager subscriber factory is nil") // SubscriberFactory 为空
	ErrHandleWorkerManagerNilPublisherFactory  = errors.New("handle worker manager publisher factory is nil")  // PublisherFactory 为空
	ErrHandleWorkerManagerInvalidWorkerCount   = errors.New("handle worker manager worker count invalid")      // Worker 数量非法
)

type HandleWorkerManagerConfig = bootstrap.HandleWorkerManagerConfig

// CloseableSubscriber 可关闭的消费者周期
type CloseableSubscriber interface {
	Subscribe(ctx context.Context) (<-chan *ConsumedMessage, error)

	Close() error
}

// 这里仍然使用工厂，但是所有的工作 channel 共用一个

// SubscriberFactory 外部注入，manager 不关心具体实现
type SubscriberFactory interface {
	// NewSubscriber 创建一个消费者
	NewSubscriber(ctx context.Context) (CloseableSubscriber, error)
}

// SubscriberFactoryFunc 函数适配器
type SubscriberFactoryFunc func(ctx context.Context) (CloseableSubscriber, error)

// NewSubscriber 实现 SubscriberFactory
func (f SubscriberFactoryFunc) NewSubscriber(ctx context.Context) (CloseableSubscriber, error) {
	return f(ctx)
}

//======
// 管理 worker 生命周期
//======

// HandleWorkerManager 管理多个 Handle Worker 生命周期
type HandleWorkerManager struct {
	config HandleWorkerManagerConfig // 配置信息

	workers          []*HandleWorker      // 管理的 worker 信息
	publisherFactory PublisherFactory     // 外部注入 publisher 创建工厂
	retryPublishers  []CloseablePublisher // 当前 manager 创建并需要关闭的 publisher

	subscriberFactory SubscriberFactory   // 外部注入的工厂
	subscriber        CloseableSubscriber // 当前 manager 创建的消费者句柄管理其生命周期
	deliveries        <-chan *ConsumedMessage

	wg      sync.WaitGroup     // 用于等待所有的 worker 退出
	mu      sync.Mutex         // 保护 started / ctx / cancel / workers / publishers
	ctx     context.Context    // 上下文
	cancel  context.CancelFunc // 取消函数
	started bool               // 是否已经启动

	logger *zap.Logger // 日志器
}

// NewHandleWorkerManager 创建 Handle Worker Manager
func NewHandleWorkerManager(
	config HandleWorkerManagerConfig,
	publisherFactory PublisherFactory,
	subscriberFactory SubscriberFactory,
	logger *zap.Logger,
) *HandleWorkerManager {
	if logger == nil {
		logger = zap.NewNop() // 空日志器，避免 nil 引用
	}

	return &HandleWorkerManager{
		config:            config,
		publisherFactory:  publisherFactory,
		subscriberFactory: subscriberFactory,
		logger: logger.With(
			zap.String("component", "handle_worker_manager"),
		),
	}
}

// Start 启动 Handle Worker Manager
//   - 非阻塞调用
//   - 启动 handle worker
func (m *HandleWorkerManager) Start(ctx context.Context, handleFunc HandleMessage) error {
	m.mu.Lock()

	if m.started {
		m.mu.Unlock()
		m.logger.Warn("Handle worker manager is already running")
		return ErrHandleWorkerManagerAlreadyStarted
	}

	if m.config.Disabled {
		m.mu.Unlock()
		m.logger.Info("Handle worker manager is disabled")
		return nil
	}

	// 检查配置依赖
	if err := m.validate(); err != nil {
		m.mu.Unlock()
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// 创建一个新的上下文，管理
	managerCtx, cancel := context.WithCancel(ctx)

	workers := make([]*HandleWorker, 0, m.config.WorkerCount)
	retryPublishers := make([]CloseablePublisher, 0, m.config.WorkerCount)

	// 创建 subscriber
	subscriber, err := m.subscriberFactory.NewSubscriber(managerCtx)
	if err != nil {
		cancel()
		m.logger.Error("Create subscriber for handle worker manager failed", zap.Error(err))
		return err
	}

	// 获取消费通道
	// 使用 managerCtx，确保 Close 后能正确退出订阅阻塞
	deliveries, err := subscriber.Subscribe(managerCtx)
	if err != nil {
		cancel()
		_ = subscriber.Close()
		m.mu.Unlock()
		m.logger.Error("Subscribe messages for handle worker manager failed", zap.Error(err))
		return err
	}

	// 创建 worker
	// 控制并发量 worker 数量就是
	for i := 0; i < m.config.WorkerCount; i++ {
		workerConfig := m.buildWorkerConfig(i)

		publisher, err := m.publisherFactory.NewPublisher(managerCtx, workerConfig.WorkerID, i)
		if err != nil {
			cancel()
			_ = subscriber.Close()
			closePublishers(retryPublishers)
			m.mu.Unlock()

			m.logger.Error("Create publisher for outbox worker failed",
				zap.String("worker_id", workerConfig.WorkerID),
				zap.Int("worker_index", i),
				zap.Error(err),
			)

			return err
		}

		worker := NewHandleWorker(
			*workerConfig,
			publisher,
			m.logger.With(
				zap.String("worker_id", workerConfig.WorkerID),
				zap.Int("worker_index", i),
			))

		// 管理资源
		retryPublishers = append(retryPublishers, publisher)
		workers = append(workers, worker)
	}

	// 全部初始化成功后，更新 manager 状态
	m.ctx = managerCtx
	m.cancel = cancel
	m.workers = workers
	m.retryPublishers = retryPublishers
	m.deliveries = deliveries
	m.subscriber = subscriber
	m.started = true

	for i, worker := range m.workers {
		index := i
		currentWorker := worker
		workerID := currentWorker.config.WorkerID

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()

			m.logger.Info("Handle worker launched successfully",
				zap.String("worker_id", workerID),
				zap.Int("worker_index", index),
			)

			currentWorker.Run(managerCtx, deliveries, handleFunc)

			m.logger.Info("Handle worker exited successfully",
				zap.String("worker_id", workerID),
				zap.Int("worker_index", index),
			)
		}()
	}

	m.logger.Info("Handle worker manager started successfully",
		zap.Int("worker_count", m.config.WorkerCount),
		zap.String("worker_id_prefix", m.config.WorkerIDPrefix),
		zap.Duration("stop_timeout", m.config.StopTimeout),
		zap.Int("max_retries", m.config.MaxRetries),
	)
	m.mu.Unlock()

	// 阻塞等待退出信号
	<-managerCtx.Done()

	// 等待所有 worker 退出
	m.wg.Wait()

	// 等幂调用，在 Close 中也会调用一次，确保资源释放
	if subscriber != nil {
		_ = subscriber.Close()
	}

	// 关闭 retry publisher
	closePublishers(retryPublishers)

	// 清理 manager 状态
	m.mu.Lock()
	m.started = false
	m.ctx = nil
	m.cancel = nil
	m.workers = nil
	m.retryPublishers = nil
	m.deliveries = nil
	m.subscriber = nil
	m.mu.Unlock()

	m.logger.Info("Handle worker manager exited successfully")

	return nil
}

// Close 停止 Handle Worker Manager
func (m *HandleWorkerManager) Close(ctx context.Context) error {
	m.mu.Lock()

	if !m.started {
		m.mu.Unlock()
		m.logger.Warn("Handle worker manager is not running")
		return ErrHandleWorkerManagerNotStarted
	}

	cancel := m.cancel
	stopTimeout := m.config.StopTimeout
	subscriber := m.subscriber
	m.mu.Unlock()

	m.logger.Info("Stopping handle worker manager",
		zap.Duration("stop_timeout", stopTimeout),
	)

	// 先关闭 subscriber，尽快打断阻塞消费
	if subscriber != nil {
		_ = subscriber.Close()
	}

	// 再取消 worker 上下文
	if cancel != nil {
		cancel()
	}

	if ctx == nil {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(context.Background(), stopTimeout)
		defer cancelTimeout()
	}

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:

		m.logger.Info("Handle worker manager stopped gracefully")
		return nil

	case <-ctx.Done():

		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			m.logger.Warn("Handle worker manager stop timed out",
				zap.Duration("stop_timeout", stopTimeout),
			)
			return context.DeadlineExceeded
		}

		m.logger.Warn("Handle worker manager stop canceled", zap.Error(ctx.Err()))
		return ctx.Err()
	}
}

// IsStarted 返回 manager 是否已启动
func (m *HandleWorkerManager) IsStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.started
}

//=====
// helper
//=====

// validate 检查依赖与配置
func (m *HandleWorkerManager) validate() error {
	if m.publisherFactory == nil {
		m.logger.Error("Handle worker publisher factory is nil")
		return ErrHandleWorkerManagerNilPublisherFactory
	}

	if m.subscriberFactory == nil {
		m.logger.Error("Handle worker subscriber factory is nil")
		return ErrHandleWorkerManagerNilSubscriberFactory
	}

	if m.config.WorkerCount <= 0 {
		m.logger.Error("Handle worker count invalid",
			zap.Int("worker_count", m.config.WorkerCount),
		)
		return ErrHandleWorkerManagerInvalidWorkerCount
	}

	return nil
}

func (m *HandleWorkerManager) buildWorkerConfig(index int) *HandleWorkerConfig {
	return &HandleWorkerConfig{
		WorkerID:           fmt.Sprintf("%s-%d", m.config.WorkerIDPrefix, index),
		HandleWorkerConfig: m.config.HandleWorkerConfig,
	}
}
