// internal/cache/worker/interface.go
// Package worker
// 定义 worker 需要的缓存接口

package worker

import "context"

// CacheDeleter 缓存删除接口（供 worker 内部使用）
//
// 这个接口由 worker 定义，用于解耦：
//   - DelayDeleteWorker 和 RetryDeleteWorker 只依赖这个接口
//   - 未来可以轻松支持内存缓存、多级缓存等实现
type CacheDeleter interface {
	// DeleteKeys 删除指定的 keys
	//   - 这是一个底层删除操作，不会触发重试逻辑
	//   - 由 worker 调用，用于执行实际的删除
	DeleteKeys(ctx context.Context, keys ...string) error

	// IsStopping 检查是否正在关闭
	IsStopping() bool
}
