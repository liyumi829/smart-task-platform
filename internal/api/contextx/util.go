// Package contextx 提供了用于在 Gin 上下文中存储和获取用户信息的工具函数。
package contextx

import (
	jwtpkg "smart-task-platform/internal/pkg/jwt"

	"github.com/gin-gonic/gin"
)

const (
	CtxUserIDKey    = "user_id"
	CtxUsernameKey  = "username"
	CtxSessionIDKey = "session_id"
	CtxClaimsKey    = "claims"
)

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

// GetSesssionID 获取当前会话ID
func GetSessionID(c *gin.Context) string {
	v, ok := c.Get(CtxSessionIDKey)
	if !ok {
		return ""
	}
	sessionID, ok := v.(string)
	if !ok {
		return ""
	}
	return sessionID
}

// GetClaims 获取自定义 JWT Claims
func GetClaims(c *gin.Context) *jwtpkg.Claims {
	v, ok := c.Get(CtxClaimsKey)
	if !ok {
		return nil
	}
	claims, ok := v.(*jwtpkg.Claims)
	if !ok {
		return nil
	}
	return claims
}
