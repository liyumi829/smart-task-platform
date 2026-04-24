// 文件路径：internal/pkg/redis/auth_store_test.go

package redis

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockRedisAuthStore 创建基于 miniredis 的 RedisAuthStore。
// miniredis 是内存 Redis mock，不依赖外部 Redis 服务。
func newMockRedisAuthStore(t *testing.T) (*RedisAuthStore, *miniredis.Miniredis, context.Context) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	t.Cleanup(func() {
		mr.Close()
	})

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	return NewRedisAuthStore(rdb), mr, context.Background()
}

// mustSeedSession 写入测试 session 数据。
func mustSeedSession(
	t *testing.T,
	store *RedisAuthStore,
	ctx context.Context,
	session *AuthSession,
	ttl time.Duration,
) {
	t.Helper()

	key := store.sessionKey(session.UserID)

	err := store.rdb.HSet(ctx, key, map[string]interface{}{
		constUserID:     strconv.FormatUint(session.UserID, 10),
		constUserName:   session.Username,
		constSessionID:  session.SessionID,
		constRefreshJTI: session.RefreshJTI,
		constLoginAt:    strconv.FormatInt(session.LoginAt, 10),
		constExpireAt:   strconv.FormatInt(session.ExpireAt, 10),
	}).Err()
	require.NoError(t, err)

	err = store.rdb.Expire(ctx, key, ttl).Err()
	require.NoError(t, err)

	t.Logf("[seed session] key=%s session_id=%s refresh_jti=%s ttl=%s",
		key,
		session.SessionID,
		session.RefreshJTI,
		ttl,
	)
}

// mustSeedRefresh 写入测试 refresh 数据。
func mustSeedRefresh(
	t *testing.T,
	store *RedisAuthStore,
	ctx context.Context,
	refresh *RefreshTokenState,
	ttl time.Duration,
) {
	t.Helper()

	key := store.refreshKey(refresh.JTI)

	err := store.rdb.HSet(ctx, key, map[string]interface{}{
		constUserID:    strconv.FormatUint(refresh.UserID, 10),
		constUserName:  refresh.Username,
		constSessionID: refresh.SessionID,
		constJTI:       refresh.JTI,
		constIssuedAt:  strconv.FormatInt(refresh.IssuedAt, 10),
		constExpireAt:  strconv.FormatInt(refresh.ExpireAt, 10),
	}).Err()
	require.NoError(t, err)

	err = store.rdb.Expire(ctx, key, ttl).Err()
	require.NoError(t, err)

	t.Logf("[seed refresh] key=%s session_id=%s jti=%s ttl=%s",
		key,
		refresh.SessionID,
		refresh.JTI,
		ttl,
	)
}

// mustExists 断言 Redis key 存在。
func mustExists(t *testing.T, store *RedisAuthStore, ctx context.Context, key string) {
	t.Helper()

	n, err := store.rdb.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "key should exist: %s", key)
	t.Logf("[assert exists] key=%s", key)
}

// mustNotExists 断言 Redis key 不存在。
func mustNotExists(t *testing.T, store *RedisAuthStore, ctx context.Context, key string) {
	t.Helper()

	n, err := store.rdb.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "key should not exist: %s", key)
	t.Logf("[assert not exists] key=%s", key)
}

// mustTTLPositive 断言 Redis key 的 TTL 大于 0。
func mustTTLPositive(t *testing.T, store *RedisAuthStore, ctx context.Context, key string) {
	t.Helper()

	ttl, err := store.rdb.TTL(ctx, key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "ttl should be positive: %s", key)
	t.Logf("[assert ttl] key=%s ttl=%s", key, ttl)
}

func TestRedisAuthStore_ValidateAccessSession(t *testing.T) {
	t.Run("参数非法", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.ValidateAccessSession(ctx, 0, "", "")
		t.Logf("[ValidateAccessSession invalid args] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("access token 已被拉黑", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		accessJTI := "ajti_blacklisted"

		err := store.rdb.Set(ctx, store.accessBlacklistKey(accessJTI), "1", time.Minute).Err()
		require.NoError(t, err)

		err = store.ValidateAccessSession(ctx, 1001, "sess_001", accessJTI)
		t.Logf("[ValidateAccessSession blacklisted] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenBlacklisted)
	})

	t.Run("session 不存在", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.ValidateAccessSession(ctx, 1001, "sess_001", "ajti_001")
		t.Logf("[ValidateAccessSession session not found] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("session_id 不匹配", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_real",
			RefreshJTI: "rjti_001",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		err := store.ValidateAccessSession(ctx, 1001, "sess_fake", "ajti_001")
		t.Logf("[ValidateAccessSession session mismatch] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionMismatch)
	})

	t.Run("校验成功", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_001",
			RefreshJTI: "rjti_001",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		err := store.ValidateAccessSession(ctx, 1001, "sess_001", "ajti_001")
		t.Logf("[ValidateAccessSession success] err=%v", err)

		require.NoError(t, err)
	})
}

func TestRedisAuthStore_LoginSession(t *testing.T) {
	t.Run("参数非法_nil", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.LoginSession(ctx, nil, nil, time.Minute)
		t.Logf("[LoginSession invalid nil] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("参数非法_session_refresh 不一致", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.LoginSession(ctx,
			&AuthSession{
				UserID:     1001,
				Username:   "zhangsan",
				SessionID:  "sess_001",
				RefreshJTI: "rjti_session", // 不同
				LoginAt:    1000,
				ExpireAt:   2000,
			},
			&RefreshTokenState{
				UserID:    1001,
				Username:  "zhangsan",
				SessionID: "sess_001",
				JTI:       "rjti_refresh",
				IssuedAt:  1000,
				ExpireAt:  2000,
			},
			time.Minute,
		)
		t.Logf("[LoginSession invalid mismatch] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("首次登录成功_写入 session 和 refresh", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		session := &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_001",
			RefreshJTI: "rjti_001",
			LoginAt:    1000,
			ExpireAt:   2000,
		}
		refresh := &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_001",
			JTI:       "rjti_001",
			IssuedAt:  1000,
			ExpireAt:  2000,
		}

		err := RetryRedisTx(func() error {
			return store.LoginSession(ctx, session, refresh, time.Minute)
		})
		t.Logf("[LoginSession first login] err=%v", err)

		require.NoError(t, err)

		sessionKey := store.sessionKey(1001)
		refreshKey := store.refreshKey("rjti_001")

		mustExists(t, store, ctx, sessionKey)
		mustExists(t, store, ctx, refreshKey)
		mustTTLPositive(t, store, ctx, sessionKey)
		mustTTLPositive(t, store, ctx, refreshKey)

		sessionValue, err := store.rdb.HGetAll(ctx, sessionKey).Result()
		require.NoError(t, err)

		assert.Equal(t, "1001", sessionValue[constUserID])
		assert.Equal(t, "zhangsan", sessionValue[constUserName])
		assert.Equal(t, "sess_001", sessionValue[constSessionID])
		assert.Equal(t, "rjti_001", sessionValue[constRefreshJTI])
		assert.Equal(t, "1000", sessionValue[constLoginAt])
		assert.Equal(t, "2000", sessionValue[constExpireAt])

		refreshValue, err := store.rdb.HGetAll(ctx, refreshKey).Result()
		require.NoError(t, err)

		assert.Equal(t, "1001", refreshValue[constUserID])
		assert.Equal(t, "zhangsan", refreshValue[constUserName])
		assert.Equal(t, "sess_001", refreshValue[constSessionID])
		assert.Equal(t, "rjti_001", refreshValue[constJTI])
		assert.Equal(t, "1000", refreshValue[constIssuedAt])
		assert.Equal(t, "2000", refreshValue[constExpireAt])

		t.Logf("[LoginSession first login result] session=%v refresh=%v", sessionValue, refreshValue)
	})

	t.Run("再次登录成功_覆盖 session 并删除旧 refresh", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_old",
			RefreshJTI: "rjti_old",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		mustSeedRefresh(t, store, ctx, &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_old",
			JTI:       "rjti_old",
			IssuedAt:  1000,
			ExpireAt:  2000,
		}, time.Minute)

		session := &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_new",
			RefreshJTI: "rjti_new",
			LoginAt:    3000,
			ExpireAt:   4000,
		}
		refresh := &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_new",
			JTI:       "rjti_new",
			IssuedAt:  3000,
			ExpireAt:  4000,
		}

		sessionKey := store.sessionKey(1001)
		oldRefreshKey := store.refreshKey("rjti_old")
		newRefreshKey := store.refreshKey("rjti_new")

		refreshValue, err := store.rdb.HGetAll(ctx, oldRefreshKey).Result()
		require.NoError(t, err)

		// 先检查
		assert.Equal(t, "1001", refreshValue[constUserID])
		assert.Equal(t, "zhangsan", refreshValue[constUserName])
		assert.Equal(t, "sess_old", refreshValue[constSessionID])
		assert.Equal(t, "rjti_old", refreshValue[constJTI])
		assert.Equal(t, "1000", refreshValue[constIssuedAt])
		assert.Equal(t, "2000", refreshValue[constExpireAt])

		err = RetryRedisTx(func() error {
			return store.LoginSession(ctx, session, refresh, time.Minute)
		})
		t.Logf("[LoginSession relogin] err=%v", err)

		require.NoError(t, err)

		mustExists(t, store, ctx, sessionKey)
		mustNotExists(t, store, ctx, oldRefreshKey)
		mustExists(t, store, ctx, newRefreshKey)

		sessionValue, err := store.rdb.HGetAll(ctx, sessionKey).Result()
		require.NoError(t, err)

		assert.Equal(t, "1001", sessionValue[constUserID])
		assert.Equal(t, "zhangsan", sessionValue[constUserName])
		assert.Equal(t, "sess_new", sessionValue[constSessionID])
		assert.Equal(t, "rjti_new", sessionValue[constRefreshJTI])
		assert.Equal(t, "3000", sessionValue[constLoginAt])
		assert.Equal(t, "4000", sessionValue[constExpireAt])

		refreshValue, err = store.rdb.HGetAll(ctx, newRefreshKey).Result()
		require.NoError(t, err)

		assert.Equal(t, "1001", refreshValue[constUserID])
		assert.Equal(t, "zhangsan", refreshValue[constUserName])
		assert.Equal(t, "sess_new", refreshValue[constSessionID])
		assert.Equal(t, "rjti_new", refreshValue[constJTI])
		assert.Equal(t, "3000", refreshValue[constIssuedAt])
		assert.Equal(t, "4000", refreshValue[constExpireAt])

		t.Logf("[LoginSession relogin result] session=%v", sessionValue)
	})
}

func TestRedisAuthStore_RotateRefreshToken(t *testing.T) {
	t.Run("参数非法", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.RotateRefreshToken(ctx, 0, "", "", nil, time.Minute)
		t.Logf("[RotateRefreshToken invalid args] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("session 不存在", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.RotateRefreshToken(ctx, 1001, "sess_001", "rjti_old", &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_001",
			JTI:       "rjti_new",
			IssuedAt:  3000,
			ExpireAt:  4000,
		}, time.Minute)
		t.Logf("[RotateRefreshToken session not found] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("旧 refresh 不存在", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_001",
			RefreshJTI: "rjti_old",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		err := store.RotateRefreshToken(ctx, 1001, "sess_001", "rjti_old", &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_001",
			JTI:       "rjti_new",
			IssuedAt:  3000,
			ExpireAt:  4000,
		}, time.Minute)
		t.Logf("[RotateRefreshToken old refresh not found] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRefreshTokenNotFound)
	})

	t.Run("session 不匹配", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001, // 1 （检验不了，构造的）
			Username:   "zhangsan",
			SessionID:  "sess_real", // * 2
			RefreshJTI: "rjti_old",  // 3
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		mustSeedRefresh(t, store, ctx, &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_real",
			JTI:       "rjti_old",
			IssuedAt:  1000,
			ExpireAt:  2000,
		}, time.Minute)

		err := store.RotateRefreshToken(ctx, 1001, "sess_fake", "rjti_old", &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_fake",
			JTI:       "rjti_new",
			IssuedAt:  3000,
			ExpireAt:  4000,
		}, time.Minute)
		t.Logf("[RotateRefreshToken session mismatch] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionMismatch)
	})

	t.Run("refresh 不匹配", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_001",
			RefreshJTI: "rjti_old",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		// 故意写入 session_id 不一致的 refresh
		mustSeedRefresh(t, store, ctx, &RefreshTokenState{
			UserID:    1001, // 1
			Username:  "zhangsan",
			SessionID: "sess_other", // * 2
			JTI:       "rjti_old",   // 3（这里检验不了，key就是用这里的参数构造的）
			IssuedAt:  1000,
			ExpireAt:  2000,
		}, time.Minute)

		err := store.RotateRefreshToken(ctx, 1001, "sess_001", "rjti_old", &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_001",
			JTI:       "rjti_new",
			IssuedAt:  3000,
			ExpireAt:  4000,
		}, time.Minute)
		t.Logf("[RotateRefreshToken refresh mismatch] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRefreshMismatch)
	})

	t.Run("轮转成功_删除旧 refresh_保存新 refresh_更新 session", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_001",
			RefreshJTI: "rjti_old",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		mustSeedRefresh(t, store, ctx, &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_001",
			JTI:       "rjti_old",
			IssuedAt:  1000,
			ExpireAt:  2000,
		}, time.Minute)

		err := RetryRedisTx(func() error {
			return store.RotateRefreshToken(ctx, 1001, "sess_001", "rjti_old", &RefreshTokenState{
				UserID:    1001,
				Username:  "zhangsan",
				SessionID: "sess_001",
				JTI:       "rjti_new",
				IssuedAt:  3000,
				ExpireAt:  4000,
			}, time.Minute)
		})
		t.Logf("[RotateRefreshToken success] err=%v", err)

		require.NoError(t, err)

		sessionKey := store.sessionKey(1001)
		oldRefreshKey := store.refreshKey("rjti_old")
		newRefreshKey := store.refreshKey("rjti_new")

		mustExists(t, store, ctx, sessionKey)
		mustNotExists(t, store, ctx, oldRefreshKey)
		mustExists(t, store, ctx, newRefreshKey)
		mustTTLPositive(t, store, ctx, sessionKey)
		mustTTLPositive(t, store, ctx, newRefreshKey)

		sessionValue, err := store.rdb.HGetAll(ctx, sessionKey).Result()
		require.NoError(t, err)

		assert.Equal(t, "rjti_new", sessionValue[constRefreshJTI])
		assert.Equal(t, "1000", sessionValue[constLoginAt], "login_at should not be overwritten")
		assert.Equal(t, "4000", sessionValue[constExpireAt])

		newRefreshValue, err := store.rdb.HGetAll(ctx, newRefreshKey).Result()
		require.NoError(t, err)

		assert.Equal(t, "1001", newRefreshValue[constUserID])
		assert.Equal(t, "zhangsan", newRefreshValue[constUserName])
		assert.Equal(t, "sess_001", newRefreshValue[constSessionID])
		assert.Equal(t, "rjti_new", newRefreshValue[constJTI])
		assert.Equal(t, "3000", newRefreshValue[constIssuedAt])
		assert.Equal(t, "4000", newRefreshValue[constExpireAt])

		t.Logf("[RotateRefreshToken success result] session=%v new_refresh=%v",
			sessionValue,
			newRefreshValue,
		)
	})
}

func TestRedisAuthStore_LogoutSession(t *testing.T) {
	t.Run("参数非法", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := store.LogoutSession(ctx, 0, "", "", time.Minute)
		t.Logf("[LogoutSession invalid args] err=%v", err)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidArgument)
	})

	t.Run("session 存在且匹配_删除 session 和 refresh_拉黑 access", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1001,
			Username:   "zhangsan",
			SessionID:  "sess_001",
			RefreshJTI: "rjti_001",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		mustSeedRefresh(t, store, ctx, &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_001",
			JTI:       "rjti_001",
			IssuedAt:  1000,
			ExpireAt:  2000,
		}, time.Minute)

		err := RetryRedisTx(func() error {
			return store.LogoutSession(ctx, 1001, "sess_001", "ajti_001", time.Minute)
		})
		t.Logf("[LogoutSession matched] err=%v", err)

		require.NoError(t, err)

		mustNotExists(t, store, ctx, store.sessionKey(1001))
		mustNotExists(t, store, ctx, store.refreshKey("rjti_001"))
		mustExists(t, store, ctx, store.accessBlacklistKey("ajti_001"))
		mustTTLPositive(t, store, ctx, store.accessBlacklistKey("ajti_001"))
	})

	t.Run("session 存在但不匹配_只拉黑 access_不删除 session 和 refresh", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		mustSeedSession(t, store, ctx, &AuthSession{
			UserID:     1000,
			Username:   "zhangsan",
			SessionID:  "sess_real",
			RefreshJTI: "rjti_001",
			LoginAt:    1000,
			ExpireAt:   2000,
		}, time.Minute)

		mustSeedRefresh(t, store, ctx, &RefreshTokenState{
			UserID:    1001,
			Username:  "zhangsan",
			SessionID: "sess_real",
			JTI:       "rjti_001",
			IssuedAt:  1000,
			ExpireAt:  2000,
		}, time.Minute)

		err := RetryRedisTx(func() error {
			return store.LogoutSession(ctx, 1001, "sess_fake", "ajti_002", time.Minute)
		})
		t.Logf("[LogoutSession mismatch] err=%v", err)

		require.NoError(t, err)

		mustExists(t, store, ctx, store.sessionKey(1001))
		mustExists(t, store, ctx, store.refreshKey("rjti_001"))
		mustExists(t, store, ctx, store.accessBlacklistKey("ajti_002"))
	})

	t.Run("session 不存在_仍然拉黑 access", func(t *testing.T) {
		store, _, ctx := newMockRedisAuthStore(t)

		err := RetryRedisTx(func() error {
			return store.LogoutSession(ctx, 1001, "sess_missing", "ajti_003", time.Minute)
		})
		t.Logf("[LogoutSession no session] err=%v", err)

		require.NoError(t, err)

		mustNotExists(t, store, ctx, store.sessionKey(1001))
		mustExists(t, store, ctx, store.accessBlacklistKey("ajti_003"))
	})
}

func TestRedisAuthStore_RetryRedisTx(t *testing.T) {
	t.Run("TxFailedErr 会重试直到成功", func(t *testing.T) {
		count := 0

		err := RetryRedisTx(func() error {
			count++
			t.Logf("[RetryRedisTx retry case] attempt=%d", count)

			if count < 3 {
				return goredis.TxFailedErr
			}
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("非 TxFailedErr 不重试", func(t *testing.T) {
		count := 0
		customErr := errors.New("custom error")

		err := RetryRedisTx(func() error {
			count++
			t.Logf("[RetryRedisTx no retry case] attempt=%d", count)
			return customErr
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, customErr)
		assert.Equal(t, 1, count)
	})
}

// TestRedisAuthStore_ConcurrentLoginSession 测试同一用户并发登录时的最终一致性。
//
// 测试目标：
//  1. 并发登录时允许出现 redis.TxFailedErr，这是 WATCH 乐观锁冲突的正常结果。
//  2. 不要求所有 goroutine 都成功。
//  3. 至少应该有一个 goroutine 成功。
//  4. 最终 Redis 中只能保留一个当前有效 session。
//  5. 最终 Redis 中只能保留一个与当前 session.refresh_jti 对应的 refresh。
//  6. 失败事务写入的 refresh 不应该残留。
//  7. 当前 session 和当前 refresh 的 user_id、session_id、refresh_jti 必须一致。
func TestRedisAuthStore_ConcurrentLoginSession(t *testing.T) {
	store, _, ctx := newMockRedisAuthStore(t)

	const userID uint64 = 1001
	const goroutineCount = 10

	type loginResult struct {
		index      int
		sessionID  string
		refreshJTI string
		err        error
	}

	var wg sync.WaitGroup
	resultCh := make(chan loginResult, goroutineCount)

	// 并发模拟同一用户多处登录。
	for i := 0; i < goroutineCount; i++ {
		i := i

		wg.Add(1)
		go func() {
			defer wg.Done()

			sessionID := "sess_concurrent_" + strconv.Itoa(i)
			refreshJTI := "rjti_concurrent_" + strconv.Itoa(i)

			session := &AuthSession{
				UserID:     userID,
				Username:   "zhangsan",
				SessionID:  sessionID,
				RefreshJTI: refreshJTI,
				LoginAt:    int64(1000 + i),
				ExpireAt:   int64(2000 + i),
			}

			refresh := &RefreshTokenState{
				UserID:    userID,
				Username:  "zhangsan",
				SessionID: sessionID,
				JTI:       refreshJTI,
				IssuedAt:  int64(1000 + i),
				ExpireAt:  int64(2000 + i),
			}

			// 注意：
			// 这里故意不使用 RetryRedisTx。
			// 该测试用于验证 WATCH 乐观锁在并发下会阻止部分冲突事务提交。
			err := store.LoginSession(ctx, session, refresh, time.Minute)

			resultCh <- loginResult{
				index:      i,
				sessionID:  sessionID,
				refreshJTI: refreshJTI,
				err:        err,
			}

			t.Logf(
				"[ConcurrentLoginSession] index=%d session_id=%s refresh_jti=%s err=%v",
				i,
				sessionID,
				refreshJTI,
				err,
			)
		}()
	}

	wg.Wait()
	close(resultCh)

	successCount := 0
	txFailedCount := 0

	successRefreshJTISet := make(map[string]struct{})
	successSessionIDSet := make(map[string]struct{})

	// 并发场景下：
	//  1. nil 表示事务成功提交。
	//  2. goredis.TxFailedErr 表示 WATCH 冲突，是允许出现的。
	//  3. 其他错误不允许出现。
	for result := range resultCh {
		switch {
		case result.err == nil:
			successCount++
			successRefreshJTISet[result.refreshJTI] = struct{}{}
			successSessionIDSet[result.sessionID] = struct{}{}

		case errors.Is(result.err, goredis.TxFailedErr):
			txFailedCount++

		default:
			require.NoErrorf(
				t,
				result.err,
				"unexpected error, index=%d session_id=%s refresh_jti=%s",
				result.index,
				result.sessionID,
				result.refreshJTI,
			)
		}
	}

	t.Logf(
		"[ConcurrentLoginSession summary] success_count=%d tx_failed_count=%d total=%d",
		successCount,
		txFailedCount,
		goroutineCount,
	)

	// 至少应该有一个登录事务成功，否则当前用户没有任何有效会话，不符合登录业务。
	require.GreaterOrEqual(t, successCount, 1)

	// 成功数量 + WATCH 冲突数量应等于总并发数量。
	assert.Equal(t, goroutineCount, successCount+txFailedCount)

	sessionKey := store.sessionKey(userID)

	// 最终必须存在当前用户 session。
	mustExists(t, store, ctx, sessionKey)
	mustTTLPositive(t, store, ctx, sessionKey)

	sessionValue, err := store.rdb.HGetAll(ctx, sessionKey).Result()
	require.NoError(t, err)
	require.NotEmpty(t, sessionValue)

	t.Logf("[ConcurrentLoginSession final session] key=%s value=%v", sessionKey, sessionValue)

	currentUserID := sessionValue[constUserID]
	currentUsername := sessionValue[constUserName]
	currentSessionID := sessionValue[constSessionID]
	currentRefreshJTI := sessionValue[constRefreshJTI]
	currentLoginAt := sessionValue[constLoginAt]
	currentExpireAt := sessionValue[constExpireAt]

	// 校验最终 session 字段完整性。
	assert.Equal(t, strconv.FormatUint(userID, 10), currentUserID)
	assert.Equal(t, "zhangsan", currentUsername)
	assert.NotEmpty(t, currentSessionID)
	assert.NotEmpty(t, currentRefreshJTI)
	assert.NotEmpty(t, currentLoginAt)
	assert.NotEmpty(t, currentExpireAt)

	// 最终 session 必须来自某一次成功提交的登录。
	_, ok := successSessionIDSet[currentSessionID]
	assert.Truef(t, ok, "final session_id should belong to successful login, session_id=%s", currentSessionID)

	_, ok = successRefreshJTISet[currentRefreshJTI]
	assert.Truef(t, ok, "final refresh_jti should belong to successful login, refresh_jti=%s", currentRefreshJTI)

	currentRefreshKey := store.refreshKey(currentRefreshJTI)

	// 最终 session 指向的 refresh 必须存在。
	mustExists(t, store, ctx, currentRefreshKey)
	mustTTLPositive(t, store, ctx, currentRefreshKey)

	currentRefreshValue, err := store.rdb.HGetAll(ctx, currentRefreshKey).Result()
	require.NoError(t, err)
	require.NotEmpty(t, currentRefreshValue)

	t.Logf(
		"[ConcurrentLoginSession final refresh] key=%s value=%v",
		currentRefreshKey,
		currentRefreshValue,
	)

	// 校验最终 refresh 与最终 session 一致。
	assert.Equal(t, currentUserID, currentRefreshValue[constUserID])
	assert.Equal(t, currentUsername, currentRefreshValue[constUserName])
	assert.Equal(t, currentSessionID, currentRefreshValue[constSessionID])
	assert.Equal(t, currentRefreshJTI, currentRefreshValue[constJTI])
	assert.Equal(t, currentExpireAt, currentRefreshValue[constExpireAt])

	// 查询所有并发登录产生的 refresh key。
	refreshKeys, err := store.rdb.Keys(ctx, "auth:refresh:jti:rjti_concurrent_*").Result()
	require.NoError(t, err)

	t.Logf("[ConcurrentLoginSession final refresh keys] keys=%v", refreshKeys)

	// 单设备登录最终只能保留一个 refresh。
	require.Len(t, refreshKeys, 1)
	assert.Equal(t, currentRefreshKey, refreshKeys[0])

	// 逐个确认非当前 refresh 不存在，防止失败事务或旧会话 refresh 残留。
	for i := 0; i < goroutineCount; i++ {
		refreshJTI := "rjti_concurrent_" + strconv.Itoa(i)
		refreshKey := store.refreshKey(refreshJTI)

		if refreshKey == currentRefreshKey {
			mustExists(t, store, ctx, refreshKey)
			continue
		}

		mustNotExists(t, store, ctx, refreshKey)
	}
}

// 检查 Redis 最终状态：
// 1、session 必须存在
// 2、session 必须来自某一次成功登录
// 3、session 对应的 refresh 必须存在
// 4、全世界只能有 1 个 refresh token 存活
// 5、旧的、失败的事务产生的 refresh 必须全部被清理
// 6、session ↔ refresh 必须完全匹配
