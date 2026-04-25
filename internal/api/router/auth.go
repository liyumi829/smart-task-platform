// Package router 负责自动注册 API 路由，减少手动维护路由的工作量。
// auth 模块的路由注册函数会被 main.go 调用，确保所有认证相关的 API 都正确注册到 Gin 路由中。
package router

import (
	"github.com/gin-gonic/gin"

	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"
)

// RegisterAuthRoutes 注册认证模块路由
func RegisterAuthRoutes(
	api *gin.RouterGroup,
	authHandler *handler.AuthHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 路由分组：/auth
	authGroup := api.Group("/auth")
	{
		// 公开接口（无需登录）
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.RefreshToken)

		// 私有接口（需要登录鉴权）
		privateGroup := authGroup.Group("/")
		privateGroup.Use(middleware.JWTAuth(jwtManager, authStore))
		{
			privateGroup.GET("/me", authHandler.Me)
			privateGroup.POST("/logout", authHandler.Logout)
		}
	}
}
