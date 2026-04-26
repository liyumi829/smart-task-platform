// Package handler 处理器层，负责处理 HTTP 请求并调用服务层进行业务逻辑处理
package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"smart-task-platform/internal/api/contextx"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/pkg/errmsg"
	"smart-task-platform/internal/pkg/response"
	"smart-task-platform/internal/service"
)

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// UpdateUserProfile 更新个人资料（昵称、头像）
func (h *UserHandler) UpdateUserProfile(c *gin.Context) {
	var req dto.UpdateProfileReq

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 绑定请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind update user profile request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数解析成功
	userID := contextx.GetUserID(c)

	// 追加 user_id，避免重复传递
	logger = logger.With(zap.Uint64("user_id", userID))

	resp, err := h.userService.UpdateUserProfile(c.Request.Context(), userID, req.Nickname, req.Avatar)
	if err != nil {
		switch {
		// 昵称格式错误
		case errors.Is(err, service.ErrInvalidNicknameFormat):
			logger.Warn("update user profile rejected: invalid nickname format")
			response.Fail(c, errmsg.InvalidNicknameFormat)

		// 头像 URL 格式错误
		case errors.Is(err, service.ErrInvalidAvatarURLFormat):
			logger.Warn("update user profile rejected: invalid avatar url format")
			response.Fail(c, errmsg.InvalidAvatarURLFormat)

		// 用户没有找到
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("update user profile failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 用户被禁用
		case errors.Is(err, service.ErrUserDisabled):
			logger.Warn("update user profile failed: user disabled")
			response.Fail(c, errmsg.UserDisabled)

		// 其它错误
		default:
			logger.Error("update user profile failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "update user profile failed")
		}
		return
	}

	// 构造成功响应
	response.SuccessWithData(c, resp)
}

// UpdateUserPassword 修改登录密码
func (h *UserHandler) UpdateUserPassword(c *gin.Context) {
	var req dto.UpdateUserPasswordReq

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind update user password request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数解析成功，调用服务
	userID := contextx.GetUserID(c)

	// 追加 user_id，避免重复传递
	logger = logger.With(zap.Uint64("user_id", userID))

	_, err := h.userService.UpdateUserPassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword)
	if err != nil {
		switch {
		// 新旧密码格式不正确
		case errors.Is(err, service.ErrInvalidPasswordFormat):
			logger.Warn("update user password rejected: invalid password format")
			response.Fail(c, errmsg.InvalidPasswordFormat)

		// 用户没有找到
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("update user password failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 用户被禁用
		case errors.Is(err, service.ErrUserDisabled):
			logger.Warn("update user password failed: user disabled")
			response.Fail(c, errmsg.UserDisabled)

		// 输入的旧密码不正确
		case errors.Is(err, service.ErrOldPasswordMismatch):
			logger.Warn("update user password rejected: old password mismatch")
			response.Fail(c, errmsg.OldPasswordIncorrect)

		// 旧密码和新密码相同
		case errors.Is(err, service.ErrNewPasswordSameAsOld):
			logger.Warn("update user password rejected: new password same as old")
			response.Fail(c, errmsg.NewPasswordSameAsOld)

		// 其它错误
		default:
			logger.Error("update user password failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "update user password failed")
		}
		return
	}

	// 不用携带数据
	response.Success(c)
}

// GetUserPublicInfo 获取用户详情 / 公开信息
func (h *UserHandler) GetUserPublicInfo(c *gin.Context) {
	var req dto.UserIDParam

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindUri(&req); err != nil {
		logger.Warn("bind get user public info request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数正确
	logger = logger.With(zap.Uint64("target_user_id", req.ID))

	resp, err := h.userService.GetUserPublicInfo(c.Request.Context(), req.ID)
	if err != nil {
		switch {
		// 用户没有找到
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("get user public info failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 其它错误
		default:
			logger.Error("get user public info failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "get user public info failed")
		}
		return
	}

	// 正确获取
	response.SuccessWithData(c, resp)
}

// ListUsers 分页搜索用户列表
func (h *UserHandler) ListUsers(c *gin.Context) {
	var req dto.UserSearchListQuery

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindQuery(&req); err != nil {
		logger.Warn("bind search user list request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 参数解析成功
	logger = logger.With(
		zap.String("keyword", req.Keyword),
		zap.Int("page", req.Page),
		zap.Int("page_size", req.PageSize),
	)

	resp, err := h.userService.ListUsers(c.Request.Context(), req.Page, req.PageSize, req.Keyword)
	if err != nil {
		// 其它错误
		logger.Error("search user list failed",
			zap.Error(err),
		)
		response.FailWithMessage(c, errmsg.ServerError, "search user list failed")
		return
	}

	response.SuccessWithData(c, resp)
}
