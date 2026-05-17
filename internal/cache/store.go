// internal/cache/store.go
// Package cache 接口定义
package cache

import (
	"context"
	"io"
	"time"
)

// Store 缓存存储接口
type Store interface {
	// Set 同步设置缓存
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// MSet 同步批量设置缓存
	MSet(ctx context.Context, data map[string]string, ttl time.Duration) error

	// Get 获取缓存
	Get(ctx context.Context, key string) (value string, ok bool, err error)

	// MGet 批量获取缓存
	MGet(ctx context.Context, keys ...string) (map[string]string, error)

	// Del 同步删除缓存
	Del(ctx context.Context, keys ...string) error

	// DelWithDelay 延迟双删
	//   - 第一次同步删除
	//   - 延迟后异步再删一次
	//   - 适用于更新 DB 后需要保证缓存一致性的场景
	DelWithDelay(ctx context.Context, delay time.Duration, keys ...string) error

	// Close 优雅关闭
	io.Closer
}
