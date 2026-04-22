// Package middleware 提供了用于 Gin 框架的中间件函数，主要包括 JWT 鉴权中间件。
package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/pkg/errmsg"
	jwtpkg "smart-task-platform/internal/pkg/jwt"
	"smart-task-platform/internal/pkg/response"
)

// JWTAuth JWT 鉴权中间件
//
// 仅需要判断访问 Token 是否有效，过期和无效的访问 Token 都会被拒绝访问
// Authorization 请求头必须以 "Bearer " 开头，后面跟着访问 Token 字符串 [Bearer <token>]
func JWTAuth(jwtMgr *jwtpkg.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1、从请求头中获取 Authorization
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			zap.L().Warn("missing authorization header",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path))
			response.Fail(c, errmsg.Unauthorized)
			c.Abort()
			return
		}
		// 2、检查 Authorization 是否以 "Bearer " 开头，并提取 Token 字符串
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			zap.L().Warn("invalid authorization header",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path))
			response.FailWithMessage(c, errmsg.Unauthorized, "invalid authorization header")
			c.Abort()
			return
		}
		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			zap.L().Warn("empty token",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path))
			response.FailWithMessage(c, errmsg.Unauthorized, "empty token")
			c.Abort()
			return
		}

		// 3、解析 Token 获取用户信息
		claims, err := jwtMgr.ParseToken(tokenString)
		if err != nil {
			switch {
			// Token 无效
			case errors.Is(err, jwtpkg.InvalidTokenError),
				errors.Is(err, jwtpkg.InvalidSigningMethodError):
				response.Fail(c, errmsg.AccessTokenInvalid)
			// Token 已过期
			case errors.Is(err, jwtpkg.ExpiredTokenError):
				zap.L().Info("access token expired",
					zap.String("token", tokenString))
				response.Fail(c, errmsg.AccessTokenExpired)
			// 其它错误
			default:
				zap.L().Error("failed to parse token",
					zap.String("token", tokenString),
					zap.Error(err))
				response.Fail(c, errmsg.ServerError)
			}
			c.Abort()
			return
		}
		// 4、将用户信息存储到上下文中，供后续处理器使用
		contextx.SetUserClaims(c, claims.UserID, claims.Username) // 存储用户信息到上下文
		c.Next()                                                  // 下一个中间件
	}
}
