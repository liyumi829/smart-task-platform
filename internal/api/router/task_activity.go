// internal/api/router/task_activity.go
// Package router
// 任务活动路由

package router

import (
	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"

	"github.com/gin-gonic/gin"
)

// RegisterTaskActivityRoutes 注册项目成员模块路由
func RegisterTaskActivityRoutes(
	api *gin.RouterGroup,
	taskActivityHandler *handler.TaskActivityHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 项目下的任务下的任务分组
	// /projects/:projectId/tasks/:taskId/activities
	activityGroup := api.Group("/projects/:projectId/tasks/:taskId/activities")
	activityGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 获取活动列表
		activityGroup.GET("", taskActivityHandler.ListTaskActivities)
	}
}
