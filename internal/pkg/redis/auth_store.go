// Package goredis 实现 auth 模块的数据存储（在 goredis 中）
package redis

import (
	"context"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisAuthStore 是 AuthStore 的 Redis 实现
type RedisAuthStore struct {
	rdb goredis.UniversalClient // Redis 客户端接口
}

// NewRedisAuthStore 创建 RedisAuthStore
func NewRedisAuthStore(rdb goredis.UniversalClient) *RedisAuthStore {
	return &RedisAuthStore{
		rdb: rdb,
	}
}

// 读 - 改 - 删：Watch解决

// ValidateAccessSession 校验 access token 是否属于当前有效会话
// 用于中间件进行鉴权
//  1. 判断 access 是否被拉黑
//  2. 判断 access 中记录的当前会话是否合法（存在 && 同一个）
//
// 注意：
// 这里只是读操作，不需要 WATCH。
// 即使校验过程中 session 被并发替换，也属于登录态实时变化，下一次请求会被拦截。
// 放过这次的请求
func (s *RedisAuthStore) ValidateAccessSession(
	ctx context.Context,
	userID uint64,
	sessionID string,
	accessJTI string,
) error {
	if userID == 0 || sessionID == "" || accessJTI == "" {
		return ErrInvalidArgument
	}

	sessionKey := s.sessionKey(userID)
	accessBlacklistKey := s.accessBlacklistKey(accessJTI)

	pipe := s.rdb.Pipeline()

	blacklistedCmd := pipe.Exists(ctx, accessBlacklistKey)
	sessionCmd := pipe.HGetAll(ctx, sessionKey)

	if _, err := pipe.Exec(ctx); err != nil { // 管道执行事务
		return err
	}

	if blacklistedCmd.Val() > 0 { // 检查是否再黑名单中
		return ErrTokenBlacklisted
	}

	sessionValue := sessionCmd.Val() // 获取值
	if len(sessionValue) == 0 {
		return ErrSessionNotFound
	}

	if sessionValue[constSessionID] != sessionID {
		return ErrSessionMismatch
	}

	return nil
}

// LoginSession 事务替换用户当前会话
// 用于单设备登录：
//  1. 读取当前 session、新 session 覆盖旧
//  2. 删除旧 refresh、保存新 refresh
func (s *RedisAuthStore) LoginSession(
	ctx context.Context,
	session *AuthSession,
	refresh *RefreshTokenState,
	ttl time.Duration,
) error {
	if session == nil || refresh == nil || ttl <= 0 {
		return ErrInvalidArgument
	}
	if session.UserID == 0 || session.SessionID == "" || session.RefreshJTI == "" {
		return ErrInvalidArgument
	}
	if refresh.UserID == 0 || refresh.SessionID == "" || refresh.JTI == "" {
		return ErrInvalidArgument
	}
	if session.UserID != refresh.UserID ||
		session.SessionID != refresh.SessionID ||
		session.RefreshJTI != refresh.JTI {
		return ErrInvalidArgument
	}

	sessionKey := s.sessionKey(session.UserID) // 获取当前会话 key
	newRefreshKey := s.refreshKey(refresh.JTI) // 获取一个新的 refresh token jti

	return s.rdb.Watch(ctx,
		func(tx *goredis.Tx) error { // Watch 监视 sessionKey
			oldSessionValue, err := tx.HGetAll(ctx, sessionKey).Result() // 获取旧的会话属性
			if err != nil {
				return err
			}

			var oldRefreshJTI string
			// 检查 key 查出来是否为空 这里为空不能退出！因为有可能是第一次登录
			if len(oldSessionValue) != 0 && oldSessionValue[constRefreshJTI] != "" {
				oldRefreshJTI = oldSessionValue[constRefreshJTI]
			}

			// 构造新的 SessionValue
			newSessionValues := map[string]interface{}{
				constUserID:     strconv.FormatUint(session.UserID, 10),
				constUserName:   session.Username,
				constSessionID:  session.SessionID,
				constRefreshJTI: session.RefreshJTI,
				constLoginAt:    strconv.FormatInt(session.LoginAt, 10),
				constExpireAt:   strconv.FormatInt(session.ExpireAt, 10),
			}

			// 构造新的 RefreshValue
			newRefreshValues := map[string]interface{}{
				constUserID:    strconv.FormatUint(refresh.UserID, 10),
				constUserName:  refresh.Username,
				constSessionID: refresh.SessionID,
				constJTI:       refresh.JTI,
				constIssuedAt:  strconv.FormatInt(refresh.IssuedAt, 10),
				constExpireAt:  strconv.FormatInt(refresh.ExpireAt, 10),
			}

			_, err = tx.TxPipelined(ctx,
				func(pipe goredis.Pipeliner) error {
					// 删除旧 refresh 状态，防止并发登录残留多个有效 refresh
					if oldRefreshJTI != "" && oldRefreshJTI != refresh.JTI {
						pipe.Del(ctx, s.refreshKey(oldRefreshJTI))
					}

					// 覆盖旧的 session，设置新的会话状态
					pipe.HSet(ctx, sessionKey, newSessionValues)
					pipe.Expire(ctx, sessionKey, ttl)

					// 保存新 refresh 状态
					pipe.HSet(ctx, newRefreshKey, newRefreshValues)
					pipe.Expire(ctx, newRefreshKey, ttl)

					return nil
				})
			return err
		}, sessionKey)
}

// RotateRefreshToken 事务轮转 refresh token
// 用于 refresh 接口：
//  1. 校验旧 refresh 是否仍属于当前 session、删除旧 refresh、保存新 refresh
//  2. 更新 session 中的 refresh_jti
func (s *RedisAuthStore) RotateRefreshToken(
	ctx context.Context,
	userID uint64,
	sessionID string,
	oldRefreshJTI string,
	newRefresh *RefreshTokenState,
	ttl time.Duration,
) error {
	// 参数校验
	if userID == 0 || sessionID == "" || oldRefreshJTI == "" || newRefresh == nil || ttl <= 0 {
		return ErrInvalidArgument
	}
	if newRefresh.UserID == 0 || newRefresh.SessionID == "" || newRefresh.JTI == "" {
		return ErrInvalidArgument
	}
	if userID != newRefresh.UserID || sessionID != newRefresh.SessionID {
		return ErrInvalidArgument
	}

	sessionKey := s.sessionKey(userID)            // 旧的 session key/新的也用这个
	oldRefreshKey := s.refreshKey(oldRefreshJTI)  // 旧的 refresh key
	newRefreshKey := s.refreshKey(newRefresh.JTI) // 新的 refresh key

	return s.rdb.Watch(ctx,
		func(tx *goredis.Tx) error { // 监视 session key 和 refresh key

			// 获取旧的 session 状态
			oldSessionValue, err := tx.HGetAll(ctx, sessionKey).Result()
			if err != nil {
				return err
			}
			// 参数校验、调用函数说明，之前一定应该有 Session 记录
			if len(oldSessionValue) == 0 {
				return ErrSessionNotFound // 返回会话没有找到
			}

			// 进行 oldSessionValue 参数校验
			if oldSessionValue[constUserID] != strconv.FormatUint(userID, 10) { // 检验 userID
				return ErrSessionMismatch
			}
			if oldSessionValue[constSessionID] != sessionID { // 检验会话ID
				return ErrSessionMismatch
			}
			if oldSessionValue[constRefreshJTI] != oldRefreshJTI { // 检验 jti
				return ErrSessionMismatch
			}
			// 获取旧的 refresh 状态
			oldRefreshValue, err := tx.HGetAll(ctx, oldRefreshKey).Result()
			if err != nil {
				return err
			}
			if len(oldRefreshValue) == 0 {
				return ErrRefreshTokenNotFound // 返回 refresh 没有找到的错误
			}

			// 进行 oldRefreshValue 参数校验
			// 校验旧 refresh 是否仍属于当前 session
			if oldRefreshValue[constUserID] != strconv.FormatUint(userID, 10) { // 检验 userID
				return ErrRefreshMismatch
			}
			if oldRefreshValue[constSessionID] != sessionID { // 检验会话ID
				return ErrRefreshMismatch
			}
			if oldRefreshValue[constJTI] != oldRefreshJTI { // 检验 jti
				return ErrRefreshMismatch
			}

			newRefreshValues := map[string]interface{}{ // 新的 refresh value
				constUserID:    strconv.FormatUint(newRefresh.UserID, 10),
				constUserName:  newRefresh.Username,
				constSessionID: newRefresh.SessionID,
				constJTI:       newRefresh.JTI,
				constIssuedAt:  strconv.FormatInt(newRefresh.IssuedAt, 10),
				constExpireAt:  strconv.FormatInt(newRefresh.ExpireAt, 10),
			}

			_, err = tx.TxPipelined(ctx,
				func(pipe goredis.Pipeliner) error {
					// 删除旧 refresh
					pipe.Del(ctx, oldRefreshKey)

					// 保存新 refresh
					pipe.HSet(ctx, newRefreshKey, newRefreshValues) // 设置新的状态
					pipe.Expire(ctx, newRefreshKey, ttl)            // 设置新的过期时间

					// 仅更新 session 中当前 refresh_jti，避免覆盖 login_at 等旧字段
					pipe.HSet(ctx, sessionKey,
						map[string]interface{}{
							constRefreshJTI: newRefresh.JTI,                             // 设置新的 jti
							constExpireAt:   strconv.FormatInt(newRefresh.ExpireAt, 10), // 设置新的过期时间
						})
					pipe.Expire(ctx, sessionKey, ttl) // 新的过期时间

					return nil
				})
			return err
		}, sessionKey, oldRefreshKey)
}

// LogoutSession 原子退出登录
// 用于 logout 接口：
// 1. 拉黑当前 access token
// 2. 当 session_id 匹配时删除 session 与 refresh
func (s *RedisAuthStore) LogoutSession(
	ctx context.Context,
	userID uint64,
	sessionID string,
	accessJTI string,
	accessTTL time.Duration,
) error {
	// 参数校验
	if userID == 0 || sessionID == "" || accessJTI == "" || accessTTL <= 0 {
		return ErrInvalidArgument
	}

	sessionKey := s.sessionKey(userID)                    // 获取 session key
	accessBlacklistKey := s.accessBlacklistKey(accessJTI) // 黑名单key
	return s.rdb.Watch(ctx, func(tx *goredis.Tx) error {  // 监视 session key
		oldSessionValue, err := tx.HGetAll(ctx, sessionKey).Result() // 获取 session value
		if err != nil {
			return err
		}
		var currentSessionID, currentRefreshJTI string
		if len(oldSessionValue) > 0 {
			currentSessionID = oldSessionValue[constSessionID]   // 获取到 session ID
			currentRefreshJTI = oldSessionValue[constRefreshJTI] // 获取到 jti
		}
		// 这里没有进行是否为空的判断：
		// 如果当前的 access token 中的用户 ID 没有找到对应的会话，说明当前 access token 以及出现问题->先拉黑
		// 进行等幂处理，LogoutSession 在 Store 层直接保持幂等，session 不存在也返回 nil。

		_, err = tx.TxPipelined(ctx,
			func(pipe goredis.Pipeliner) error {
				// 先拉黑当前 access token，保证当前 token 立即失效
				pipe.Set(ctx, accessBlacklistKey, "1", accessTTL)

				// 仅删除当前活跃会话对应的 session 和 refresh
				if currentSessionID != "" && currentSessionID == sessionID { // 有效的会话
					pipe.Del(ctx, sessionKey) // 删除 session 状态
					if currentRefreshJTI != "" {
						pipe.Del(ctx, s.refreshKey(currentRefreshJTI)) // 删除 refresh 状态
					}
				}
				return nil
			})
		return err
	}, sessionKey)
}
