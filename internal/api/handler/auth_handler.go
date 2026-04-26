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

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind register request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 注册请求解析成功，交给业务层处理业务
	resp, err := h.authService.Register(c.Request.Context(), &req)
	if err != nil {
		switch {
		// 用户名或邮箱已存在
		case errors.Is(err, service.ErrUsernameExists),
			errors.Is(err, service.ErrEmailExists):
			logger.Warn("register rejected: user already exists")
			response.Fail(c, errmsg.UserAlreadyExists)

		// 无效的用户名
		case errors.Is(err, service.ErrInvalidUsernameFormat):
			logger.Warn("register rejected: invalid username format")
			response.Fail(c, errmsg.InvalidUsernameFormat)

		// 无效的邮箱地址
		case errors.Is(err, service.ErrInvalidEmailFormat):
			logger.Warn("register rejected: invalid email format")
			response.Fail(c, errmsg.InvalidEmailFormat)

		// 无效的密码
		case errors.Is(err, service.ErrInvalidPasswordFormat):
			logger.Warn("register rejected: invalid password format")
			response.Fail(c, errmsg.InvalidPasswordFormat)

		// 无效的昵称
		case errors.Is(err, service.ErrInvalidNicknameFormat):
			logger.Warn("register rejected: invalid nickname format")
			response.Fail(c, errmsg.InvalidNicknameFormat)

		// 其他错误
		default:
			logger.Error("register failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "register failed")
		}
		return
	}

	response.SuccessWithData(c, resp)
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind login request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	// 调用登录模块
	resp, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		switch {
		// 用户不存在
		case errors.Is(err, service.ErrUserNotFound):
			logger.Warn("login failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 无效的账户，格式不正确
		case errors.Is(err, service.ErrInvalidAccountFormat):
			logger.Warn("login rejected: invalid account format")
			response.Fail(c, errmsg.InvalidAccountFormat)

		// 无效的密码，格式不正确
		case errors.Is(err, service.ErrInvalidPasswordFormat):
			logger.Warn("login rejected: invalid password format")
			response.Fail(c, errmsg.InvalidPasswordFormat)

		// 密码匹配错误
		case errors.Is(err, service.ErrPasswordMismatch):
			logger.Warn("login rejected: password mismatch")
			response.Fail(c, errmsg.PasswordIncorrect)

		// 用户被禁用
		case errors.Is(err, service.ErrUserDisabled):
			logger.Warn("login failed: user disabled")
			response.Fail(c, errmsg.UserDisabled)

		// 操作过于频繁
		case errors.Is(err, service.ErrOperationTooFrequent):
			logger.Warn("login rejected: operation too frequent")
			response.Fail(c, errmsg.TooManyRequest)

		// 其他错误
		default:
			logger.Error("login failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "login failed")
		}
		return
	}

	response.SuccessWithData(c, resp)
}

// Logout 退出登录，需要携带 Token
func (h *AuthHandler) Logout(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	claims := contextx.GetClaims(c)
	if claims.UserID == 0 {
		logger.Warn("logout rejected: unauthorized")
		response.Fail(c, errmsg.Unauthorized) // 用户未登录或无效的用户 ID
		return
	}

	// 追加 user_id
	logger = logger.With(zap.Uint64("user_id", claims.UserID))

	resp, err := h.authService.Logout(c.Request.Context(), claims)
	if err != nil {
		logger.Error("logout failed",
			zap.Error(err),
		)
		response.FailWithMessage(c, errmsg.ServerError, "logout failed")
		return
	}

	response.SuccessWithData(c, resp)
}

// Me 获取当前用户，需要携带 Token
func (h *AuthHandler) Me(c *gin.Context) {
	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	userID := contextx.GetUserID(c) // 从上下文中获取用户 ID
	if userID == 0 {
		logger.Warn("get current user rejected: unauthorized")
		response.Fail(c, errmsg.Unauthorized) // 用户未登录或无效的用户 ID
		return
	}

	// 追加 user_id
	logger = logger.With(zap.Uint64("user_id", userID))

	resp, err := h.authService.Me(c.Request.Context(), userID)
	if err != nil {
		switch {
		// 用户不存在
		case errors.Is(err, repository.ErrUserNotFound),
			errors.Is(err, service.ErrUserNotFound):
			logger.Warn("get current user failed: user not found")
			response.Fail(c, errmsg.UserNotFound)

		// 用户被禁用
		case errors.Is(err, service.ErrUserDisabled):
			logger.Warn("get current user failed: user disabled")
			response.Fail(c, errmsg.UserDisabled)

		// 其他错误
		default:
			logger.Error("get current user failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "get current user failed")
		}
		return
	}

	response.SuccessWithData(c, resp)
}

// RefreshToken 重新获取 Token，需要携带刷新 Token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req dto.RefreshTokenReq

	// 构造基础请求日志
	logger := zap.L().With(
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	// 参数绑定
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("bind refresh token request failed",
			zap.Error(err),
		)
		response.Fail(c, errmsg.InvalidParams) // 错误的参数
		return
	}

	resp, err := h.authService.RefreshToken(c.Request.Context(), &req)
	if err != nil {
		switch {
		// 无效的令牌
		case errors.Is(err, service.ErrInvalidToken):
			logger.Warn("refresh token rejected: invalid token")
			response.Fail(c, errmsg.RefreshTokenMissing)

		// 过期的令牌
		case errors.Is(err, service.ErrExpiredToken):
			logger.Warn("refresh token rejected: expired token")
			response.FailWithMessage(c, errmsg.RefreshTokenExpired, "Please log in again")

		// 会话不存在
		case errors.Is(err, service.ErrSessionNotExists):
			logger.Info("refresh token rejected: user logout")
			response.FailWithMessage(c, errmsg.Unauthorized, "Please log in again")

		default:
			logger.Error("refresh token failed",
				zap.Error(err),
			)
			response.FailWithMessage(c, errmsg.ServerError, "refresh token failed")
		}
		return
	}

	response.SuccessWithData(c, resp)
}
