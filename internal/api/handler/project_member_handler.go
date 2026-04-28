// internal/api/handler/project_member_handler.go
// Package handler 处理器层，负责处理 HTTP 请求并调用服务层进行业务逻辑处理

package handler

import (
	"errors"
	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/pkg/errmsg"
	"smart-task-platform/internal/pkg/response"
	"smart-task-platform/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ProjectMemberHandler 项目成员模块处理器
type ProjectMemberHandler struct {
	projectMemberService *service.ProjectMemberService
}

// NewProjectMemberHandler 创建项目成员模块处理器
func NewProjectMemberHandler(projectMemberService *service.ProjectMemberService) *ProjectMemberHandler {
	return &ProjectMemberHandler{
		projectMemberService: projectMemberService,
	}
}

// AddProjectMember 添加项目成员
func (h *ProjectMemberHandler) AddProjectMember(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)
	var uri dto.ProjectMemberIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	var req dto.AddProjectMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse add project member json failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}
	// 解析成功，构造请求响应
	invitorID := contextx.GetUserID(c) // 获取邀请者ID
	projectID := uri.ID                // 项目ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("project_id", projectID),
		zap.Uint64("invitor_id", invitorID),
		zap.Uint64("invited_user_id", req.UserID),
		zap.String("role", req.Role),
	)

	resp, err := h.projectMemberService.AddProjectMember(c.Request.Context(), &service.AddProjectMemberParam{
		ProjectID:     projectID,
		InvitorID:     invitorID,
		InvitedUserID: req.UserID,
		Role:          req.Role,
	})
	if err != nil {
		switch {
		// 非法参数
		case errors.Is(err, service.ErrInvalidProjectMemberParam):
			logger.Warn("add project member rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 项目角色为空
		case errors.Is(err, service.ErrEmptyProjectMemberRole):
			logger.Warn("add project member rejected: empty project member role")
			response.Fail(c, errmsg.EmptyProjectMemberRole)

		// 项目角色不合法
		case errors.Is(err, service.ErrInvalidProjectMemberRole):
			logger.Warn("add project member rejected: invalid project member role")
			response.Fail(c, errmsg.InvalidProjectMemberRole)

		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("add project member failed: invited user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 被邀请者已经是项目成员
		case errors.Is(err, service.ErrProjectMemberAlreadyExists):
			logger.Warn("add project member rejected: project member already exists")
			response.Fail(c, errmsg.ProjectMemberAlreadyExists)

		// 管理人员已经达到上限
		case errors.Is(err, service.ErrExceedsAdminMemberLimit):
			logger.Warn("add project member rejected: admin member limit exceeded")
			response.Fail(c, errmsg.ExceededAdminMemberLimit)

		// 项目不存在，统一返回无项目权限
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("add project member failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 邀请者不是项目成员，统一返回无项目权限
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("add project member failed: invitor project member not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 角色权限不足
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("add project member rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		default:
			logger.Error("add project member failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "add project member failed")
		}
		return
	}

	logger.Info("add project member success")

	response.SuccessWithData(c, resp) // 携带数据返回
}

// ListProjectMembers 获取项目成员列表
func (h *ProjectMemberHandler) ListProjectMembers(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 解析 URI 参数
	var uri dto.ProjectMemberIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project id failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析 Query 参数
	var query dto.ProjectMemberListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		logger.Warn("parse project member list query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	userID := contextx.GetUserID(c) // 操作用户ID
	projectID := uri.ID             // 项目ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
		zap.Int("page", query.Page),
		zap.Int("page_size", query.PageSize),
		zap.String("role", query.Role),
		zap.String("keyword", query.Keyword),
	)

	// 参数解析成功，调用服务
	resp, err := h.projectMemberService.ListProjectMembers(c.Request.Context(), &service.ListProjectMembersParam{
		UserID:    userID,
		ProjectID: projectID,
		Page:      query.Page,
		PageSize:  query.PageSize,
		Role:      query.Role,
		Keyword:   query.Keyword,
	})
	if err != nil {
		switch {
		// 非法参数
		case errors.Is(err, service.ErrInvalidProjectMemberParam):
			logger.Warn("list project members rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 角色不合法
		case errors.Is(err, service.ErrInvalidProjectMemberRole):
			logger.Warn("list project members rejected: invalid project member role")
			response.Fail(c, errmsg.InvalidProjectMemberRole)

		// 项目不存在，统一处理为无权限
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("list project members failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)
		// 成员不存在项目中，统一处理为无权限
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("list project members failed: current user is not project member")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 其它错误
		default:
			logger.Error("list project members failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "list project member error")
		}
		return
	}

	logger.Info("list project members success")
	response.SuccessWithData(c, resp)
}

// UpdateProjectMember 修改项目成员属性，这里特指修改角色
func (h *ProjectMemberHandler) UpdateProjectMember(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 解析 URI 参数
	var uri dto.ProjectMemberIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project member uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	// 解析 Request 参数
	var req dto.UpdateProjectMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("parse update project member query failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	modifierID := contextx.GetUserID(c) // 修改者用户ID
	modifiedUserID := uri.UserID        // 被修改用户ID
	projectID := uri.ID                 // 项目ID

	role := ""
	if req.Role != nil {
		role = *req.Role
	}

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("project_id", projectID),
		zap.Uint64("modifier_id", modifierID),
		zap.Uint64("modified_user_id", modifiedUserID),
		zap.String("role", role),
	)

	// 调用服务
	resp, err := h.projectMemberService.UpdateProjectMember(c.Request.Context(), &service.UpdateProjectMemberParam{
		ProjectID:      projectID,
		ModifierID:     modifierID,
		ModifiedUserID: modifiedUserID,
		Role:           &role,
	})
	if err != nil {
		switch {
		// 不合法的参数
		case errors.Is(err, service.ErrInvalidProjectMemberParam):
			logger.Warn("update project member rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 不合法的身份
		case errors.Is(err, service.ErrInvalidProjectMemberRole):
			logger.Warn("update project member rejected: invalid project member role")
			response.Fail(c, errmsg.InvalidProjectMemberRole)

		// 超过管理员人数限制
		case errors.Is(err, service.ErrExceedsAdminMemberLimit):
			logger.Warn("update project member rejected: admin member limit exceeded")
			response.Fail(c, errmsg.ExceededAdminMemberLimit)

		// 项目不存在，统一处理成无权限
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("update project member failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 项目成员不存在，统一处理成无权限
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("update project member failed: project member not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 没有项目操作权限
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("update project member rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 其它错误
		default:
			logger.Error("update project member failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "update project member failed")
		}
		return
	}

	logger.Info("update project member success")
	response.SuccessWithData(c, resp)
}

// RemoveProjectMember 移除项目成员
func (h *ProjectMemberHandler) RemoveProjectMember(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 解析 URI 参数
	var uri dto.ProjectMemberIDUri
	if err := c.ShouldBindUri(&uri); err != nil {
		logger.Warn("parse project member uri failed", zap.Error(err))
		response.Fail(c, errmsg.InvalidParams)
		return
	}

	operatorID := contextx.GetUserID(c) // 操作者用户ID
	projectID := uri.ID                 // 项目ID
	removedUserID := uri.UserID         // 被移除用户ID

	// 追加业务字段，避免后续日志重复传递
	logger = logger.With(
		zap.Uint64("project_id", projectID),
		zap.Uint64("operator_id", operatorID),
		zap.Uint64("removed_user_id", removedUserID),
	)

	// 调用业务
	_, err := h.projectMemberService.RemoveProjectMember(c.Request.Context(), &service.RemoveProjectMemberParam{
		OperatorID:    operatorID,
		ProjectID:     projectID,
		RemovedUserID: removedUserID,
	})
	if err != nil {
		switch {
		// 不合法的参数
		case errors.Is(err, service.ErrInvalidProjectMemberParam):
			logger.Warn("remove project member rejected: invalid params")
			response.Fail(c, errmsg.InvalidParams)

		// 不合法的身份
		case errors.Is(err, service.ErrInvalidProjectMemberRole):
			logger.Warn("remove project member rejected: invalid project member role")
			response.Fail(c, errmsg.InvalidProjectMemberRole)

		// 项目不存在，统一处理成无权限
		case errors.Is(err, service.ErrProjectNotFound):
			logger.Warn("remove project member failed: project not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 项目成员不存在，统一处理成无权限
		case errors.Is(err, service.ErrProjectMemberNotFound):
			logger.Warn("remove project member failed: project member not found")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 没有操作权限
		case errors.Is(err, service.ErrProjectForbidden):
			logger.Warn("remove project member rejected: project forbidden")
			response.Fail(c, errmsg.ProjectNoPermission)

		// 其它错误
		default:
			logger.Error("remove project member failed", zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "remove project member failed")
		}
		return
	}

	logger.Info("remove project member success")
	response.Success(c)
}
