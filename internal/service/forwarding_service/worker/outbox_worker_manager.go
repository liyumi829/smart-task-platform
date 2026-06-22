// internal/worker/outbox_worker_manager.go
// Package worker
// 实现了 Outbox Worker Manager 的核心逻辑：管理多个 Outbox Worker 实例的生命周期，协调它们的工作
package worker

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/repository"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrOutboxWorkerManagerAlreadyStarted      = errors.New("outbox worker manager already started")          // Manager 已经启动
	ErrOutboxWorkerManagerNotStarted          = errors.New("outbox worker manager not started")              // Manager 未启动
	ErrOutboxWorkerManagerNilRepository       = errors.New("outbox worker manager repository is nil")        // Repository 为空
	ErrOutboxWorkerManagerNilTxManager        = errors.New("outbox worker manager tx manager is nil")        // TxManager 为空
	ErrOutboxWorkerManagerNilPublisherFactory = errors.New("outbox worker manager publisher factory is nil") // PublisherFactory 为空
	ErrOutboxWorkerManagerInvalidWorkerCount  = errors.New("outbox worker manager worker count invalid")     // Worker 数量非法
)

// OutboxWorkerManagerConfig 配置信息
type OutboxWorkerManagerConfig = bootstrap.OutboxWorkerManagerConfig

// CloseablePublisher 表示 worker 需要的发布能力 + manager 需要的关闭能力
type CloseablePublisher interface {
	messagePublisher

	// Close 关闭发布器资源
	Close() error
}

// PublisherFactory 用于为每个 worker 创建独立 publisher
// 由外部注入，manager 不关心具体实现
type PublisherFactory interface {
	// NewPublisher 为指定 worker 创建 publisher
	NewPublisher(ctx context.Context, workerID string, workerIndex int) (CloseablePublisher, error)
}

// PublisherFactoryFunc 函数适配器，方便用函数直接实现 PublisherFactory
type PublisherFactoryFunc func(ctx context.Context, workerID string, workerIndex int) (CloseablePublisher, error)

// NewPublisher 实现 PublisherFactory
func (f PublisherFactoryFunc) NewPublisher(ctx context.Context, workerID string, workerIndex int) (CloseablePublisher, error) {
	return f(ctx, workerID, workerIndex)
}

//=======
// 管理 worker 生命周期
//=======

// OutboxWorkerManager 管理多个 Outbox Worker 生命周期
type OutboxWorkerManager struct {
	config OutboxWorkerManagerConfig // 配置信息

	workers          []*OutboxWorker         // 管理的worker信息
	txMgr            *repository.TxManager   // 事务管理器
	repo             outboxMessageRepository // outboxMessage仓储管理
	publisherFactory PublisherFactory        // 外部注入 publisher 创建工厂
	publishers       []CloseablePublisher    // 当前 manager 创建并需要关闭的 publisher

	wg      sync.WaitGroup     // 用于等待所有的 worker 退出
	mu      sync.Mutex         // 保护 started / ctx / cancel / workers / publishers
	ctx     context.Context    // 上下文
	cancel  context.CancelFunc // 取消函数
	started bool               // 是否已经启动

	logger *zap.Logger // 日志器
}

// NewOutboxWorkerManager 创建 Outbox Worker Manager
func NewOutboxWorkerManager(
	config OutboxWorkerManagerConfig,
	txMgr *repository.TxManager,
	repo outboxMessageRepository,
	publisherFactory PublisherFactory,
	logger *zap.Logger,
) *OutboxWorkerManager {
	if logger == nil {
		logger = zap.NewNop() // 空日志器，避免 nil 引用
	}

	return &OutboxWorkerManager{
		config:           config,
		txMgr:            txMgr,
		repo:             repo,
		publisherFactory: publisherFactory,
		logger: logger.With(
			zap.String("component", "outbox_worker_manager"),
		),
	}
}

// Start 启动 Outbox Worker Manager
//   - 阻塞调用
//   - 启动 outbox worker
//   - Start 负责完整生命周期：启动 worker、阻塞等待 ctx 结束、等待 worker 退出、关闭 publisher、清理 manager 状态
func (m *OutboxWorkerManager) Start(ctx context.Context) error {
	m.mu.Lock()

	if m.started {
		m.logger.Warn("Outbox worker manager is already running")
		return ErrOutboxWorkerManagerAlreadyStarted
	}

	if m.config.Disabled {
		m.logger.Info("Outbox worker manager is disabled")
		return nil
	}

	// 检查配置依赖
	if err := m.validate(); err != nil {
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// 创建一个新的上下文，独立于外部传入的 ctx， 方便我们在 stop 方法中取消它
	managerCtx, cancel := context.WithCancel(ctx)

	workers := make([]*OutboxWorker, 0, m.config.WorkerCount)
	publishers := make([]CloseablePublisher, 0, m.config.WorkerCount)

	// 先创建所有 worker 和 publisher，全部成功后再启动，避免半启动状态
	for i := 0; i < m.config.WorkerCount; i++ {
		workerConfig := m.buildWorkerConfig(i)

		publisher, err := m.publisherFactory.NewPublisher(managerCtx, workerConfig.WorkerID, i)
		if err != nil {
			cancel()
			closePublishers(publishers)

			m.logger.Error("Create publisher for outbox worker failed",
				zap.String("worker_id", workerConfig.WorkerID),
				zap.Int("worker_index", i),
				zap.Error(err),
			)

			return err
		}

		worker := NewOutboxWorker(
			*workerConfig,
			m.txMgr,
			m.repo,
			publisher,
			m.logger.With(
				zap.String("worker_id", workerConfig.WorkerID),
				zap.Int("worker_index", i),
			),
		)

		publishers = append(publishers, publisher)
		workers = append(workers, worker)
	}
	// 所有的 worker 都启动成功了，才算 manager 启动成功
	m.ctx = managerCtx
	m.cancel = cancel
	m.workers = workers
	m.publishers = publishers
	m.started = true

	for i, worker := range m.workers { // 遍历worker，启动
		index := i
		currentWorker := worker
		workerID := currentWorker.config.WorkerID

		m.wg.Add(1)

		go func() {
			defer m.wg.Done()

			m.logger.Info("Outbox worker launched successfully",
				zap.String("worker_id", workerID),
				zap.Int("worker_index", index),
			)

			currentWorker.Run(managerCtx)

			m.logger.Info("Outbox worker exited successfully",
				zap.String("worker_id", workerID),
				zap.Int("worker_index", index),
			)
		}()
	}

	m.logger.Info("Outbox worker manager started successfully",
		zap.Int("worker_count", m.config.WorkerCount),
		zap.String("worker_id_prefix", m.config.WorkerIDPrefix),
		zap.Duration("stop_timeout", m.config.StopTimeout),
		zap.Duration("poll_interval", m.config.PollInterval),
		zap.Int("batch_size", m.config.BatchSize),
		zap.Duration("processing_timeout", m.config.ProcessingTimeout),
		zap.Duration("retry_backoff", m.config.RetryBackoff),
	)
	m.mu.Unlock() // 启动成功解锁

	// 等待退出
	<-managerCtx.Done()
	m.wg.Wait()                 // 等待所有 worker 退出
	closePublishers(publishers) // 退出后关闭 publisher 资源

	m.mu.Lock()
	m.started = false
	m.ctx = nil
	m.cancel = nil
	m.workers = nil
	m.publishers = nil
	m.mu.Unlock()

	return nil
}

// Close 停止 Outbox Worker Manager
//   - Close 只负责触发 cancel 并等待退出，不直接关闭 publisher，避免重复关闭资源
func (m *OutboxWorkerManager) Close(ctx context.Context) error {
	m.mu.Lock()

	if !m.started {
		m.mu.Unlock()
		m.logger.Warn("Outbox worker manager is not running")
		return ErrOutboxWorkerManagerNotStarted
	}

	cancel := m.cancel
	stopTimeout := m.config.StopTimeout
	m.mu.Unlock()

	if ctx == nil {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(context.Background(), stopTimeout)
		defer cancelTimeout()
	}

	m.logger.Info("Stopping outbox worker manager",
		zap.Duration("stop_timeout", stopTimeout),
	)

	// 先停止 worker
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{}) // 退出信号
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done: // 等待退出

		m.logger.Info("Outbox worker manager stopped gracefully")
		return nil

	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			m.logger.Warn("Outbox worker manager stop timed out",
				zap.Duration("stop_timeout", stopTimeout),
			)
			return context.DeadlineExceeded
		}

		m.logger.Warn("Outbox worker manager stop canceled", zap.Error(ctx.Err()))
		return ctx.Err()
	}
}

// IsStarted 返回 manager 是否已启动
func (m *OutboxWorkerManager) IsStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.started
}

// =======
// helper
// =======

// validate 校验 manager 依赖
func (m *OutboxWorkerManager) validate() error {
	if m.txMgr == nil {
		m.logger.Error("Outbox worker tx manager is nil")
		return ErrOutboxWorkerManagerNilTxManager
	}

	if m.repo == nil {
		m.logger.Error("Outbox worker repository is nil")
		return ErrOutboxWorkerManagerNilRepository
	}

	if m.publisherFactory == nil {
		m.logger.Error("Outbox worker publisher factory is nil")
		return ErrOutboxWorkerManagerNilPublisherFactory
	}

	if m.config.WorkerCount <= 0 {
		m.logger.Error("Outbox worker count invalid",
			zap.Int("worker_count", m.config.WorkerCount),
		)
		return ErrOutboxWorkerManagerInvalidWorkerCount
	}

	if m.config.StopTimeout <= 0 {
		m.config.StopTimeout = 10 * time.Second
	}

	return nil
}

// buildWorkerConfig 构建单个 Worker 配置
func (m *OutboxWorkerManager) buildWorkerConfig(index int) *OutboxWorkerConfig {
	return &OutboxWorkerConfig{
		WorkerID:           fmt.Sprintf("%s-%d", m.config.WorkerIDPrefix, index+1),
		OutboxWorkerConfig: m.config.OutboxWorkerConfig,
	}
}

// closePublishers 关闭 publisher 列表
func closePublishers(publishers []CloseablePublisher) {
	for _, publisher := range publishers {
		if publisher != nil {
			_ = publisher.Close()
		}
	}
}
