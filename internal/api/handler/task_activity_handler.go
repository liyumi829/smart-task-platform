// internal/api/handler/task_activities_handler.go
// Package handler
// 任务活动模块的处理器

package handler

import (
	"context"
	"errors"
	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/pkg/errmsg"
	"smart-task-platform/internal/pkg/response"
	"smart-task-platform/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TaskActivityHandler 任务活动处理器
type TaskActivityHandler struct {
	taskActivityService *service.TaskActivityService
}

// NewTaskActivityHandler 实例化任务活动处理器
func NewTaskActivityHandler(taskActivityService *service.TaskActivityService) *TaskActivityHandler {
	return &TaskActivityHandler{
		taskActivityService: taskActivityService,
	}
}

// ListTaskActivities 获取任务活动列表
func (h *TaskActivityHandler) ListTaskActivities(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：URI
	var uri dto.TaskActivityIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse list task activities uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 参数解析：Query
	var query dto.ListTaskActivitiesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse list task activities query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析业务参数
	userID := contextx.GetUserID(c)
	projectID := uri.ProjectID
	taskID := uri.TaskID
	page := query.Page
	pageSize := query.PageSize

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.Uint64("task_id", taskID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
	)

	resp, err := h.taskActivityService.ListTaskActivities(c.Request.Context(),
		&service.ListTaskActivitiesParam{
			UserID:    userID,
			ProjectID: projectID,
			TaskID:    taskID,
			Page:      page,
			PageSize:  pageSize,
			NeedTotal: query.NeedTotal,
		})
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidTaskActivityParam):
			logger.Warn("list task activities rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 任务不存在（无任务权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("list task activities failed: task not found or no permission")
			response.Fail(c, errmsg.TaskNoPermission)

		// 无任务权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("list task activities rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("list task activities canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("list task activities deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("list task activities failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "list task activities failed")
		}
		return
	}

	logger.Info("list task activities success")
	response.SuccessWithData(c, resp)
}
