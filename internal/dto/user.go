// Package dto
// 用户模块的数据传输对象
package dto

// UserIDParam 用户ID路径参数
type UserIDParam struct {
	ID uint64 `uri:"userId" binding:"required,min=1"`
}

// UpdateProfileReq 更新个人资料请求
type UpdateProfileReq struct {
	Nickname string `json:"nickname" binding:"omitempty,max=32"` // 昵称
	Avatar   string `json:"avatar" binding:"omitempty,max=255"`  // 头像地址
}

// UpdateProfileResp 更新个人资料响应
type UpdateProfileResp struct {
	ID       uint64 `json:"id"`               // 用户 ID
	Username string `json:"username"`         // 用户名
	Nickname string `json:"nickname"`         // 昵称
	Avatar   string `json:"avatar,omitempty"` // 头像
}

// UpdateUserPasswordReq 修改密码请求
type UpdateUserPasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"` // 旧密码
	NewPassword string `json:"new_password" binding:"required"` // 新密码
}

// UpdateUserPasswordResp 修改密码的响应
// 空
type UpdateUserPasswordResp struct{}

// UserPublicInfoResp 用户公开信息响应
type UserPublicProfileResp struct {
	UserPublicProfile // 共有信息
}

// UserSearchListQuery 用户搜索查询查询请求
type UserSearchListQuery struct {
	PageQuery
	Keyword string `form:"keyword" binding:"required,max=16"` // 搜索关键词
}

// UserListItem 用户搜索列表项
type UserListItem struct {
	ID       uint64 `json:"id"`               // 用户 ID
	Username string `json:"username"`         // 用户名
	Nickname string `json:"nickname"`         // 昵称
	Avatar   string `json:"avatar,omitempty"` // 头像
}

// UserSearchListResp 用户搜索查询响应
type UserSearchListResp = PageResp[*UserListItem]
