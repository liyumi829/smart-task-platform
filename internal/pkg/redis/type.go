// Package redis 实现对 token、登录状态的管理
package redis

import (
	"errors"
)

// 认证状态仓储层统一错误

var (
	ErrSessionNotFound      = errors.New("session not found")       // 会话不存在
	ErrRefreshTokenNotFound = errors.New("refresh token not found") // 刷新令牌不存在
	ErrTokenBlacklisted     = errors.New("token blacklisted")       // token 已被拉黑
	ErrSessionMismatch      = errors.New("session mismatch")        // 会话不匹配
	ErrRefreshMismatch      = errors.New("refresh token mismatch")  // refresh token 不匹配
	ErrInvalidTokenType     = errors.New("invalid token type")      // token 类型错误
	ErrInvalidArgument      = errors.New("invalid argument")        // 非法的参数
)

// 存储信息

const (
	constUserID     = "user_id"     // user_id
	constUserName   = "username"    // username
	constSessionID  = "session_id"  // session_id
	constRefreshJTI = "refresh_jti" // refresh_jti
	constLoginAt    = "login_at"    // login_at
	constExpireAt   = "expire_at"   // expire_at
	constJTI        = "jti"         // jti
	constIssuedAt   = "issued_at"   // issued_at
)

// AuthSession 表示当前用户在 Redis 中保存的唯一有效会话
type AuthSession struct {
	UserID     uint64 // 用户 ID
	Username   string // 用户名
	SessionID  string // 当前登录会话 ID
	RefreshJTI string // 当前有效 refresh token 的 jti
	LoginAt    int64  // 登录时间戳
	ExpireAt   int64  // 会话逻辑过期时间戳
}

// RefreshTokenState 表示 Redis 中保存的 refresh token 状态
type RefreshTokenState struct {
	UserID    uint64 // 用户 ID
	Username  string // 用户名
	SessionID string // 所属会话 ID
	JTI       string // refresh token 唯一 ID
	IssuedAt  int64  // 签发时间戳
	ExpireAt  int64  // 过期时间戳
}
