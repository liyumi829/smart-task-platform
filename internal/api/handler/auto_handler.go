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
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Register 注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Error("failed to bind register request",
			zap.Error(err))
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 注册请求解析成功 交给业务层处理业务
	resp, err := h.authService.Register(c.Request.Context(), &req)
	if err != nil {
		switch {
		// 用户名和用户邮箱已存在 -> 用户存在
		case errors.Is(err, service.ErrUsernameExists),
			errors.Is(err, service.ErrEmailExists):
			response.Fail(c, errmsg.UserAlreadyExists) // 返回用户存在信息
		// 无效的用户名
		case errors.Is(err, service.ErrInvalidUsernameFormat):
			response.Fail(c, errmsg.InvalidUsernameFormat) // 返回无效参数错误
		// 无效的邮箱地址
		case errors.Is(err, service.ErrInvalidEmailFormat):
			response.Fail(c, errmsg.InvalidEmailFormat) // 返回无效参数错误
		// 无效的密码
		case errors.Is(err, service.ErrInvalidPasswordFormat):
			response.Fail(c, errmsg.InvalidPasswordFormat) // 返回无效参数错误
		// 无效的昵称
		case errors.Is(err, service.ErrInvalidNicknameFormat):
			response.Fail(c, errmsg.InvalidNicknameFormat) // 返回无效参数错误
		// 其他错误
		default:
			zap.L().Error("failed to register user",
				zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "registration failed") // 返回注册失败信息
		}
		return
	}

	response.SuccessWithData(c, resp)
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Error("failed to bind login request",
			zap.Error(err))
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 调用登录模块
	resp, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		switch {
		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			response.Fail(c, errmsg.UserNotFound)
		// 无效的账户 格式不正确
		case errors.Is(err, service.ErrInvalidAccountFormat):
			response.Fail(c, errmsg.InvalidAccountFormat) // 返回无效账户错误
		// 无效的密码 格式不正确
		case errors.Is(err, service.ErrInvalidPasswordFormat):
			response.Fail(c, errmsg.InvalidPasswordFormat) // 返回无效密码错误
		// 密码匹配错误
		case errors.Is(err, service.ErrPasswordMismatch):
			response.Fail(c, errmsg.PasswordIncorrect)
		// 用户被禁用
		case errors.Is(err, service.ErrUserDisabled):
			response.Fail(c, errmsg.UserDisabled)
		// 其他错误
		default:
			zap.L().Error("failed to login user",
				zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "login failed")
		}
		return
	}

	response.SuccessWithData(c, resp)
}

// Logout 退出登录 需要携带 Token
func (h *AuthHandler) Logout(c *gin.Context) {
	userID := contextx.GetUserID(c) // 从上下文中获取用户ID
	if userID == 0 {
		zap.L().Warn("unauthorized access to current user info")
		response.Fail(c, errmsg.Unauthorized) // 用户未登录或无效的用户ID
		return
	}

	resp, err := h.authService.Logout(c.Request.Context(), userID)
	if err != nil {
		zap.L().Error("failed to logout user",
			zap.Error(err))
		response.FailWithMessage(c, errmsg.ServerError, "logout failed")
		return
	}

	response.SuccessWithData(c, resp)
}

// Me 获取当前用户 需要携带 Token
func (h *AuthHandler) Me(c *gin.Context) {
	userID := contextx.GetUserID(c) // 从上下文中获取用户ID
	if userID == 0 {
		zap.L().Warn("unauthorized access to current user info")
		response.Fail(c, errmsg.Unauthorized) // 用户未登录或无效的用户ID

		return
	}

	resp, err := h.authService.Me(c.Request.Context(), userID)
	if err != nil {
		switch {
		// 用户不存在
		case errors.Is(err, repository.ErrUserNotFound):
			response.Fail(c, errmsg.UserNotFound)
		// 用户被禁用
		case errors.Is(err, service.ErrUserDisabled):
			response.Fail(c, errmsg.UserDisabled)
		// 其他错误
		default:
			zap.L().Error("failed to get current user info",
				zap.Error(err))

			response.FailWithMessage(c, errmsg.ServerError, "get current user failed")
		}
		return
	}

	response.SuccessWithData(c, resp)
}

// RefreshToken 重新获取 Token 需要携带刷新 Token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// 1、参数绑定
	var req dto.RefreshTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Error("failed to bind refresh token request",
			zap.Error(err))
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	resp, err := h.authService.RefreshToken(c.Request.Context(), &req)
	if err != nil {
		// 判断错误类型
		switch {
		// 无效的令牌
		case errors.Is(err, service.ErrInvalidToken):
			response.Fail(c, errmsg.RefreshTokenMissing) // 刷新令牌无效，返回无效令牌错误
		// 过期的令牌
		case errors.Is(err, service.ErrExpiredToken):
			response.Fail(c, errmsg.RefreshTokenExpired) // 刷新令牌过期，返回 Token 已过期错误
		default:
			zap.L().Error("failed to refresh token",
				zap.Error(err))
			response.FailWithMessage(c, errmsg.ServerError, "refresh token failed") // 刷新 Token 失败
		}
		return
	}
	response.SuccessWithData(c, resp)
}
