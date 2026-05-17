// internal/cache/worker/delay_delete.go
// Package worker
// 实现延迟删除的 Worker
package worker

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DelayDeleteTask 延迟删除任务
type DelayDeleteTask struct {
	Keys  []string
	Delay time.Duration
}

// DelayDeleteConfig 延迟删除模块配置
type DelayDeleteConfig struct {
	ChanSize         int           // 队列容量
	OperationTimeout time.Duration // 操作超时
	ShutdownTimeout  time.Duration // 关闭超时
	WorkerCount      int           // Worker 数量
}

// setDefaultConfig 为未设置的字段填充默认值
func (c *DelayDeleteConfig) setDefaultConfig() {
	if c == nil {
		return
	}
	if c.ChanSize <= 0 {
		c.ChanSize = 1000
	}
	if c.OperationTimeout <= 0 {
		c.OperationTimeout = 500 * time.Millisecond
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 10 * time.Second
	}
	if c.WorkerCount <= 0 {
		c.WorkerCount = 1 // 默认一个
	}
}

// DelayDeleteWorker 延迟删除 worker
type DelayDeleteWorker struct {
	config     *DelayDeleteConfig   // 配置
	deleter    CacheDeleter         // 缓存删除接口
	retryQueue RetryEnqueuer        // 重试队列接口
	taskChan   chan DelayDeleteTask // 任务队列
	wg         sync.WaitGroup       // 退出管理
	closed     chan struct{}
}

// NewDelayDeleteWorker 创建延迟删除 worker
func NewDelayDeleteWorker(
	deleter CacheDeleter,
	retryQueue RetryEnqueuer,
	config *DelayDeleteConfig,
) *DelayDeleteWorker {
	config.setDefaultConfig() // 设置默认配置
	worker := &DelayDeleteWorker{
		config:     config,
		deleter:    deleter,
		retryQueue: retryQueue,
		taskChan:   make(chan DelayDeleteTask, config.ChanSize),
		closed:     make(chan struct{}),
	}

	// 启动 worker
	worker.wg.Add(config.WorkerCount)
	for i := 0; i < config.WorkerCount; i++ {
		go worker.run(i)
	}

	zap.L().Info("delay delete worker started",
		zap.Int("chan_size", config.ChanSize),
		zap.Duration("operation_timeout", config.OperationTimeout),
		zap.Duration("shutdown_timeout", config.ShutdownTimeout),
	)

	return worker
}

// Enqueue 将任务加入延迟删除队列
//
// - 提供给外部使用的入队列
func (w *DelayDeleteWorker) Enqueue(keys []string, delay time.Duration) {
	// 如果正在关闭，拒绝新任务
	if w.deleter.IsStopping() {
		zap.L().Warn("delay delete worker is shutting down, task rejected",
			zap.Strings("keys", keys),
		)
		return
	}

	select {
	// 加入延迟删除队列
	case w.taskChan <- DelayDeleteTask{
		Keys:  keys,
		Delay: delay,
	}:
		zap.L().Debug("delay delete task queued",
			zap.Strings("keys", keys),
			zap.Duration("delay", delay),
		)
	// 删除队列满了
	default:
		zap.L().Warn("delay delete channel full, task dropped",
			zap.Strings("keys", keys),
			zap.Int("chan_size", w.config.ChanSize),
		)
	}
}

// Close 关闭 worker
func (w *DelayDeleteWorker) Close() error {
	close(w.closed)
	w.wg.Wait()
	close(w.taskChan)
	return nil
}

// run 运行 worker
func (w *DelayDeleteWorker) run(id int) {
	defer w.wg.Done()

	zap.L().Info("delay delete worker started",
		zap.Int("worker_id", id),
	)

	for {
		select {
		// 收到关闭信号
		case <-w.closed:
			w.drain() // 先情况队列中的数据
			zap.L().Info("delay delete worker stopped",
				zap.Int("worker_id", id),
			)
			return

		// 获取延迟队列中的数据
		case task := <-w.taskChan:
			w.processTask(task)
		}
	}
}

// processTask 处理延迟删除任务
func (w *DelayDeleteWorker) processTask(task DelayDeleteTask) {
	// 等待指定延迟
	timer := time.NewTimer(task.Delay)

	select {
	case <-w.closed:
		timer.Stop()
		// 关闭时立即执行删除
		w.executeDelete(task.Keys)
		return
	case <-timer.C:
		// 延迟时间到，执行删除
		w.executeDelete(task.Keys)
	}
}

// executeDelete 执行删除操作
func (w *DelayDeleteWorker) executeDelete(keys []string) {
	ctx, cancel := context.WithTimeout(context.Background(), w.config.OperationTimeout)
	defer cancel()

	err := w.deleter.DeleteKeys(ctx, keys...)
	if err != nil {
		zap.L().Warn("delay delete failed",
			zap.Strings("keys", keys),
			zap.Error(err),
		)
		// 删除失败，加入重试队列
		w.retryQueue.Enqueue(keys)
	} else {
		zap.L().Debug("delay delete success",
			zap.Strings("keys", keys),
		)
	}
}

// drain 处理剩余任务（带超时）
func (w *DelayDeleteWorker) drain() {
	remaining := len(w.taskChan)
	if remaining == 0 {
		return
	}

	zap.L().Info("draining delay delete channel",
		zap.Int("remaining", remaining),
	)

	deadline := time.Now().Add(w.config.ShutdownTimeout)
	processed := 0

	for {
		if time.Now().After(deadline) {
			zap.L().Warn("delay delete drain timeout",
				zap.Int("processed", processed),
				zap.Int("remaining", len(w.taskChan)),
			)
			return
		}

		select {
		case task := <-w.taskChan:
			// 关闭时不等待延迟，立即执行
			w.executeDelete(task.Keys)
			processed++
		default:
			zap.L().Info("delay delete channel drained",
				zap.Int("processed", processed),
			)
			return
		}
	}
}

// RetryEnqueuer 重试队列接口（供 DelayDeleteWorker 使用）
type RetryEnqueuer interface {
	// Enqueue 将失败的删除任务加入重试队列
	Enqueue(keys []string)
}
