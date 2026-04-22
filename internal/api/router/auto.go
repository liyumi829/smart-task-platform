// Package router 负责自动注册 API 路由，减少手动维护路由的工作量。
// auto 模块的路由注册函数会被 main.go 调用，确保所有认证相关的 API 都正确注册到 Gin 路由中。
package router

import (
	"github.com/gin-gonic/gin"

	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
)

// RegisterAuthRoutes 注册认证模块路由
func RegisterAuthRoutes(
	api *gin.RouterGroup,
	authHandler *handler.AuthHandler,
	jwtMgr *authjwt.Manager,
) {
	auth := api.Group("/auth")
	{
		// 注册和登录接口不需要鉴权，任何人都可以访问
		auth.POST("/register", authHandler.Register)    // 注册接口不需要鉴权，任何人都可以访问
		auth.POST("/login", authHandler.Login)          // 登录成功后会返回 Token，后续需要携带 Token 访问以下接口
		auth.POST("/refresh", authHandler.RefreshToken) // 	刷新 Token

		// 登录成功 需要携带 Token 访问以下接口
		auth.GET("/me", middleware.JWTAuth(jwtMgr), authHandler.Me)          // 获取当前用户信息
		auth.POST("/logout", middleware.JWTAuth(jwtMgr), authHandler.Logout) // 退出登录
	}
}
