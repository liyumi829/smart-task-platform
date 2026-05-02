// Package dto common 模块的数据传输对象定义
package dto

const (
	MinPageSize = 10
	MaxPageSize = 50
)

// UserSummary 通用用户摘要信息
type UserSummary struct {
	ID       uint64 `json:"id"`               // 用户 ID
	Username string `json:"username"`         // 用户名
	Nickname string `json:"nickname"`         // 昵称
	Avatar   string `json:"avatar,omitempty"` // 头像
	Email    string `json:"email,omitempty"`  // 邮箱
}

// UserPublicProfile 用户公开资料
type UserPublicProfile struct {
	ID       uint64 `json:"id"`               // 用户 ID
	Username string `json:"username"`         // 用户名
	Nickname string `json:"nickname"`         // 昵称
	Avatar   string `json:"avatar,omitempty"` // 头像
}

// ProjectPublicProfile 项目公开资料
type ProjectPublicProfile struct {
	ID   uint64 `json:"id"`   // 项目ID
	Name string `json:"name"` // 项目名称
}

// PageQuery 通用分页查询参数
type PageQuery struct {
	Page     int `form:"page" binding:"omitempty"`      // 页码
	PageSize int `form:"page_size" binding:"omitempty"` // 每页数量
}

// PageResp 页面泛型通用响应
type PageResp[T interface{}] struct {
	List     []T `json:"list"`      // 项目列表
	Total    int `json:"total"`     // 总数
	Page     int `json:"page"`      // 当前页
	PageSize int `json:"page_size"` // 每页条数
}

// CommentIDUri 评论 ID 路径参数
type CommentIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 评论 ID
}

// NotificationIDUri 通知 ID 路径参数
type NotificationIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 通知 ID
}
