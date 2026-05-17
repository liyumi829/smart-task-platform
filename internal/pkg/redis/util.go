// Package redis 实现工具
package redis

import (
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const redisTxMaxRetry = 3 // 三次重试

// RetryRedisTx 对 Redis WATCH 乐观锁冲突做有限重试
func RetryRedisTx(fn func() error) error {
	var err error
	for i := 0; i < redisTxMaxRetry; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !errors.Is(err, goredis.TxFailedErr) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}
