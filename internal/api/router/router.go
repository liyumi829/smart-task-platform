// Package router 自动注册 API 路由
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"smart-task-platform/internal/api/handler"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"
)

// Register 注册路由
func Register(r *gin.Engine, authHandler *handler.AuthHandler, jwtMgr *authjwt.Manager, autoStore *authredis.RedisAuthStore) {
	r.GET("/ping",
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"code":    0,
				"message": "success",
				"data":    "pong",
			})
		}) // 测试连通性

	api := r.Group("/api/v1")
	{
		RegisterAuthRoutes(api, authHandler, jwtMgr, autoStore) // 注册认证模块路由
		// 其他模块的路由注册函数也会在这里调用，例如：
		// RegisterTaskRoutes(api, taskHandler, jwtMgr) // 注册任务模块路由
		// RegisterProjectRoutes(api, projectHandler, jwtMgr) // 注册项目模块路由
		// ...
	}
}
