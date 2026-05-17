package cache

import (
	"context"
	"fmt"
	"smart-task-platform/internal/cache/worker"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// setupTestRedis 创建测试用的 Redis 和 Store
func setupTestRedis(t *testing.T, config *RedisCacheStoreConfig) (*miniredis.Miniredis, *RedisCacheStore) {
	// 创建内存 Redis
	mr := miniredis.RunT(t)

	// 创建 Redis 客户端
	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	// 替换全局 logger 为测试 logger
	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	zap.ReplaceGlobals(logger)

	// 创建 Store
	var store *RedisCacheStore
	if config != nil {
		store = NewRedisCacheStoreWithConfig(rdb, config)
	} else {
		store = NewRedisCacheStore(rdb)
	}

	return mr, store
}

// TestRedisCacheStore_SetAndGet 测试基本的 Set 和 Get
func TestRedisCacheStore_SetAndGet(t *testing.T) {
	mr, store := setupTestRedis(t, nil)
	defer mr.Close()
	defer store.Close()

	ctx := context.Background()

	t.Run("set and get success", func(t *testing.T) {
		err := store.Set(ctx, "test:key1", "value1", 10*time.Minute)
		require.NoError(t, err)

		val, ok, err := store.Get(ctx, "test:key1")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "value1", val)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		val, ok, err := store.Get(ctx, "test:non-existent")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)
	})

	t.Run("set with ttl", func(t *testing.T) {
		err := store.Set(ctx, "test:ttl", "value", 1*time.Second)
		require.NoError(t, err)

		// 立即获取应该成功
		val, ok, err := store.Get(ctx, "test:ttl")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "value", val)

		// 快进时间
		mr.FastForward(2 * time.Second)

		// 过期后应该获取不到
		val, ok, err = store.Get(ctx, "test:ttl")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)
	})
}

// TestRedisCacheStore_Del 测试同步删除
func TestRedisCacheStore_Del(t *testing.T) {
	mr, store := setupTestRedis(t, nil)
	defer mr.Close()
	defer store.Close()

	ctx := context.Background()

	t.Run("del success", func(t *testing.T) {
		// 设置 key
		err := store.Set(ctx, "test:del1", "value1", 10*time.Minute)
		require.NoError(t, err)

		// 删除 key
		err = store.Del(ctx, "test:del1")
		require.NoError(t, err)

		// 验证已删除
		val, ok, err := store.Get(ctx, "test:del1")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)
	})

	t.Run("del multiple keys", func(t *testing.T) {
		// 设置多个 key
		err := store.Set(ctx, "test:del2", "value2", 10*time.Minute)
		require.NoError(t, err)
		err = store.Set(ctx, "test:del3", "value3", 10*time.Minute)
		require.NoError(t, err)

		// 批量删除
		err = store.Del(ctx, "test:del2", "test:del3")
		require.NoError(t, err)

		// 验证已删除
		_, ok, err := store.Get(ctx, "test:del2")
		require.NoError(t, err)
		assert.False(t, ok)

		_, ok, err = store.Get(ctx, "test:del3")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("del non-existent key", func(t *testing.T) {
		// 删除不存在的 key 不应该报错
		err := store.Del(ctx, "test:non-existent")
		require.NoError(t, err)
	})
}

// TestRedisCacheStore_DelWithDelay 测试延迟双删
func TestRedisCacheStore_DelWithDelay(t *testing.T) {
	config := &RedisCacheStoreConfig{
		DelayDeleteConfig: worker.DelayDeleteConfig{
			ChanSize:         10,
			OperationTimeout: 1 * time.Second,
		},
		RetryDeleteConfig: worker.RetryDeleteConfig{
			ChanSize:         10,
			WorkerCount:      1,
			MaxRetryAttempts: 3,
			RetryBaseDelay:   50 * time.Millisecond,
		},
		ShutdownTimeout: 5 * time.Second,
	}

	mr, store := setupTestRedis(t, config)
	defer mr.Close()
	defer store.Close()

	ctx := context.Background()

	t.Run("del with delay success", func(t *testing.T) {
		// 设置 key
		err := store.Set(ctx, "test:delay1", "value1", 10*time.Minute)
		require.NoError(t, err)

		// 延迟双删
		err = store.DelWithDelay(ctx, 200*time.Millisecond, "test:delay1")
		require.NoError(t, err)

		// 第一次删除应该立即生效
		val, ok, err := store.Get(ctx, "test:delay1")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)

		// 重新设置 key（模拟主从延迟导致的脏读）
		err = store.Set(ctx, "test:delay1", "dirty_value", 10*time.Minute)
		require.NoError(t, err)

		// 等待延迟删除执行
		time.Sleep(300 * time.Millisecond)

		// 延迟删除应该已经执行，key 应该被删除
		val, ok, err = store.Get(ctx, "test:delay1")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)
	})

	t.Run("del with delay first attempt failed", func(t *testing.T) {
		// 设置 key
		err := store.Set(ctx, "test:delay2", "value2", 10*time.Minute)
		require.NoError(t, err)

		// 模拟 Redis 故障
		mr.SetError("simulated error")

		// 延迟双删（第一次删除会失败）
		err = store.DelWithDelay(ctx, 200*time.Millisecond, "test:delay2")
		assert.Error(t, err) // 第一次删除失败

		// 恢复 Redis
		mr.SetError("")

		// 等待延迟删除执行
		time.Sleep(300 * time.Millisecond)

		// 延迟删除应该成功，key 应该被删除
		val, ok, err := store.Get(ctx, "test:delay2")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)
	})
}

// TestRedisCacheStore_RetryDelete 测试重试删除
func TestRedisCacheStore_RetryDelete(t *testing.T) {
	config := &RedisCacheStoreConfig{
		DelayDeleteConfig: worker.DelayDeleteConfig{
			ChanSize:         10,
			OperationTimeout: 1 * time.Second,
		},
		RetryDeleteConfig: worker.RetryDeleteConfig{
			ChanSize:          10,
			WorkerCount:       1,
			MaxRetryAttempts:  3,
			RetryBaseDelay:    50 * time.Millisecond,
			QueueFullStrategy: worker.StrategyLog,
		},
		ShutdownTimeout: 5 * time.Second,
	}

	mr, store := setupTestRedis(t, config)
	defer mr.Close()
	defer store.Close()

	ctx := context.Background()

	t.Run("retry delete success", func(t *testing.T) {
		// 设置 key
		err := store.Set(ctx, "test:retry1", "value1", 10*time.Minute)
		require.NoError(t, err)

		// 模拟 Redis 故障
		mr.SetError("simulated error")

		// 删除会失败，进入重试队列
		err = store.Del(ctx, "test:retry1")
		assert.Error(t, err)

		// 恢复 Redis
		mr.SetError("")

		// 等待重试执行（第一次重试：50ms，第二次重试：100ms）
		time.Sleep(300 * time.Millisecond)

		// 重试应该成功，key 应该被删除
		val, ok, err := store.Get(ctx, "test:retry1")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, val)
	})

	t.Run("retry delete exhausted", func(t *testing.T) {
		// 设置 key
		err := store.Set(ctx, "test:retry2", "value2", 10*time.Minute)
		require.NoError(t, err)

		// 模拟 Redis 持续故障
		mr.SetError("persistent error")

		// 删除会失败，进入重试队列
		err = store.Del(ctx, "test:retry2")
		assert.Error(t, err)

		// 等待所有重试执行完毕
		// 第一次：50ms，第二次：100ms，第三次：200ms
		time.Sleep(500 * time.Millisecond)

		// 重试耗尽，key 仍然存在
		mr.SetError("")
		val, ok, err := store.Get(ctx, "test:retry2")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "value2", val)

		// 恢复 Redis
		mr.SetError("")
	})
}

func TestRedisCacheStore_QueueFullStrategy(t *testing.T) {
	const (
		chanSize       = 2
		workerCount    = 1
		maxWaitTime    = 100 * time.Millisecond
		retryBaseDelay = 500 * time.Millisecond
	)

	newStore := func(t *testing.T, strategy worker.RetryQueueFullStrategy) (*miniredis.Miniredis, *RedisCacheStore) {
		t.Helper()

		config := &RedisCacheStoreConfig{
			RetryDeleteConfig: worker.RetryDeleteConfig{
				ChanSize:          chanSize,
				WorkerCount:       workerCount,
				MaxRetryAttempts:  5,
				RetryBaseDelay:    retryBaseDelay,
				QueueFullStrategy: strategy,
				MaxWaitTime:       maxWaitTime,
			},
			ShutdownTimeout: 3 * time.Second,
		}

		mr, store := setupTestRedis(t, config)

		t.Cleanup(func() {
			// 注意：mr.SetError 会影响后续所有 Redis 命令。
			// 所以 cleanup 前必须先恢复 Redis，否则 Close / 后续操作可能受影响。
			mr.SetError("")
			_ = store.Close()
			mr.Close()
		})

		return mr, store
	}

	prepareKeys := func(t *testing.T, ctx context.Context, store *RedisCacheStore, prefix string, n int) []string {
		t.Helper()

		keys := make([]string, 0, n)
		for i := 0; i < n; i++ {
			key := fmt.Sprintf("%s:%d", prefix, i)
			err := store.Set(ctx, key, "value", 10*time.Minute)
			require.NoError(t, err)
			keys = append(keys, key)
		}

		return keys
	}

	exists := func(t *testing.T, ctx context.Context, store *RedisCacheStore, key string) bool {
		t.Helper()

		_, ok, err := store.Get(ctx, key)
		require.NoError(t, err)

		return ok
	}

	waitDeleted := func(t *testing.T, ctx context.Context, store *RedisCacheStore, key string) {
		t.Helper()

		require.Eventually(t, func() bool {
			_, ok, err := store.Get(ctx, key)
			if err != nil {
				return false
			}
			return !ok
		}, 2*time.Second, 20*time.Millisecond, "key %s should be deleted eventually", key)
	}

	occupyWorkerAndFillQueue := func(
		t *testing.T,
		ctx context.Context,
		mr *miniredis.Miniredis,
		store *RedisCacheStore,
		keys []string,
	) {
		t.Helper()

		require.Len(t, keys, workerCount+chanSize+1)

		// 模拟 Redis 故障。
		// 注意：这之后所有 Get / Set / Del 都会报错，直到 mr.SetError("")。
		mr.SetError("simulated error")

		// 第 1 个任务：删除失败后进入重试队列。
		// 预期：worker 会拿走这个任务，并在 Redis 故障下进入重试等待。
		err := store.Del(ctx, keys[0])
		require.Error(t, err)

		// 给 worker 一点时间取走第一个任务并进入 retry sleep。
		// retryBaseDelay 设置得远大于这个 sleep，所以 worker 不会很快回来消费队列。
		time.Sleep(30 * time.Millisecond)

		// 接下来 chanSize 个任务用于填满 channel buffer。
		for i := 1; i <= chanSize; i++ {
			err = store.Del(ctx, keys[i])
			require.Error(t, err)
		}
	}

	t.Run("strategy block should wait max wait time when queue is full", func(t *testing.T) {
		ctx := context.Background()

		mr, store := newStore(t, worker.StrategyBlock)
		keys := prepareKeys(t, ctx, store, "test:block", workerCount+chanSize+1)

		occupyWorkerAndFillQueue(t, ctx, mr, store, keys)

		// 第 4 个任务：此时 worker 被占用，channel buffer 已满。
		// StrategyBlock 应该阻塞等待 MaxWaitTime，然后放弃入队。
		start := time.Now()
		err := store.Del(ctx, keys[workerCount+chanSize])
		duration := time.Since(start)

		require.Error(t, err)

		assert.GreaterOrEqual(t, duration, maxWaitTime)
		assert.Less(t, duration, maxWaitTime+150*time.Millisecond)

		// 恢复 Redis，让已成功入队的任务完成重试删除。
		mr.SetError("")

		// 前 workerCount + chanSize 个任务应该最终被删除。
		for i := 0; i < workerCount+chanSize; i++ {
			waitDeleted(t, ctx, store, keys[i])
		}

		// 最后这个 overflow 任务因为 Block 超时没有入队，所以应该仍然存在。
		assert.True(t, exists(t, ctx, store, keys[workerCount+chanSize]))
	})

	t.Run("strategy drop should return immediately when queue is full", func(t *testing.T) {
		ctx := context.Background()

		mr, store := newStore(t, worker.StrategyDrop)
		keys := prepareKeys(t, ctx, store, "test:drop", workerCount+chanSize+1)

		occupyWorkerAndFillQueue(t, ctx, mr, store, keys)

		// 第 4 个任务：队列已满，StrategyDrop 应该直接丢弃，不阻塞。
		start := time.Now()
		err := store.Del(ctx, keys[workerCount+chanSize])
		duration := time.Since(start)

		require.Error(t, err)
		assert.Less(t, duration, 50*time.Millisecond)

		// 恢复 Redis，让已成功入队的任务完成重试删除。
		mr.SetError("")

		// 前 workerCount + chanSize 个任务应该最终被删除。
		for i := 0; i < workerCount+chanSize; i++ {
			waitDeleted(t, ctx, store, keys[i])
		}

		// overflow 任务被 Drop 掉了，所以仍然存在。
		assert.True(t, exists(t, ctx, store, keys[workerCount+chanSize]))
	})

	t.Run("strategy log should return immediately when queue is full", func(t *testing.T) {
		ctx := context.Background()

		mr, store := newStore(t, worker.StrategyLog)
		keys := prepareKeys(t, ctx, store, "test:log", workerCount+chanSize+1)

		occupyWorkerAndFillQueue(t, ctx, mr, store, keys)

		// 第 4 个任务：队列已满，StrategyLog 应该记录日志后丢弃，不阻塞。
		start := time.Now()
		err := store.Del(ctx, keys[workerCount+chanSize])
		duration := time.Since(start)

		require.Error(t, err)
		assert.Less(t, duration, 50*time.Millisecond)

		// 恢复 Redis，让已成功入队的任务完成重试删除。
		mr.SetError("")

		// 前 workerCount + chanSize 个任务应该最终被删除。
		for i := 0; i < workerCount+chanSize; i++ {
			waitDeleted(t, ctx, store, keys[i])
		}

		// overflow 任务被 Log 策略丢弃，所以仍然存在。
		assert.True(t, exists(t, ctx, store, keys[workerCount+chanSize]))
	})
}

// TestRedisCacheStore_Close 测试优雅关闭
func TestRedisCacheStore_Close(t *testing.T) {
	config := &RedisCacheStoreConfig{
		DelayDeleteConfig: worker.DelayDeleteConfig{
			ChanSize:         10,
			OperationTimeout: 1 * time.Second,
		},
		RetryDeleteConfig: worker.RetryDeleteConfig{
			ChanSize:         10,
			WorkerCount:      2,
			MaxRetryAttempts: 3,
			RetryBaseDelay:   50 * time.Millisecond,
		},
		ShutdownTimeout: 2 * time.Second,
	}

	mr, store := setupTestRedis(t, config)
	defer mr.Close()

	ctx := context.Background()

	t.Run("close with pending tasks", func(t *testing.T) {
		// 设置一些 key
		for i := 0; i < 5; i++ {
			key := "test:close:" + string(rune('a'+i))
			err := store.Set(ctx, key, "value", 10*time.Minute)
			require.NoError(t, err)
		}

		// 延迟删除
		for i := 0; i < 5; i++ {
			key := "test:close:" + string(rune('a'+i))
			err := store.DelWithDelay(ctx, 100*time.Millisecond, key)
			require.NoError(t, err)
		}

		// 立即关闭（drain 会立即执行延迟删除任务）
		start := time.Now()
		err := store.Close()
		duration := time.Since(start)

		require.NoError(t, err)
		// 应该在 ShutdownTimeout 内完成
		assert.Less(t, duration, 3*time.Second)

		// 验证所有 key 都被删除
		for i := 0; i < 5; i++ {
			key := "test:close:" + string(rune('a'+i))
			val, ok, err := store.Get(ctx, key)
			require.NoError(t, err)
			assert.False(t, ok, "key %s should be deleted", key)
			assert.Empty(t, val)
		}
	})
}

// TestRedisCacheStore_ConcurrentOperations 测试并发操作
func TestRedisCacheStore_ConcurrentOperations(t *testing.T) {
	config := &RedisCacheStoreConfig{
		DelayDeleteConfig: worker.DelayDeleteConfig{
			ChanSize:         100,
			OperationTimeout: 1 * time.Second,
		},
		RetryDeleteConfig: worker.RetryDeleteConfig{
			ChanSize:         100,
			WorkerCount:      5,
			MaxRetryAttempts: 3,
			RetryBaseDelay:   50 * time.Millisecond,
		},
		ShutdownTimeout: 5 * time.Second,
	}

	mr, store := setupTestRedis(t, config)
	defer mr.Close()
	defer store.Close()

	ctx := context.Background()

	t.Run("concurrent set and get", func(t *testing.T) {
		const numGoroutines = 10
		const numOperations = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("test:concurrent:%d", id)
					value := fmt.Sprintf("value%d", j)

					// Set
					err := store.Set(ctx, key, value, 10*time.Minute)
					assert.NoError(t, err)

					// Get
					val, ok, err := store.Get(ctx, key)
					assert.NoError(t, err)
					assert.True(t, ok)
					assert.NotEmpty(t, val)
				}
			}(i)
		}

		// 等待所有 goroutine 完成
		wg.Wait()
	})

	t.Run("concurrent del with delay", func(t *testing.T) {
		const numGoroutines = 10
		const numOperations = 50

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("test:concurrent:del:%d:%d", id, j)

					// Set
					err := store.Set(ctx, key, "value", 10*time.Minute)
					assert.NoError(t, err)

					// DelWithDelay
					err = store.DelWithDelay(ctx, 50*time.Millisecond, key)
					assert.NoError(t, err)
				}
			}(i)
		}

		// 等待所有 goroutine 完成
		wg.Wait()

		// 等待延迟删除执行
		time.Sleep(200 * time.Millisecond)

		// 验证所有 key 都被删除
		for i := 0; i < numGoroutines; i++ {
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("test:concurrent:del:%d:%d", i, j)
				val, ok, err := store.Get(ctx, key)
				require.NoError(t, err)
				assert.False(t, ok, "key %s should be deleted", key)
				assert.Empty(t, val)
			}
		}
	})
}
