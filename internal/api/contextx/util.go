// Package contextx 提供了用于在 Gin 上下文中存储和获取用户信息的工具函数。
package contextx

import "github.com/gin-gonic/gin"

const (
	CtxUserIDKey   = "user_id"
	CtxUsernameKey = "username"
)

// SetUserClaims 写入用户信息到上下文 利用 JWT 鉴权中间件解析 Token 后调用该函数将用户信息存储到上下文中
// 便于后续处理器和业务逻辑层获取当前登录用户的信息
func SetUserClaims(c *gin.Context, userID uint64, username string) {
	c.Set(CtxUserIDKey, userID)
	c.Set(CtxUsernameKey, username)
}

// GetUserID 获取当前登录用户 ID
func GetUserID(c *gin.Context) uint64 {
	v, ok := c.Get(CtxUserIDKey)
	if !ok {
		return 0
	}

	id, ok := v.(uint64)
	if !ok {
		return 0
	}

	return id
}

// GetUsername 获取当前登录用户名
func GetUsername(c *gin.Context) string {
	v, ok := c.Get(CtxUsernameKey)
	if !ok {
		return ""
	}

	username, ok := v.(string)
	if !ok {
		return ""
	}

	return username
}
