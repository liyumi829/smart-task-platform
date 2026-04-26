// Package dto auth 模块的数据传输对象定义
// 该文件定义了 auth 模块相关的请求和响应结构体，用于数据传输和接口交互。
package dto

// RegisterReq 用户注册请求
type RegisterReq struct {
	Username string `json:"username" binding:"required"`  // 用户名
	Email    string `json:"email" binding:"required"`     // 邮箱
	Password string `json:"password" binding:"required"`  // 密码
	Nickname string `json:"nickname" binding:"omitempty"` // 昵称
}

// RegisterResp 用户注册响应
type RegisterResp struct {
	ID       uint64 `json:"id"`       // 用户 ID
	Username string `json:"username"` // 用户名
	Email    string `json:"email"`    // 邮箱
	Nickname string `json:"nickname"` // 昵称
}

// LoginReq 用户登录请求
type LoginReq struct {
	Account  string `json:"account" binding:"required"`  // 用户名或邮箱
	Password string `json:"password" binding:"required"` // 密码
}

// LoginResp 用户登录响应
type LoginResp struct {
	AccessToken  string      `json:"access_token"`  // 访问令牌
	RefreshToken string      `json:"refresh_token"` // 刷新令牌
	TokenType    string      `json:"token_type"`    // Bearer
	ExpiresIn    int64       `json:"expires_in"`    // 访问过期秒数
	User         UserSummary `json:"user"`          // 用户摘要
}

// LogoutReq 退出登录请求
// 空结构体，因为退出登录不需要额外的参数
type LogoutReq struct{}

// LogoutResp 退出登录响应
type LogoutResp struct {
	Logout bool `json:"logged_out"` // 是否成功退出登录
}

// MeResp 当前登录用户信息响应
type MeResp struct {
	ID       uint64 `json:"id"`               // 用户 ID
	Username string `json:"username"`         // 用户名
	Nickname string `json:"nickname"`         // 昵称
	Email    string `json:"email"`            // 邮箱
	Avatar   string `json:"avatar,omitempty"` // 头像
}

// 重新获取 Token 请求
type RefreshTokenReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"` // 刷新令牌
}

// 重新获取 Token 响应
type RefreshTokenResp struct {
	AccessToken  string `json:"access_token"`  // 新的访问令牌
	RefreshToken string `json:"refresh_token"` // 新的刷新令牌
	TokenType    string `json:"token_type"`    // Bearer
	ExpiresIn    int64  `json:"expires_in"`    // 过期秒数
}
