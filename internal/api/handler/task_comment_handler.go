// internal/api/handler/task_comment_handler.go
// Package handler
// 任务评论模块的处理器

package handler

import (
	"errors"
	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/pkg/errmsg"
	"smart-task-platform/internal/pkg/response"
	"smart-task-platform/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TaskCommmentHandler 任务评论处理器
type TaskCommentHandler struct {
	taskCommentService *service.TaskCommentService
}

// NewTaskCommentHandler 实例化任务评论处理器
func NewTaskCommentHandler(taskCommentService *service.TaskCommentService) *TaskCommentHandler {
	return &TaskCommentHandler{
		taskCommentService: taskCommentService,
	}
}

// CreateTaskComment 创建评论
func (h *TaskCommentHandler) CreateTaskComment(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：URI
	var uri dto.ProjectTaskCommentUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse create task comment uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 参数解析：JSON Body
	var req dto.CreateTaskCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse create task comment json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析业务参数
	creatorID := contextx.GetUserID(c)
	projectID := uri.ProjectID
	taskID := uri.TaskID
	parentID := req.ParentID
	content := strings.TrimSpace(req.Content)

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("creator_id", creatorID),
		zap.Uint64("project_id", projectID),
		zap.Uint64("task_id", taskID),
		zap.String("content", content),
	)

	// 父评论 ID 可能为空，非空时追加到日志
	if parentID != nil {
		logger = logger.With(zap.Uint64("parent_comment_id", *parentID))
	}

	// 调用 Service 创建任务评论
	resp, err := h.taskCommentService.CreateTaskComment(c.Request.Context(), &service.CreateTaskCommentParam{
		CreatorID:       creatorID,
		ProjectID:       projectID,
		TaskID:          taskID,
		Content:         content,
		ParentCommentID: parentID,
	})

	// 错误识别
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidTaskCommentParam):
			logger.Warn("create task comment rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 评论内容为空
		case errors.Is(err, service.ErrEmptyTaskCommentContent):
			logger.Warn("create task comment rejected: empty content")
			response.Fail(c, errmsg.EmptyTaskCommentContent)

		// 评论内容格式不合法
		case errors.Is(err, service.ErrInvalidTaskCommentContent):
			logger.Warn("create task comment rejected: invalid content")
			response.Fail(c, errmsg.InvalidTaskCommentContent)

		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("create task comment failed: creator user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 任务不存在（无权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("create task comment failed: task not found or no permission")
			response.Fail(c, errmsg.TaskNoPermission)

		// 无任务评论权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("create task comment rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 父评论不存在
		case errors.Is(err, service.ErrParentCommentNotFound):
			logger.Warn("create task comment failed: parent comment not found")
			response.Fail(c, errmsg.ParentCommentNotFound)

		// 父评论不合法
		case errors.Is(err, service.ErrInvalidParentComment):
			logger.Warn("create task comment rejected: invalid parent comment")
			response.Fail(c, errmsg.InvalidParentComment)

		// 其它错误
		default:
			logger.Error("create task comment failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "create task comment failed")
		}
		return
	}

	logger.Info("create task comment success")
	response.SuccessWithData(c, resp)
}

// ListTaskComments 查询任务评论列表
func (h *TaskCommentHandler) ListTaskComments(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：URI
	var uri dto.ProjectTaskCommentUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse list task comments uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 参数解析：Query
	var query dto.ListTaskCommentsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse list task comments query failed", zap.Error(err))
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

	// 调用 Service 查询任务评论列表
	resp, err := h.taskCommentService.ListTaskComments(c.Request.Context(), &service.ListTaskCommentsParam{
		UserID:    userID,
		ProjectID: projectID,
		TaskID:    taskID,
		Page:      page,
		PageSize:  pageSize,
	})

	// 错误识别
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidTaskCommentParam):
			logger.Warn("list task comments rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 任务不存在（无任务权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("list task comments failed: task not found or no permission")
			response.Fail(c, errmsg.TaskNoPermission)

		// 无任务权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("list task comments rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 其它错误
		default:
			logger.Error("list task comments failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "list task comments failed")
		}
		return
	}

	logger.Info("list task comments success")
	response.SuccessWithData(c, resp)
}

// RemoveTaskComment 删除评论
func (h *TaskCommentHandler) RemoveTaskComment(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析：URI
	var uri dto.ProjectTaskCommentUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse remove task comment uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析业务参数
	userID := contextx.GetUserID(c)
	projectID := uri.ProjectID
	taskID := uri.TaskID
	commentID := uri.CommentID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.Uint64("task_id", taskID),
		zap.Uint64("comment_id", commentID),
	)

	// 调用 Service 删除任务评论
	_, err := h.taskCommentService.RemoveTaskComment(c.Request.Context(), &service.RemoveTaskCommentParam{
		UserID:    userID,
		ProjectID: projectID,
		TaskID:    taskID,
		CommentID: commentID,
	})

	// 错误识别
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidTaskCommentParam):
			logger.Warn("remove task comment rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 任务不存在（当作无权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("remove task comment failed: task not found or no permission")
			response.Fail(c, errmsg.TaskCommentNoPermission)

		// 无任务权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("remove task comment rejected: task forbidden")
			response.Fail(c, errmsg.TaskCommentNoPermission)

		// 任务评论不存在（当作无权限）
		case errors.Is(err, service.ErrTaskCommentNotFound):
			logger.Warn("remove task comment failed: comment not found or no permission")
			response.Fail(c, errmsg.TaskCommentNoPermission)

		// 项目成员不存在（当作无权限）
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("remove task comment failed: project member not found or no permission")
			response.Fail(c, errmsg.TaskCommentNoPermission)

		// 其它错误
		default:
			logger.Error("remove task comment failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "remove task comment failed")
		}
		return
	}

	logger.Info("remove task comment success")
	response.Success(c)
}
