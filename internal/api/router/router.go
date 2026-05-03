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
func Register(
	r *gin.Engine,
	jwtMgr *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
	authHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	projectHandler *handler.ProjectHandler,
	projectMemberHandler *handler.ProjectMemberHandler,
	taskHandler *handler.TaskHandler,
	taskCommentHandler *handler.TaskCommentHandler,
) {
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
		RegisterAuthRoutes(api, authHandler, jwtMgr, authStore)                   // 注册认证模块路由
		RegisterUserRoutes(api, userHandler, jwtMgr, authStore)                   // 注册用户模块路由
		RegisterProjectRoutes(api, projectHandler, jwtMgr, authStore)             // 注册项目模块路由
		RegisterProjectMemberRoutes(api, projectMemberHandler, jwtMgr, authStore) // 注册项目成员模块路由
		RegisterTaskRoutes(api, taskHandler, jwtMgr, authStore)                   // 注册任务模块路由
		RegisterTaskCommentRoutes(api, taskCommentHandler, jwtMgr, authStore)     // 注册任务评论模块路由
		// 其他模块的路由注册函数也会在这里调用，例如：
		// ...
	}
}
