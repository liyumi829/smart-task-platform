// internal/dto/project_member.go
// Package dto project member模块的数据传输对象
package dto

import "time"

// ProjectMemberIDUri 项目 ID 路径参数
type ProjectMemberIDUri struct {
	ID     uint64 `uri:"id" binding:"required,min=1"`      // 项目 ID
	UserID uint64 `uri:"userId" binding:"omitempty,min=1"` // 用户ID
}

// AddProjectMemberReq 添加项目成员请求
type AddProjectMemberReq struct {
	UserID uint64 `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

// AddProjectMemberResp 添加项目成员响应
type AddProjectMemberResp struct {
	ProjectID uint64             `json:"project_id"`
	UserID    uint64             `json:"user_id"`
	Role      string             `json:"role"`
	User      *UserPublicProfile `json:"user"`
	JoinedAt  time.Time          `json:"joined_at"`
}

// ProjectMemberListQuery 项目成员列表请求
type ProjectMemberListQuery struct {
	PageQuery
	Role    string `form:"role" binding:"omitempty"`    // 状态筛选
	Keyword string `form:"keyword" binding:"omitempty"` // 关键字搜索
}

// ProjectMemberItem 项目成员列表项响应
type ProjectMemberListItem struct {
	ProjectID uint64             `json:"project_id"`
	UserID    uint64             `json:"user_id"`
	Role      string             `json:"role"`
	User      *UserPublicProfile `json:"user"`
	JoinedAt  time.Time          `json:"joined_at"`
}

// ProjectMemberListResp 项目成员列表响应
type ProjectMemberListResp struct {
	List     []*ProjectMemberListItem `json:"list"`
	Total    int                      `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

// UpdateProjectMemberReq 更新项目成员属性
type UpdateProjectMemberReq struct {
	Role *string `json:"role" binding:"omitempty"` // 不传就什么也不改
}

// UpdateProjectMemberResp 修改项目成员属性响应
type UpdateProjectMemberResp struct {
	ProjectID uint64             `json:"project_id"`
	UserID    uint64             `json:"user_id"`
	Role      string             `json:"role"`
	User      *UserPublicProfile `json:"user"`
}

// RemoveProjectMemberReq 移除项目成员请求
type RemoveProjectMemberReq struct{}

// RemoveProjectMemberResp 移除项目成员响应
type RemoveProjectMemberResp struct{}
