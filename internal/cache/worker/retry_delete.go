// internal/cache/worker/delay_delete.go
// Package worker
// 实现重试删除的 Worker
package worker

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RetryQueueFullStrategy 重试队列满时的策略
type RetryQueueFullStrategy int

const (
	// StrategyBlock 阻塞等待
	//   - 优点：保证不丢失删除任务
	//   - 缺点：可能导致业务请求变慢
	StrategyBlock RetryQueueFullStrategy = 1 + iota

	// StrategyDrop 直接丢弃
	//   - 优点：不阻塞业务请求
	//   - 缺点：可能丢失删除任务，导致脏缓存
	StrategyDrop

	// StrategyLog 记录日志后丢弃
	//   - 优点：不阻塞业务请求，且有日志可追溯
	//   - 缺点：可能丢失删除任务，需要人工介入
	StrategyLog
)

// RetryDeleteConfig 重试删除模块配置
type RetryDeleteConfig struct {
	ChanSize          int                    // 队列容量
	OperationTimeout  time.Duration          // 操作超时
	MaxRetryAttempts  int                    // 最大重试次数
	RetryBaseDelay    time.Duration          // 延迟
	WorkerCount       int                    // Worker 数量
	QueueFullStrategy RetryQueueFullStrategy // 队列满时的策略
	MaxWaitTime       time.Duration          // 队列满时的最大等待时间（仅在 StrategyBlock 时生效）
}

// setDefaultConfig 为未设置的字段填充默认值
func (c *RetryDeleteConfig) setDefaultConfig() {
	if c == nil {
		return
	}
	if c.ChanSize <= 0 {
		c.ChanSize = 1000
	}
	if c.OperationTimeout <= 0 {
		c.OperationTimeout = 500 * time.Millisecond
	}
	if c.MaxRetryAttempts <= 0 {
		c.MaxRetryAttempts = 3
	}
	if c.RetryBaseDelay <= 0 {
		c.RetryBaseDelay = 100 * time.Millisecond
	}
	if c.WorkerCount <= 0 {
		c.WorkerCount = 3
	}
	if c.QueueFullStrategy <= 0 {
		c.QueueFullStrategy = StrategyLog
	}
	if c.MaxWaitTime <= 0 {
		c.MaxWaitTime = 800 * time.Millisecond
	}
}

// RetryDeleteWorker 删除重试 worker
type RetryDeleteWorker struct {
	config   *RetryDeleteConfig // 配置
	deleter  CacheDeleter       // 缓存删除接口
	taskChan chan []string      // 任务队列
	wg       sync.WaitGroup     // 退出管理
	closed   chan struct{}
}

// NewRetryDeleteWorker 创建重试删除 worker
func NewRetryDeleteWorker(
	deleter CacheDeleter,
	config *RetryDeleteConfig,
) *RetryDeleteWorker {
	config.setDefaultConfig() // 合并默认配置

	worker := &RetryDeleteWorker{
		config:   config,
		deleter:  deleter,
		taskChan: make(chan []string, config.ChanSize),
		closed:   make(chan struct{}),
	}

	// 启动多个 worker
	worker.wg.Add(config.WorkerCount)
	for i := 0; i < config.WorkerCount; i++ {
		go worker.run(i)
	}

	zap.L().Info("retry delete workers started",
		zap.Int("worker_count", config.WorkerCount),
		zap.Int("chan_size", config.ChanSize),
		zap.Int("max_retry_attempts", config.MaxRetryAttempts),
		zap.Duration("retry_base_delay", config.RetryBaseDelay),
		zap.Duration("operation_timeout", config.OperationTimeout),
		zap.String("queue_full_strategy", getStrategyName(config.QueueFullStrategy)),
	)

	return worker
}

// Enqueue 将任务加入重试队列（根据策略处理）
func (w *RetryDeleteWorker) Enqueue(keys []string) {
	// 如果正在关闭，不再重试
	if w.deleter.IsStopping() {
		zap.L().Warn("retry delete worker is shutting down, task rejected",
			zap.Strings("keys", keys),
		)
		return
	}

	switch w.config.QueueFullStrategy {
	case StrategyBlock:
		// 阻塞等待（带超时）
		w.enqueueWithTimeout(keys)

	case StrategyDrop:
		// 直接丢弃
		select {
		case w.taskChan <- keys:
			zap.L().Info("retry delete task queued",
				zap.Strings("keys", keys),
			)
		// 如果队列满了，直接丢弃
		default:
			zap.L().Warn("retry delete channel full, task dropped (strategy: drop)",
				zap.Strings("keys", keys),
			)
		}

	case StrategyLog:
		// 记录日志后丢弃
		select {
		case w.taskChan <- keys:
			zap.L().Info("retry delete task queued",
				zap.Strings("keys", keys),
			)
		default:
			zap.L().Error("retry delete channel full, task dropped (strategy: log)",
				zap.Strings("keys", keys),
				zap.Int("chan_size", w.config.ChanSize),
			)
		}
	}
}

// Close 关闭 worker
func (w *RetryDeleteWorker) Close() error {
	close(w.closed)
	w.wg.Wait()
	close(w.taskChan)
	return nil
}

// enqueueWithTimeout 阻塞式入队（带超时）
func (w *RetryDeleteWorker) enqueueWithTimeout(keys []string) {
	select {
	case w.taskChan <- keys:
		zap.L().Info("retry delete task queued",
			zap.Strings("keys", keys),
		)
	case <-time.After(w.config.MaxWaitTime):
		// 超时后仍无法入队，记录错误
		zap.L().Error("retry delete channel full after timeout, task dropped",
			zap.Strings("keys", keys),
			zap.Int("chan_size", w.config.ChanSize),
			zap.Duration("max_wait_time", w.config.MaxWaitTime),
		)
	}
}

// run 运行 worker
func (w *RetryDeleteWorker) run(id int) {
	defer w.wg.Done()

	zap.L().Info("retry delete worker started",
		zap.Int("worker_id", id),
	)

	for {
		select {
		case <-w.closed:
			zap.L().Info("retry delete worker stopped",
				zap.Int("worker_id", id),
			)
			return

		case keys := <-w.taskChan:
			w.retryDelete(keys)
		}
	}
}

// retryDelete 重试删除，使用指数退避
func (w *RetryDeleteWorker) retryDelete(keys []string) {
	zap.L().Info("retry delete started",
		zap.Strings("keys", keys),
		zap.Int("max_attempts", w.config.MaxRetryAttempts),
	)

	for attempt := 0; attempt < w.config.MaxRetryAttempts; attempt++ {
		// 计算退避时间
		backoff := w.config.RetryBaseDelay * time.Duration(1<<attempt)
		timer := time.NewTimer(backoff)

		select {
		case <-w.closed:
			timer.Stop()
			zap.L().Info("retry delete stopped during backoff",
				zap.Strings("keys", keys),
				zap.Int("attempt", attempt+1),
			)
			return
		case <-timer.C:
			// timer 已触发，无需 Stop
		}
		// 等待重试时间到期

		// 执行删除
		ctx, cancel := context.WithTimeout(context.Background(), w.config.OperationTimeout)
		err := w.deleter.DeleteKeys(ctx, keys...)
		cancel()

		if err == nil {
			zap.L().Info("retry delete success",
				zap.Strings("keys", keys),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
			)
			return // 删除成功返回
		}

		// 删除失败：记录日志
		zap.L().Warn("retry delete attempt failed",
			zap.Strings("keys", keys),
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", w.config.MaxRetryAttempts),
			zap.Duration("backoff", backoff),
			zap.Error(err),
		)

		if attempt == w.config.MaxRetryAttempts-1 {
			zap.L().Error("retry delete exhausted all attempts",
				zap.Strings("keys", keys),
				zap.Int("max_attempts", w.config.MaxRetryAttempts),
				zap.Error(err),
			)
		}
	}
}

// getStrategyName 获取策略名称（用于日志）
func getStrategyName(strategy RetryQueueFullStrategy) string {
	switch strategy {
	case StrategyBlock:
		return "block"
	case StrategyDrop:
		return "drop"
	case StrategyLog:
		return "log"
	default:
		return "unknown"
	}
}
