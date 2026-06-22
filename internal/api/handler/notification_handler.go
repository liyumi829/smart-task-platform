// internal/api/handler/task_comment_handler.go
// Package handler
// 任务评论模块的处理器

package handler

import (
	"context"
	"errors"
	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/pkg/errmsg"
	"smart-task-platform/internal/pkg/response"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NotificationHandler 任务评论处理器
type NotificationHandler struct {
	notificationService *service.NotificationService
}

// NewNotificationHandler 实例化任务评论处理器
func NewNotificationHandler(notificationService *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
	}
}

// ListNotifications 获取通知列表
func (h *NotificationHandler) ListNotification(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：Query
	var query dto.ListNotificationsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse list notifications query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 业务参数
	userID := contextx.GetUserID(c)
	page := query.Page
	pageSize := query.PageSize
	isRead := utils.SafePtrClone(query.IsRead)

	// 追加业务字段
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
	)

	resp, err := h.notificationService.ListNotifications(c.Request.Context(),
		&service.ListNotificationsParam{
			UserID:    userID,
			Page:      page,
			PageSize:  pageSize,
			NeedTotal: query.NeedTotal,
			IsRead:    isRead,
		})
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidNotificationParam):
			logger.Warn("list notifications rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 客户端主动断开或请求被取消
		case errors.Is(err, context.Canceled):
			logger.Warn("list notifications canceled", zap.Error(err))
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("list notifications deadline exceeded", zap.Error(err))
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("list notifications failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "list notifications failed")
		}
		return
	}

	logger.Info("list notifications success")
	response.SuccessWithData(c, resp)
}

// GetUnreadCount 获取未读取的条数
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：Query
	var query dto.ListNotificationsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse get unread count query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 业务参数
	userID := contextx.GetUserID(c)
	logger = logger.With(zap.Uint64("user_id", userID))

	resp, err := h.notificationService.GetUnreadCount(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidNotificationParam):
			logger.Warn("get unread count rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		case errors.Is(err, context.Canceled):
			logger.Warn("get unread count canceled", zap.Error(err))
			response.Fail(c, errmsg.ClientNotFound)

		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("get unread count deadline exceeded", zap.Error(err))
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("get unread count failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "get unread notification count failed")
		}
		return
	}

	logger.Info("get unread count success")
	response.SuccessWithData(c, resp)
}

// MarkAsRead 标记某个通知为已读
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：URI
	var uri dto.NotificationIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse mark as read uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 业务参数
	userID := contextx.GetUserID(c)
	notificationID := uri.NotificationID
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("notification_id", notificationID),
	)

	resp, err := h.notificationService.MarkAsRead(c.Request.Context(), userID, notificationID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidNotificationParam):
			logger.Warn("mark notification as read rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		case errors.Is(err, service.ErrNotificationNotFound):
			logger.Warn("mark notification as read failed: notification not found")
			response.Fail(c, errmsg.NotFound)

		case errors.Is(err, context.Canceled):
			logger.Warn("mark notification as read canceled", zap.Error(err))
			response.Fail(c, errmsg.ClientNotFound)

		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("mark notification as read deadline exceeded", zap.Error(err))
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("mark notification as read failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "mark notification as read failed")
		}
		return
	}

	logger.Info("mark notification as read success")
	response.SuccessWithData(c, resp)
}

// MarkAllAsRead 标记全部通知为已读
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 业务参数
	userID := contextx.GetUserID(c)
	logger = logger.With(zap.Uint64("user_id", userID))

	_, err := h.notificationService.MarkAllAsRead(c.Request.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidNotificationParam):
			logger.Warn("mark all as read rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		case errors.Is(err, context.Canceled):
			logger.Warn("mark all as read canceled", zap.Error(err))
			response.Fail(c, errmsg.ClientNotFound)

		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("mark all as read deadline exceeded", zap.Error(err))
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("mark all as read failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "mark all notifications as read failed")
		}
		return
	}

	logger.Info("mark all notifications as read success")
	response.SuccessWithData(c, nil)
}
