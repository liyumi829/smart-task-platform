// Package middleware 提供了用于 Gin 框架的中间件函数，主要包括 JWT 鉴权中间件。
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/pkg/errmsg"
	jwtpkg "smart-task-platform/internal/pkg/jwt"
	redispkg "smart-task-platform/internal/pkg/redis"
	"smart-task-platform/internal/pkg/response"
)

// JWTAuth JWT 鉴权中间件
//
// 仅需要判断访问 Token 是否有效，过期和无效的访问 Token 都会被拒绝访问
// Authorization 请求头必须以 "Bearer " 开头，后面跟着访问 Token 字符串 [Bearer <token>]
func JWTAuth(jwtMgr *jwtpkg.Manager, authStore AuthStoreInvoker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1、从请求头中获取 Authorization
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			zap.L().Warn("missing authorization header",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path))
			response.Abort(c, http.StatusUnauthorized, errmsg.Unauthorized)
			return
		}
		// 2、检查 Authorization 是否以 "Bearer " 开头，并提取 Token 字符串
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			zap.L().Warn("invalid authorization header",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path))
			response.AbortWithMessage(c, http.StatusUnauthorized, errmsg.Unauthorized, "invalid authorization header")
			return
		}
		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			zap.L().Warn("empty token",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path))
			response.AbortWithMessage(c, http.StatusUnauthorized, errmsg.Unauthorized, "empty token")
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

		// 4、获取到用户信息之后，对 access token 进行检查
		// 是否是 “access类型”、是否黑名单、是否是当前的session会话
		if claims.TokenType != "access" {
			response.AbortWithMessage(c, http.StatusUnauthorized, errmsg.Unauthorized, "invalid token type")
		}

		if err := redispkg.RetryRedisTx( // 重试机制
			func() error {
				return authStore.ValidateAccessSession(
					c.Request.Context(),
					claims.UserID,
					claims.SessionID,
					claims.ID)
			}); err != nil {
			switch {
			// 在黑名单中
			case errors.Is(err, redispkg.ErrTokenBlacklisted):
				response.AbortWithMessage(c, http.StatusUnauthorized, errmsg.Unauthorized, "token blacklisted")
				return
			// 会话不匹配/会话没有找到
			case errors.Is(err, redispkg.ErrSessionMismatch),
				errors.Is(err, redispkg.ErrSessionNotFound):
				// 这里就是“旧设备识别自己已被踢下线”的关键点
				response.AbortWithMessage(c, http.StatusUnauthorized, errmsg.Unauthorized, "logged in on another device or session invalid")
				return
			// 其它错误
			default:
				response.AbortWithMessage(c, http.StatusUnauthorized, errmsg.Unauthorized, "failed to validate session")
				return
			}
		}

		// 5、将用户信息存储到上下文中，供后续处理器使用
		c.Set(contextx.CtxUserIDKey, claims.UserID)
		c.Set(contextx.CtxUsernameKey, claims.Username)
		c.Set(contextx.CtxSessionIDKey, claims.SessionID)
		c.Set(contextx.CtxClaimsKey, claims)
		c.Next() // 下一个中间件
	}
}

// AuthStoreInvoker 接口，校验当前 access token 是否有效
type AuthStoreInvoker interface {
	ValidateAccessSession(ctx context.Context, userID uint64, sessionID string, accessJTI string) error
}
