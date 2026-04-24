// Package service 存放业务处理的时候发生的错误变量
package service

import "errors"

var (
	ErrUsernameExists        = errors.New("username already exists")                           // 用户名已存在错误消息
	ErrEmailExists           = errors.New("email already exists")                              // 邮箱已存在错误消息
	ErrInvalidUsernameFormat = errors.New("invalid username")                                  // 无效的用户名错误消息
	ErrInvalidEmailFormat    = errors.New("invalid email")                                     // 无效的邮箱错误消息
	ErrInvalidPasswordFormat = errors.New("invalid password")                                  // 无效的密码错误消息
	ErrInvalidNicknameFormat = errors.New("invalid nickname")                                  // 无效的昵称错误消息
	ErrInvalidAccountFormat  = errors.New("invalid account")                                   // 无效的账户错误消息
	ErrUserDisabled          = errors.New("user is disabled")                                  // 用户被禁用错误消息
	ErrPasswordMismatch      = errors.New("password does not match")                           // 密码不匹配错误消息
	ErrUserNotFound          = errors.New("user not found")                                    // 用户未找到错误消息
	ErrInvalidToken          = errors.New("invalid token")                                     // 无效的 Token 错误消息
	ErrExpiredToken          = errors.New("refresh token expired")                             // 刷新令牌过期错误消息
	ErrOperationTooFrequent  = errors.New("operation is too frequent, please try again later") // 重试多次出现错误
	ErrInternal              = errors.New("internal server error")                             // 内部服务器错误消息
)
