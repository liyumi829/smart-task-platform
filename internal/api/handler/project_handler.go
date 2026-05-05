// internal/api/handler/project_handler.go

// Package handler 处理器层，负责处理 HTTP 请求并调用服务层进行业务逻辑处理
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

// ProjectHandler 项目模块处理器
type ProjectHandler struct {
	projectService *service.ProjectService
}

// NewProjectHandler 创建项目模块处理器
func NewProjectHandler(projectService *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{
		projectService: projectService,
	}
}

// CreateProject 创建项目
func (h *ProjectHandler) CreateProject(c *gin.Context) {
	// 绑定参数参数
	var req dto.CreateProjectReq

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind create project request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数解析成功
	userID := contextx.GetUserID(c) // 获取用户ID，后续用到

	// 追加 user_id，避免重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.String("project_name", req.Name),
	)

	resp, err := h.projectService.CreateProject(c.Request.Context(),
		&service.CreateProjectParams{
			UserID:      userID,
			Name:        req.Name,
			Description: req.Description,
			StartTime:   req.StartDate,
			EndTime:     req.EndDate,
		})
	if err != nil {
		switch {
		// 非法的项目请求的参数
		case errors.Is(err, service.ErrInvalidProjectParam):
			logger.Warn("create project rejected: invalid project params")
			response.Fail(c, errmsg.InvalidProjectParams)

		// 非法的项目名称格式
		case errors.Is(err, service.ErrInvalidProjectName):
			logger.Warn("create project rejected: invalid project name format")
			response.Fail(c, errmsg.InvalidProjectNameFormat)

		// 项目名称为空
		case errors.Is(err, service.ErrEmptyProjectName):
			logger.Warn("create project rejected: project name is empty")
			response.FailWithMessage(c, errmsg.InvalidProjectNameFormat, "project name can not be empty")

		// 非法的时间参数
		case errors.Is(err, service.ErrInvalidTime):
			logger.Warn("create project rejected: invalid time param",
				zap.String("start_date", req.StartDate),
				zap.String("end_date", req.EndDate),
			)
			response.FailWithMessage(c, errmsg.InvalidTimeParam, "Invalid project time param")

		// 非法的时间范围
		case errors.Is(err, service.ErrInvalidTimeRange):
			logger.Warn("create project rejected: invalid time range",
				zap.String("start_date", req.StartDate),
				zap.String("end_date", req.EndDate),
			)
			response.FailWithMessage(c, errmsg.InvalidTimeRange, "Invalid time range(start < end)")

		// 项目描述过长
		case errors.Is(err, service.ErrInvalidProjectDescription):
			logger.Warn("create project rejected: invalid description",
				zap.Int("description_len", len(req.Description)),
			)
			response.FailWithMessage(c, errmsg.InvalidProjectDescription, "Project description cannot exceed 200 characters")

		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("create project failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("create project canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("create project deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("create project failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "create project failed")
		}
		return
	}

	// 成功
	logger.Info("create project success",
		zap.Uint64("project_id", resp.ID),
	)
	response.SuccessWithData(c, resp)
}

// ListProjects 获取项目列表
func (h *ProjectHandler) ListProjects(c *gin.Context) {
	var query dto.ProjectListQuery

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("bind list projects request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数解析成功，调用服务
	userID := contextx.GetUserID(c)

	// 追加 user_id，避免重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Int("page", query.Page),
		zap.Int("page_size", query.PageSize),
		zap.String("status", query.Status),
		zap.String("keyword", query.Keyword),
	)

	resp, err := h.projectService.ListProjects(c.Request.Context(), &service.ListProjectsParam{
		UserID:    userID,
		Page:      query.Page,
		PageSize:  query.PageSize,
		NeedTotal: query.NeedTotal,
		Status:    query.Status,
		Keyword:   query.Keyword,
	})
	if err != nil {
		switch {
		// 非法的项目请求的参数
		case errors.Is(err, service.ErrInvalidProjectParam):
			logger.Warn("list projects rejected: invalid project params")
			response.Fail(c, errmsg.InvalidProjectParams)

		// 非法的项目状态
		case errors.Is(err, service.ErrInvalidProjectStatus):
			logger.Warn("list projects rejected: invalid project status",
				zap.String("status", query.Status),
			)
			response.Fail(c, errmsg.InvalidProjectStatus)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("list projects rejected canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("list projects rejected deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("list projects failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "list projects failed")
		}
		return
	}

	// 成功
	logger.Info("list projects success",
		zap.Int("page", resp.Page),
		zap.Int("page_size", resp.PageSize),
		zap.Bool("has_more", resp.HasMore),
		zap.Int("result_count", len(resp.List)),
	)
	response.SuccessWithData(c, resp)
}

// GetProjectDetail 获取项目详细情况
func (h *ProjectHandler) GetProjectDetail(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	var req dto.ProjectIDUri
	if err := c.ShouldBindUri(&req); err != nil {
		logger.Warn("parse project id failed",
			zap.Error(err))
		response.Fail(c, errmsg.InvalidProjectParams) // 错误的项目参数
		return
	}

	// 参数解析成功，调用服务
	userID := contextx.GetUserID(c)
	projectID := req.ProjectID

	// 追加 user_id 和 project_id，避免重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
	)

	resp, err := h.projectService.GetProjectDetail(c.Request.Context(), &service.GetProjectDetailParam{
		UserID:    userID,
		ProjectID: projectID,
	})
	if err != nil {
		switch {
		// 非法的项目请求的参数
		case errors.Is(err, service.ErrInvalidProjectParam):
			logger.Warn("get project detail rejected: invalid project params")
			response.Fail(c, errmsg.InvalidProjectParams)

		// 项目不存在
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("get project detail failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 项目成员没有找到
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("get project detail failed: project member not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 没有项目权限
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("get project detail rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("get project detail canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("get project detail deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("get project detail failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "get project detail failed")
		}
		return
	}

	// 成功
	logger.Info("get project detail success")
	response.SuccessWithData(c, resp)
}

// UpdateProject 更新项目数据
func (h *ProjectHandler) UpdateProject(c *gin.Context) {
	var req dto.UpdateProjectReq

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	var uri dto.ProjectIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed",
			zap.Error(err))
		response.Fail(c, errmsg.InvalidProjectParams) // 错误的项目参数
		return
	}
	// 解析成功
	projectID := uri.ProjectID

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind update project request failed",
			zap.Uint64("project_id", projectID),
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数解析成功，调用服务
	userID := contextx.GetUserID(c)

	// 追加 user_id 和 project_id，避免重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.String("project_name", req.Name),
		zap.String("status", req.Status),
	)

	resp, err := h.projectService.UpdateProject(c.Request.Context(), &service.UpdateProjectParam{
		UserID:      userID,
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		StartTime:   req.StartDate,
		EndTime:     req.EndDate,
	})
	if err != nil {
		switch {
		// 非法的项目请求的参数
		case errors.Is(err, service.ErrInvalidProjectParam):
			logger.Warn("update project rejected: invalid project params")
			response.Fail(c, errmsg.InvalidProjectParams)

		// 非法的项目状态
		case errors.Is(err, service.ErrInvalidProjectStatus):
			logger.Warn("update project rejected: invalid project status",
				zap.String("status", req.Status),
			)
			response.Fail(c, errmsg.InvalidProjectStatus)

		// 非法的项目名称
		case errors.Is(err, service.ErrInvalidProjectName):
			logger.Warn("update project rejected: invalid project name")
			response.Fail(c, errmsg.InvalidProjectNameFormat)

		// 非法的时间参数
		case errors.Is(err, service.ErrInvalidTime):
			logger.Warn("update project rejected: invalid time param",
				zap.String("start_date", req.StartDate),
				zap.String("end_date", req.EndDate))
			response.FailWithMessage(c, errmsg.InvalidTimeParam, "Invalid project time param")

		// 非法的时间范围
		case errors.Is(err, service.ErrInvalidTimeRange):
			logger.Warn("update project rejected: invalid time range",
				zap.String("start_date", req.StartDate),
				zap.String("end_date", req.EndDate))
			response.FailWithMessage(c, errmsg.InvalidTimeRange, "Invalid time range(start < end)")

		// 项目描述过长
		case errors.Is(err, service.ErrInvalidProjectDescription):
			logger.Warn("update project rejected: invalid description",
				zap.Int("description_len", len(req.Description)),
			)
			response.FailWithMessage(c, errmsg.InvalidProjectDescription, "Project description cannot exceed 200 characters")

		// 下面三种情况都返回没有权限
		// 项目不存在
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("update project failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 项目成员不存在
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("update project failed: project member not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 没有项目权限
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("update project rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("update project  canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("update project  deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("update project failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "update project failed")
		}
		return
	}

	// 成功
	logger.Info("update project success")
	response.SuccessWithData(c, resp)
}

// ArchiveProject 归档项目
func (h *ProjectHandler) ArchiveProject(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	var uri dto.ProjectIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed",
			zap.Error(err))
		response.Fail(c, errmsg.InvalidProjectParams) // 错误的项目参数
		return
	}
	// 参数解析成功，调用服务
	projectID := uri.ProjectID
	userID := contextx.GetUserID(c)

	// 追加 user_id 和 project_id，避免重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
	)

	resp, err := h.projectService.ArchiveProject(c.Request.Context(), userID, projectID)
	if err != nil {
		switch {
		// 非法的项目请求的参数
		case errors.Is(err, service.ErrInvalidProjectParam):
			logger.Warn("archive project rejected: invalid project params")
			response.Fail(c, errmsg.InvalidProjectParams)

		// 项目不存在
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("archive project failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 项目成员不存在
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("archive project failed: project member not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 没有项目权限
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("archive project rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 客户端主动断开或请求被取消，压测时常见，不作为服务端错误
		case errors.Is(err, context.Canceled):
			logger.Warn("archive project canceled",
				zap.Error(err),
			)
			response.Fail(c, errmsg.ClientNotFound)

		// 请求超时，需要重点关注
		case errors.Is(err, context.DeadlineExceeded):
			logger.Error("archive project deadline exceeded",
				zap.Error(err),
			)
			response.Fail(c, errmsg.TooManyRequest)

		default:
			logger.Error("archive project failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "archive project failed")
		}
		return
	}

	// 成功
	logger.Info("archive project success")
	response.SuccessWithData(c, resp)
}
