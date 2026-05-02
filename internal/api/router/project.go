// internal/api/router/project_router.go
// Package router 路由层，负责注册 HTTP 路由
package router

import (
	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/middleware"
	authjwt "smart-task-platform/internal/pkg/jwt"
	authredis "smart-task-platform/internal/pkg/redis"

	"github.com/gin-gonic/gin"
)

// RegisterProjectRoutes 注册项目模块路由
func RegisterProjectRoutes(
	api *gin.RouterGroup,
	projectHandler *handler.ProjectHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {

	// 路由分组：/projects
	privateGroup := api.Group("/projects")
	privateGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 创建项目
		privateGroup.POST("", projectHandler.CreateProject)

		// 获取项目列表
		privateGroup.GET("", projectHandler.ListProjects)

		// 获取项目详细情况
		privateGroup.GET("/:projectId", projectHandler.GetProjectDetail)

		// 更新项目数据
		privateGroup.PUT("/:projectId", projectHandler.UpdateProject)

		// 归档项目
		privateGroup.PATCH("/:projectId/archive", projectHandler.ArchiveProject)
	}
}
