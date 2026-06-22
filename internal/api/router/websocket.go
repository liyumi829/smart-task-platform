// Package router 负责自动注册 API 路由，减少手动维护路由的工作量。
// 负责 webocket 的路由
package router

import (
	"github.com/gin-gonic/gin"

	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"
)

// RegisterWebsocket 注册协议升级路由
func RegisterWebsocket(
	api *gin.RouterGroup,
	websocketHandler *handler.WebSocketHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 路由分组：/ws
	authGroup := api.Group("/ws")
	{
		// 私有接口（需要登录鉴权）
		privateGroup := authGroup.Group("")
		privateGroup.Use(middleware.JWTAuth(jwtManager, authStore))
		{
			privateGroup.GET("", websocketHandler.Upgrade)
		}
	}
}
