// internal/api/handler/task_handler.go
// Package handler
// 处理任务调度
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
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TaskHandler 项目成员模块处理器
type TaskHandler struct {
	taskService *service.TaskService
}

// NewTaskHandler 创建项目成员模块处理器
func NewTaskHandler(taskService *service.TaskService) *TaskHandler {
	return &TaskHandler{
		taskService: taskService,
	}
}

// CreateTask 创建任务
func (h *TaskHandler) CreateTask(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.ProjectID == nil {
		logger.Warn("parse project id failed: project id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var req dto.CreateTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse create task json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	creatorID := contextx.GetUserID(c) // 获取创建者ID
	projectID := *uri.ProjectID        // 项目ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("project_id", projectID),
		zap.Uint64("creator_id", creatorID),
		zap.String("title", strings.TrimSpace(req.Title)),
		zap.String("priority", strings.TrimSpace(req.Priority)),
		zap.String("due_date", strings.TrimSpace(utils.SafeStringValue(req.DueDate))), // 前端传入时间需为 ISO8601 格式
	)

	// 参数解析完成

	// 调用接口
	resp, err := h.taskService.CreateTask(c.Request.Context(), &service.CreateTaskParam{
		CreatorID:   creatorID,
		ProjectID:   projectID,
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(utils.SafeStringValue(req.Description)),
		Priority:    strings.TrimSpace(req.Priority),
		AssigneeID:  utils.SafeValue(req.AssigneeID),
		DueDate:     strings.TrimSpace(utils.SafeStringValue(req.DueDate)), // 前端传入时间需为 ISO8601 格式
	})

	// 错误识别
	if err != nil {
		switch {
		// 参数不正确
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("create task rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 标题为空
		case errors.Is(err, service.ErrEmptyTaskTitle):
			logger.Warn("create task rejected: empty task title")
			response.Fail(c, errmsg.EmptyTaskTitle)

		// 标题格式不正确
		case errors.Is(err, service.ErrInvalidTaskTitle):
			logger.Warn("create task rejected: invalid task title")
			response.Fail(c, errmsg.InvalidTaskTitleFormat)

		// 描述格式不正确
		case errors.Is(err, service.ErrInvalidTaskDescription):
			logger.Warn("create task rejected: invalid task description")
			response.Fail(c, errmsg.InvalidTaskDescriptionFormat)

		// 优先级格式不正确
		case errors.Is(err, service.ErrInvalidTaskPriority):
			logger.Warn("create task rejected: invalid task priority")
			response.Fail(c, errmsg.InvalidTaskPriorityFormat)

		// 时间格式不正确
		case errors.Is(err, service.ErrInvalidTaskTime):
			logger.Warn("create task rejected: invalid task time")
			response.Fail(c, errmsg.InvalidTaskTimeFormat)

		// 项目不存在
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("create task failed: project not found")
			response.Fail(c, errmsg.ProjectNotFound)

		// 创建者不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("create task failed: creator not found")
			response.Fail(c, errmsg.UserNotFound)

		// 无权限
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("create task rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 指派的负责人不存在
		case errors.Is(err, service.ErrAssigneeNotFount):
			logger.Warn("create task failed: assignee not found")
			response.Fail(c, errmsg.AssigneeNotFound)

		// 指派的负责人不是项目成员
		case errors.Is(err, service.ErrAssigneeNotProjectMember):
			logger.Warn("create task rejected: assignee is not project member")
			response.Fail(c, errmsg.AssigneeNotProjectMember)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("create task canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("create task deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("create task failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "create task failed")
		}
		return
	}

	logger.Info("create task success")
	response.SuccessWithData(c, resp)
}

// ListProjectTasks 获取项目下的任务列表
func (h *TaskHandler) ListProjectTasks(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.ProjectID == nil {
		logger.Warn("parse project id failed: project id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var query dto.ListTaskQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse list project tasks query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c) // 当前操作用户ID
	projectID := *uri.ProjectID     // 项目ID

	status := strings.TrimSpace(query.Status)
	priority := strings.TrimSpace(query.Priority)
	keyword := strings.TrimSpace(query.Keyword)
	sortBy := strings.TrimSpace(query.SortBy)
	sortOrder := strings.ToUpper(strings.TrimSpace(query.SortOrder))

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.Int("page", query.Page),
		zap.Int("page_size", query.PageSize),
		zap.String("status", status),
		zap.String("priority", priority),
		zap.String("keyword", keyword),
		zap.String("sort_by", sortBy),
		zap.String("sort_order", sortOrder),
		zap.Bool("has_assignee_filter", query.AssigeeID != nil), // nil 表示不筛选负责人，0 表示筛选未分配任务
	)

	if query.AssigeeID != nil {
		logger = logger.With(zap.Uint64("assignee_id", *query.AssigeeID))
	}

	resp, err := h.taskService.ListProjectTasks(c.Request.Context(), &service.ListProjectTasksParam{
		UserID:    userID,
		ProjectID: projectID,
		Page:      query.Page,
		PageSize:  query.PageSize,
		NeedTotal: query.NeedTotal,
		Status:    status,
		Priority:  priority,
		Keyword:   keyword,
		SortBy:    sortBy,
		SortOrder: sortOrder,

		// AssigneeID 语义：
		// nil：不筛选负责人
		// 0：筛选未分配任务
		// >0：筛选指定负责人
		AssigneeID: query.AssigeeID,
	})

	// 区别错误
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("list project tasks rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 状态不正确
		case errors.Is(err, service.ErrInvalidTaskStatus):
			logger.Warn("list project tasks rejected: invalid task status")
			response.Fail(c, errmsg.InvalidTaskStatusFormat)

		// 优先级不正确
		case errors.Is(err, service.ErrInvalidTaskPriority):
			logger.Warn("list project tasks rejected: invalid task priority")
			response.Fail(c, errmsg.InvalidTaskPriorityFormat)

		// 排序规则不正确
		case errors.Is(err, service.ErrInvalidTaskSortBy):
			logger.Warn("list project tasks rejected: invalid task sort by")
			response.Fail(c, errmsg.InvalidTaskSortByFormat)

		// 排序顺序不正确
		case errors.Is(err, service.ErrInvalidTaskSortOrder):
			logger.Warn("list project tasks rejected: invalid task sort order")
			response.Fail(c, errmsg.InvalidTaskSortOrderFormat)

		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("list project tasks failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 项目不存在（无权限）
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("list project tasks failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 权限不允许
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("list project tasks rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 筛选的负责人不存在
		case errors.Is(err, service.ErrAssigneeNotFount):
			logger.Warn("list project tasks failed: assignee not found")
			response.Fail(c, errmsg.AssigneeNotFound)

		// 筛选负责不是项目成员
		case errors.Is(err, service.ErrAssigneeNotProjectMember):
			logger.Warn("list project tasks rejected: assignee is not project member")
			response.Fail(c, errmsg.AssigneeNotProjectMember)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("list project tasks canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("list project tasks deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("list project tasks failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "list project tasks failed")
		}
		return
	}

	logger.Info("list project tasks success")
	response.SuccessWithData(c, resp)
}

// ListUserTasks 获取用户的任务列表
func (h *TaskHandler) ListUserTasks(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析
	var query dto.ListTaskQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse list user tasks query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c)
	status := strings.TrimSpace(query.Status)
	priority := strings.TrimSpace(query.Priority)
	keyword := strings.TrimSpace(query.Keyword)
	sortBy := strings.TrimSpace(query.SortBy)
	sortOrder := strings.ToUpper(strings.TrimSpace(query.SortOrder))

	// 业务定义：
	// 0 全量查询
	projectID := utils.SafeValue(query.ProjectID)

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.Int("page", query.Page),
		zap.Int("page_size", query.PageSize),
		zap.String("status", status),
		zap.String("priority", priority),
		zap.String("keyword", keyword),
		zap.String("sort_by", sortBy),
		zap.String("sort_order", sortOrder),
	)

	resp, err := h.taskService.ListUserTasks(c.Request.Context(), &service.ListUserTasksParam{
		UserID:    userID,
		Page:      query.Page,
		PageSize:  query.PageSize,
		NeedTotal: query.NeedTotal,
		Status:    status,
		Priority:  priority,
		Keyword:   keyword,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		ProjectID: projectID,
	})

	// 判断错误
	if err != nil {
		switch {
		// 错误参数
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("list user tasks rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 状态不正确
		case errors.Is(err, service.ErrInvalidTaskStatus):
			logger.Warn("list user tasks rejected: invalid task status")
			response.Fail(c, errmsg.InvalidTaskStatusFormat)

		// 优先级不正确
		case errors.Is(err, service.ErrInvalidTaskPriority):
			logger.Warn("list user tasks rejected: invalid task priority")
			response.Fail(c, errmsg.InvalidTaskPriorityFormat)

		// 排序规则不正确
		case errors.Is(err, service.ErrInvalidTaskSortBy):
			logger.Warn("list user tasks rejected: invalid task sort by")
			response.Fail(c, errmsg.InvalidTaskSortByFormat)

		// 排序顺序不正确
		case errors.Is(err, service.ErrInvalidTaskSortOrder):
			logger.Warn("list user tasks rejected: invalid task sort order")
			response.Fail(c, errmsg.InvalidTaskSortOrderFormat)

		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("list user tasks failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 项目不存在（无权限）
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("list user tasks failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 权限不允许
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("list user tasks rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("list user tasks canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("list user tasks deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("list user tasks failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "list user tasks failed")
		}
		return
	}

	logger.Info("list user tasks success")
	response.SuccessWithData(c, resp)
}

// GetTaskDetail 获取任务详情
func (h *TaskHandler) GetTaskDetail(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse task id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.TaskID == nil {
		logger.Warn("parse task id failed: task id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c) // 当前操作用户ID
	taskID := *uri.TaskID           // 任务ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("task_id", taskID),
	)

	resp, err := h.taskService.GetTaskDetail(c.Request.Context(), &service.GetTaskDetailParam{
		UserID: userID,
		TaskID: taskID,
	})

	// 错误判断
	if err != nil {
		switch {
		// 参数错误
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("get task detail rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("get task detail failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 任务不存在（没有权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("get task detail failed: task not found")
			response.Fail(c, errmsg.TaskNoPermission)

		// 没有任务权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("get task detail rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("get task detail canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("get task detail deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("get task detail failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "get task detail failed")
		}
		return
	}

	logger.Info("get task detail success")
	response.SuccessWithData(c, resp)
}

// UpdateTaskBaseInfo 更新任务基础信息
func (h *TaskHandler) UpdateTaskBaseInfo(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse task id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.TaskID == nil {
		logger.Warn("parse task id failed: task id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var req dto.UpdateTaskBaseInfoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse update task base info json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c) // 当前操作用户ID
	taskID := *uri.TaskID           // 任务ID
	title := strings.TrimSpace(req.TiTle)
	priority := strings.TrimSpace(req.Priority)

	var description, dueDate *string
	if req.Description != nil {
		v := strings.TrimSpace(utils.SafeStringValue(req.Description))
		description = &v
	}
	if req.DueDate != nil {
		v := strings.TrimSpace(utils.SafeStringValue(req.DueDate))
		dueDate = &v // 前端传入时间需为 ISO8601 格式；空字符串表示清空为 NULL
	}

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("task_id", taskID),
		zap.Bool("update_title", title != ""),
		zap.Bool("update_priority", priority != ""),
		zap.Bool("update_description", description != nil),
		zap.Bool("update_due_date", dueDate != nil),
		zap.String("title", title),
		zap.String("priority", priority),
	)

	if dueDate != nil {
		logger = logger.With(zap.String("due_date", *dueDate))
	}

	resp, err := h.taskService.UpdateTaskBaseInfo(c.Request.Context(), &service.UpdateTaskBaseInfoParam{
		UserID:      userID,
		TaskID:      taskID,
		Title:       title,
		Priority:    priority,
		Description: description,
		DueDate:     dueDate,
	})

	// 错误检验
	if err != nil {
		switch {
		// 错误的参数
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("update task base info rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 标题格式不正确
		case errors.Is(err, service.ErrInvalidTaskTitle):
			logger.Warn("update task base info rejected: invalid task title")
			response.Fail(c, errmsg.InvalidTaskTitleFormat)

		// 优先级格式不正确
		case errors.Is(err, service.ErrInvalidTaskPriority):
			logger.Warn("update task base info rejected: invalid task priority")
			response.Fail(c, errmsg.InvalidTaskPriorityFormat)

		// 描述格式不正确
		case errors.Is(err, service.ErrInvalidTaskDescription):
			logger.Warn("update task base info rejected: invalid task description")
			response.Fail(c, errmsg.InvalidTaskDescriptionFormat)

		// 时间格式不正确
		case errors.Is(err, service.ErrInvalidTaskTime):
			logger.Warn("update task base info rejected: invalid task time")
			response.Fail(c, errmsg.InvalidTaskTimeFormat)

		// 任务不存在（没有权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("update task base info failed: task not found")
			response.Fail(c, errmsg.TaskNoPermission)

		// 没有权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("update task base info rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("update task base info canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("update task base info deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("update task base info failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "update task base info failed")
		}
		return
	}

	logger.Info("update task base info success")
	response.SuccessWithData(c, resp)
}

// UpdateTaskStatus 更新任务状态
func (h *TaskHandler) UpdateTaskStatus(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数校验
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse task id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.TaskID == nil {
		logger.Warn("parse task id failed: task id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var req dto.UpdateTaskStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse update task status json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c)         // 当前操作用户ID
	taskID := *uri.TaskID                   // 任务ID
	status := strings.TrimSpace(req.Status) // 任务状态；空字符串表示不更新，由 Service 层直接返回当前状态

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("task_id", taskID),
		zap.String("status", status),
	)

	resp, err := h.taskService.UpdateTaskStatus(c.Request.Context(), &service.UpdateTaskStatusParam{
		UserID: userID,
		TaskID: taskID,
		Status: status,
	})

	// 判断错误
	if err != nil {
		switch {
		// 参数错误
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("update task status rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 任务状态格式非法
		case errors.Is(err, service.ErrInvalidTaskStatus):
			logger.Warn("update task status rejected: invalid task status")
			response.Fail(c, errmsg.InvalidTaskStatusFormat)

		// 任务不存在（无权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("update task status failed: task not found")
			response.Fail(c, errmsg.TaskNoPermission)

		// 无权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("update task status rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("update task status canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("update task status deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("update task status failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "update task status failed")
		}
		return
	}

	logger.Info("update task status success")
	response.SuccessWithData(c, resp)
}

// UpdateTaskAssignee 更新任务的负责人
func (h *TaskHandler) UpdateTaskAssignee(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数解析
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse task id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.TaskID == nil {
		logger.Warn("parse task id failed: task id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var req dto.UpdateTaskAssigneeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse update task assignee json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c) // 当前操作用户ID
	taskID := *uri.TaskID           // 任务ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("task_id", taskID),
		zap.Bool("has_assignee_id", req.AssigneeID != nil), // nil 表示清空负责人
	)
	if req.AssigneeID != nil {
		logger = logger.With(zap.Uint64("assignee_id", *req.AssigneeID))
	}

	resp, err := h.taskService.UpdateTaskAssignee(c.Request.Context(), &service.UpdateTaskAssigneeParam{
		UserID:     userID,
		TaskID:     taskID,
		AssigneeID: req.AssigneeID, // AssigneeID == nil：清空负责人；非 nil：更新为指定负责人
	})

	// 错误解析
	if err != nil {
		switch {
		// 错误参数
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("update task assignee rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 任务不存在（无权限）
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("update task assignee failed: task not found")
			response.Fail(c, errmsg.TaskNoPermission)

		// 无权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("update task assignee rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 负责人没有找到
		case errors.Is(err, service.ErrAssigneeNotFount):
			logger.Warn("update task assignee failed: assignee not found")
			response.Fail(c, errmsg.AssigneeNotFound)

		// 负责人不是项目成员
		case errors.Is(err, service.ErrAssigneeNotProjectMember):
			logger.Warn("update task assignee rejected: assignee is not project member")
			response.Fail(c, errmsg.AssigneeNotProjectMember)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("update task assignee canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("update task assignee deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("update task assignee failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "update task assignee failed")
		}
		return
	}

	logger.Info("update task assignee success")
	response.SuccessWithData(c, resp)
}

// UpdateTaskSortOrder 更新任务的排序值
func (h *TaskHandler) UpdateTaskSortOrder(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数校验
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.ProjectID == nil {
		logger.Warn("parse project id failed: project id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var req dto.UpdateTaskSortOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse update task sort order json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c) // 当前操作用户ID
	projectID := *uri.ProjectID     // 项目ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.Int("item_count", len(req.Items)),
	)

	resp, err := h.taskService.UpdateTaskSortOrder(c.Request.Context(), &service.UpdateTaskSortOrderParam{
		UserID:    userID,
		ProjectID: projectID,
		Items:     req.Items, // 排序项：包含 task_id 与 sort_order
	})

	// 错误判断
	if err != nil {
		switch {
		// 错误参数
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("update task sort order rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 无权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("update task sort order rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 排序列表为空
		case errors.Is(err, service.ErrEmptyTaskSortItems):
			logger.Warn("update task sort order rejected: sort items is empty")
			response.Fail(c, errmsg.EmptyTaskSortItem)

		// 排序项非法
		case errors.Is(err, service.ErrInvalidTaskSortOrderItem):
			logger.Warn("update task sort order rejected: invalid sort item")
			response.Fail(c, errmsg.InvalidTaskSortItem)

		// 排序更新失败，没有任何数据被更新
		case errors.Is(err, service.ErrTaskSortNoRowsUpdated):
			logger.Warn("update task sort order failed: no rows updated")
			response.Fail(c, errmsg.InvalidTaskSortItem)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("update task sort order canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("update task sort order deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("update task sort order failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "update task sort order failed")
		}
		return
	}

	logger.Info("update task sort order success")
	response.SuccessWithData(c, resp)
}

// RemoveTask 删除任务
func (h *TaskHandler) RemoveTask(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数校验
	var uri dto.TaskIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse task id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	if uri.TaskID == nil {
		logger.Warn("parse task id failed: task id is nil")
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析成功
	userID := contextx.GetUserID(c) // 当前操作用户ID
	taskID := *uri.TaskID           // 任务ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("task_id", taskID),
	)

	_, err := h.taskService.RemoveTask(c.Request.Context(), &service.RemoveTaskParam{
		UserID: userID,
		TaskID: taskID,
	})
	if err != nil {
		switch {
		// 错误参数
		case errors.Is(err, service.ErrInvalidTaskParam):
			logger.Warn("delete task rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 无权限
		case errors.Is(err, service.ErrTaskForbidden):
			logger.Warn("delete task rejected: task forbidden")
			response.Fail(c, errmsg.TaskNoPermission)

		// 任务不存在
		case errors.Is(err, service.ErrTaskNotFound):
			logger.Warn("delete task failed: task not found")
			response.Fail(c, errmsg.TaskNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("delete task failed canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("delete task failed deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		// 其它错误
		default:
			logger.Error("delete task failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "delete task failed")
		}
		return
	}

	logger.Info("delete task success")
	response.Success(c)
}
