// internal/api/router/project_member.go
// Package router 实现项目成员模块的路由
package router

import (
	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"

	"github.com/gin-gonic/gin"
)

// RegisterTaskRoutes 注册项目成员模块路由
func RegisterTaskRoutes(
	api *gin.RouterGroup,
	taskHandler *handler.TaskHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 项目下的任务路由组：
	// /api/v1/projects/:projectId/tasks/:taskId
	// :projectId 表示 projectID
	projectGroup := api.Group("/projects/:projectId/tasks")
	projectGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 创建任务
		projectGroup.POST("", taskHandler.CreateTask)

		// 获取项目下的任务列表
		projectGroup.GET("", taskHandler.ListProjectTasks)

		// 更新任务排序值
		projectGroup.PATCH("/sort", taskHandler.UpdateTaskSortOrder)
	}
	// 任务模块的路由组
	// /api/v1/tasks/:taskId
	// :taskId 表示 taskID
	taskGroup := api.Group("/tasks")
	taskGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 获取用户的任务列表
		taskGroup.GET("/my", taskHandler.ListUserTasks)

		// 获取任务详情
		taskGroup.GET("/:taskId", taskHandler.GetTaskDetail)

		// 更新任务基础信息
		taskGroup.PATCH("/:taskId/base-info", taskHandler.UpdateTaskBaseInfo)

		// 更新任务状态
		taskGroup.PATCH("/:taskId/status", taskHandler.UpdateTaskStatus)

		// 更新任务负责人
		taskGroup.PATCH("/:taskId/assignee", taskHandler.UpdateTaskAssignee)

		// 删除任务
		taskGroup.DELETE("/:taskId", taskHandler.RemoveTask)
	}
}
