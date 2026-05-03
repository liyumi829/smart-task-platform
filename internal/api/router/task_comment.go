// internal/api/router/task_comment.go
// Package router
// 任务评论模块路由
package router

import (
	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"

	"github.com/gin-gonic/gin"
)

// RegisterTaskCommentRoutes 注册项目成员模块路由
func RegisterTaskCommentRoutes(
	api *gin.RouterGroup,
	taskCommentHandler *handler.TaskCommentHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 项目下的任务下的评论分组
	// /projects/:projectId/tasks/:taskId/comments
	commentGroup := api.Group("/projects/:projectId/tasks/:taskId/comments")
	commentGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 创建评论
		commentGroup.POST("", taskCommentHandler.CreateTaskComment)

		// 获取评论列表
		commentGroup.GET("", taskCommentHandler.ListTaskComments)

		// 删除评论
		commentGroup.DELETE("/:commentId", taskCommentHandler.RemoveTaskComment)
	}
}
