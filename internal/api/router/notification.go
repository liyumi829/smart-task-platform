// internal/api/router/notification.go
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

// RegisterNotificationRoutes 注册项目成员模块路由
func RegisterNotificationRoutes(
	api *gin.RouterGroup,
	notificationHandler *handler.NotificationHandler,
	jwtManager *authjwt.Manager,
	authStore *authredis.RedisAuthStore,
) {
	// 通知列表
	// /notifications
	notificationGroup := api.Group("/notifications")
	notificationGroup.Use(middleware.JWTAuth(jwtManager, authStore))
	{
		// 获取通知列表
		notificationGroup.GET("", notificationHandler.ListNotification)

		// 获取未读通知梳理
		notificationGroup.GET("/unread-count", notificationHandler.GetUnreadCount)

		// 标记一条消息已读
		notificationGroup.PATCH("/:notificationId/read", notificationHandler.MarkAsRead)

		// 标记全部消息已读
		notificationGroup.PATCH("/read-all", notificationHandler.MarkAllAsRead)
	}
}
