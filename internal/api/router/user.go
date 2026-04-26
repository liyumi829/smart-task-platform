// Package router 负责自动注册 API 路由，减少手动维护路由的工作量。
// user 模块的路由注册函数会被 main.go 调用，确保所有认证相关的 API 都正确注册到 Gin 路由中。
package router

import (
	"github.com/gin-gonic/gin"

	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"
)

// RegisterUserRoutes 注册用户模块路由
func RegisterUserRoutes(
	api *gin.RouterGroup,
	userHandler *handler.UserHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 路由分组：/users
	userGroup := api.Group("/users")
	{
		// 公开接口 无需登录
		// 获取任意用户公开信息
		userGroup.GET("/:id", userHandler.GetUserPublicInfo)

		// 获取用户公开列表
		userGroup.GET("", userHandler.ListUsers)

		// 私有接口
		privateUserGroup := userGroup.Group("/me")
		privateUserGroup.Use(middleware.JWTAuth(jwtManager, authStore))
		{
			// 修改自己的资料
			privateUserGroup.PUT("", userHandler.UpdateUserProfile)

			// 修改的密码
			privateUserGroup.PATCH("/password", userHandler.UpdateUserPassword)
		}
	}
}
