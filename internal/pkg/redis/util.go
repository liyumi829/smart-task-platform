// Package redis 实现工具
package redis

import (
	"errors"
	"fmt"
	"smart-task-platform/internal/pkg/utils"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// NewSessionID 生成新的 session_id
func NewSessionID() string {
	return "sess_" + utils.Uuid()
}

// NewAccessJTI 生成 access token jti
func NewAccessJTI() string {
	return "ajti_" + utils.Uuid()
}

// NewRefreshJTI 生成 refresh token jti
func NewRefreshJTI() string {
	return "rjti_" + utils.Uuid()
}

// sessionKey 用户当前唯一有效会话 Key
//
// 用于会话状态管理的字段
func (s *RedisAuthStore) sessionKey(userID uint64) string {
	return fmt.Sprintf("auth:session:user:%d", userID)
}

// refreshKey refresh token 状态 Key
//
// 管理刷新令牌
func (s *RedisAuthStore) refreshKey(jti string) string {
	return fmt.Sprintf("auth:refresh:jti:%s", jti)
}

// accessBlacklistKey access token 黑名单 Key
//
// 访问令牌的黑名单，避免退出登录/顶号之后的 token 还能被使用
func (s *RedisAuthStore) accessBlacklistKey(jti string) string {
	return fmt.Sprintf("auth:blacklist:access:%s", jti)
}

// parseUint64 安全解析 uint64
func parseUint64(v string) uint64 {
	n, _ := strconv.ParseUint(v, 10, 64)
	return n
}

// parseInt64 安全解析 int64
func parseInt64(v string) int64 {
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

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
