// internal/cache/redis_store.go
// Package cache 利用 redis 作为缓存

// 主要实现几个接口
// 1、Set 设置key（同步）
// 2、Get 获取key（同步）
// 3、Del 删除key（同步+异步重试）
package cache

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"smart-task-platform/internal/cache/worker"
)

// RedisCacheStoreConfig Redis 缓存配置
type RedisCacheStoreConfig struct {
	worker.DelayDeleteConfig               // 延迟删除配置
	worker.RetryDeleteConfig               // 重试删除配置
	ShutdownTimeout          time.Duration // Store 级别的关闭超时（覆盖子模块的 ShutdownTimeout）
}

// setDefaultConfig 为未设置的字段填充默认值
func (c *RedisCacheStoreConfig) setDefaultConfig() {
	if c == nil {
		return
	}
	// Store 级别的关闭超时
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 5 * time.Second
	}
	// 统一子模块的 ShutdownTimeout
	c.DelayDeleteConfig.ShutdownTimeout = c.ShutdownTimeout
}

// RedisCacheStore 是 CacheStore 的 Redis 实现
type RedisCacheStore struct {
	config      RedisCacheStoreConfig     // 配置
	rdb         goredis.UniversalClient   // Redis 客户端
	delayDelete *worker.DelayDeleteWorker // 延迟删除
	retryDelete *worker.RetryDeleteWorker // 重试模块

	// 用于关闭
	closed   chan struct{}
	stopping atomic.Bool // 标记正在关闭，拒绝新任务
}

// NewRedisCacheStore 创建 RedisCacheStore（使用默认配置）
func NewRedisCacheStore(rdb goredis.UniversalClient) *RedisCacheStore {
	return NewRedisCacheStoreWithConfig(rdb, &RedisCacheStoreConfig{})
}

// NewRedisCacheStoreWithConfig 创建 RedisCacheStore（自定义配置）
func NewRedisCacheStoreWithConfig(rdb goredis.UniversalClient, config *RedisCacheStoreConfig) *RedisCacheStore {
	config.setDefaultConfig() // 合并默认值

	store := &RedisCacheStore{
		rdb:    rdb,
		config: *config,
		closed: make(chan struct{}),
	}

	// 创建重试删除模块（传入 CacheDeleter 接口和重试配置）
	store.retryDelete = worker.NewRetryDeleteWorker(store, &store.config.RetryDeleteConfig)

	// 创建延迟删除模块（传入 CacheDeleter、RetryEnqueuer 接口和延迟配置）
	store.delayDelete = worker.NewDelayDeleteWorker(store, store.retryDelete, &store.config.DelayDeleteConfig)

	zap.L().Info("redis cache store initialized",
		zap.Int("delay_delete_chan_size", store.config.DelayDeleteConfig.ChanSize),
		zap.Int("retry_delete_chan_size", store.config.RetryDeleteConfig.ChanSize),
		zap.Int("delay_worker_count", store.config.DelayDeleteConfig.WorkerCount),
		zap.Int("retry_worker_count", store.config.RetryDeleteConfig.WorkerCount),
		zap.Duration("operation_timeout", store.config.RetryDeleteConfig.OperationTimeout),
		zap.Duration("shutdown_timeout", store.config.ShutdownTimeout),
	)

	return store
}

// Set 同步设置 Redis 中的键值对
func (s *RedisCacheStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := s.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		zap.L().Error("cache set failed",
			zap.String("key", key),
			zap.Duration("ttl", ttl),
			zap.Error(err),
		)
		return err
	}

	zap.L().Debug("cache set success",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// MSet 同步批量设置 Redis 中的键值对。
//   - 注意：data 中的数据都有同样的 ttl。
//   - 注意：ttl <= 0 或 data 为空时直接忽略。
func (s *RedisCacheStore) MSet(ctx context.Context, data map[string]string, ttl time.Duration) error {
	pipe := s.rdb.Pipeline()

	for key, val := range data {
		pipe.Set(ctx, key, val, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		zap.L().Error("cache mset failed",
			zap.Int("key_number", len(data)),
			zap.Duration("ttl", ttl),
			zap.Error(err),
		)
		return err
	}

	zap.L().Debug("cache mset success",
		zap.Int("key_number", len(data)),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// Get 获取 Redis 中存储的对象
func (s *RedisCacheStore) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			zap.L().Debug("cache miss", zap.String("key", key))
			return "", false, nil
		}

		zap.L().Error("cache get failed",
			zap.String("key", key),
			zap.Error(err),
		)
		return "", false, err
	}

	zap.L().Debug("cache hit", zap.String("key", key))

	return val, true, nil
}

// MGet 批量获取 Redis 中存储的对象
//   - 注意返回的都是有效值
func (s *RedisCacheStore) MGet(ctx context.Context, keys ...string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}

	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		zap.L().Error("cache mget failed",
			zap.Int("keys_len", len(keys)),
			zap.Error(err),
		)
		return map[string]string{}, err
	}

	res := make(map[string]string, len(keys))

	for i, val := range vals {
		if val == nil {
			zap.L().Debug("cache miss", zap.String("key", keys[i]))
			continue
		}
		if s, ok := val.(string); ok {
			zap.L().Debug("cache hit", zap.String("key", keys[i]))
			res[keys[i]] = s
		} else {
			zap.L().Warn("cache mget got unexpected value type",
				zap.String("key", keys[i]),
				zap.Int("index", i),
				zap.String("value_type", fmt.Sprintf("%T", val)))

			return map[string]string{}, err
		}
	}

	return res, nil
}

// Del 同步删除 Redis 中存储的键值对
//   - 同步删除，立即返回结果
//   - 失败后会根据配置策略处理（阻塞重试/丢弃/记录日志）
func (s *RedisCacheStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	err := s.rdb.Del(ctx, keys...).Err()
	if err != nil {
		zap.L().Error("cache del failed",
			zap.Strings("keys", keys),
			zap.Error(err),
		)

		// 删除失败，根据策略处理
		s.retryDelete.Enqueue(keys)
		return err
	}

	zap.L().Debug("cache del success",
		zap.Strings("keys", keys),
	)

	return nil
}

// DelWithDelay 删除缓存并在延迟后再删一次（延迟双删）
//   - 第一次同步删除
//   - 延迟后异步再删一次
//   - 适用于更新 DB 后需要保证缓存一致性的场景
func (s *RedisCacheStore) DelWithDelay(ctx context.Context, delay time.Duration, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	zap.L().Info("cache del with delay",
		zap.Strings("keys", keys),
		zap.Duration("delay", delay),
	)

	// 第一次同步删除
	err := s.rdb.Del(ctx, keys...).Err()
	if err != nil {
		zap.L().Error("cache del with delay first attempt failed",
			zap.Strings("keys", keys),
			zap.Error(err),
		)
		// 失败不重试 - 交给延迟删除
	} else {
		zap.L().Debug("cache del with delay first attempt success",
			zap.Strings("keys", keys),
		)
	}

	// 延迟第二次删除
	s.delayDelete.Enqueue(keys, delay)

	return err
}

// Close 优雅关闭
func (s *RedisCacheStore) Close() error {
	zap.L().Info("redis cache store closing")

	// 1. 标记正在关闭，拒绝新任务
	s.stopping.Store(true)

	// 2. 通知所有模块开始关闭流程
	close(s.closed)

	// 3. 使用带超时的等待
	done := make(chan struct{})
	go func() {
		// 关闭延迟删除模块
		if err := s.delayDelete.Close(); err != nil {
			zap.L().Error("failed to close delay delete worker", zap.Error(err))
		}

		// 关闭重试删除模块
		if err := s.retryDelete.Close(); err != nil {
			zap.L().Error("failed to close retry delete worker", zap.Error(err))
		}

		close(done)
	}()

	// 等待关闭完成或超时
	select {
	case <-done:
		zap.L().Info("redis cache store closed successfully")
	case <-time.After(s.config.ShutdownTimeout):
		zap.L().Warn("redis cache store close timeout, forcing shutdown",
			zap.Duration("timeout", s.config.ShutdownTimeout),
		)
	}

	return nil
}

// DeleteKeys 实现 worker.CacheDeleter 接口
//   - 这是底层删除操作，不会触发重试逻辑
func (s *RedisCacheStore) DeleteKeys(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	return s.rdb.Del(ctx, keys...).Err()
}

// IsStopping 实现 worker.CacheDeleter 接口
func (s *RedisCacheStore) IsStopping() bool {
	return s.stopping.Load()
}
