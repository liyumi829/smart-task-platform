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

// RegisterProjectMemberRoutes 注册项目成员模块路由
func RegisterProjectMemberRoutes(
	api *gin.RouterGroup,
	projectMemberHandler *handler.ProjectMemberHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 项目成员路由组：
	// /api/v1/projects/:id/members
	//
	// :id 表示 project_id
	// userId 表示对哪一个用户资源
	memberGroup := api.Group("/projects/:id/members")
	memberGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 添加项目成员
		memberGroup.POST("", projectMemberHandler.AddProjectMember)

		// 获取项目成员列表
		memberGroup.GET("", projectMemberHandler.ListProjectMembers)

		// 修改项目成员角色
		memberGroup.PATCH("/:userId", projectMemberHandler.UpdateProjectMember)

		// 移除项目成员，软删除/归档语义
		memberGroup.DELETE("/:userId", projectMemberHandler.RemoveProjectMember)
	}
}
