// Package dto common 模块的数据传输对象定义
package dto

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

// EmptyResp 空响应占位结构
type EmptyResp struct{}

// IDUri 通用资源 ID 路径参数
type IDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 资源 ID
}

// UserIDUri 用户 ID 路径参数
type UserIDUri struct {
	UserID uint64 `uri:"userId" binding:"required,min=1"` // 用户 ID
}

// TagIDUri 标签 ID 路径参数
type TagIDUri struct {
	TagID uint64 `uri:"tagId" binding:"required,min=1"` // 标签 ID
}

// ProjectIDUri 项目 ID 路径参数
type ProjectIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 项目 ID
}

// TaskIDUri 任务 ID 路径参数
type TaskIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 任务 ID
}

// CommentIDUri 评论 ID 路径参数
type CommentIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 评论 ID
}

// NotificationIDUri 通知 ID 路径参数
type NotificationIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 通知 ID
}
