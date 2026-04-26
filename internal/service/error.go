// Package service 存放业务处理的时候发生的错误变量
package service

import "errors"

var (
	ErrOperationTooFrequent = errors.New("operation is too frequent, please try again later") // 重试多次出现错误
	ErrInternal             = errors.New("internal server error")                             // 内部服务器错误消息
	ErrUserNotFound         = errors.New("user not found")                                    // 用户未找到错误消息

	ErrUsernameExists         = errors.New("username already exists")   // 用户名已存在错误消息
	ErrSessionNotExists       = errors.New("session not exists")        // 会话不存在（用户退出登录）
	ErrEmailExists            = errors.New("email already exists")      // 邮箱已存在错误消息
	ErrInvalidUsernameFormat  = errors.New("invalid username format")   // 无效的用户名错误消息
	ErrInvalidEmailFormat     = errors.New("invalid email format")      // 无效的邮箱错误消息
	ErrInvalidPasswordFormat  = errors.New("invalid password format")   // 无效的密码错误消息
	ErrInvalidNicknameFormat  = errors.New("invalid nickname format")   // 无效的昵称错误消息
	ErrInvalidAccountFormat   = errors.New("invalid account format")    // 无效的账户错误消息
	ErrInvalidAvatarURLFormat = errors.New("invalid avatar url format") // 无效的头像url错误消息
	ErrUserDisabled           = errors.New("user is disabled")          // 用户被禁用错误消息
	ErrPasswordMismatch       = errors.New("password does not match")   // 密码不匹配错误消息
	ErrInvalidToken           = errors.New("invalid token")             // 无效的 Token 错误消息
	ErrExpiredToken           = errors.New("refresh token expired")     // 刷新令牌过期错误消息

	ErrOldPasswordMismatch  = errors.New("old password is incorrect")                       // 输入的旧密码不正确
	ErrNewPasswordSameAsOld = errors.New("new password cannot be the same as old password") // 新密码和旧密码相同
)
