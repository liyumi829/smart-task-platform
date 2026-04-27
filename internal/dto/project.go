// internal/dto/project.go
// Package dto project 模块的数据传输对象
package dto

import "time"

// ProjectIDUri 项目 ID 路径参数
type ProjectIDUri struct {
	ID uint64 `uri:"id" binding:"required,min=1"` // 项目 ID
}

// CreateProjectReq 创建项目请求
type CreateProjectReq struct {
	Name        string `json:"name" binding:"required"`         // 项目名称
	Description string `json:"description" binding:"omitempty"` // 项目描述
	StartDate   string `json:"start_date"`                      // 项目开始时间
	EndDate     string `json:"end_date"`                        // 项目结束时间
}

// CreateProjectResp 创建项目响应
type CreateProjectResp struct {
	ID          uint64             `json:"id"`                   // 项目 ID
	Name        string             `json:"name"`                 // 项目名称
	Description string             `json:"description"`          // 项目描述
	Status      string             `json:"status"`               // 项目状态
	StartDate   *time.Time         `json:"start_date,omitempty"` // 项目开始时间
	EndDate     *time.Time         `json:"end_date,omitempty"`   // 项目结束时间
	CreatedAt   time.Time          `json:"created_at"`           // 创建时间
	OwnerID     uint64             `json:"owner_id"`             // 创建人 / 拥有者 ID
	Owner       *UserPublicProfile `json:"owner"`                // 拥有者信息
}

// ProjectDetailReq 获取项目详情请求
// 保留以后扩展
type ProjectDetailReq struct{}

// ProjectDetailResp 获取项目详情响应
type ProjectDetailResp struct {
	ID          uint64             `json:"id"`                   // 项目 ID
	Name        string             `json:"name"`                 // 项目名称
	Description string             `json:"description"`          // 项目描述
	Status      string             `json:"status"`               // 项目状态
	StartDate   *time.Time         `json:"start_date,omitempty"` // 项目开始时间
	EndDate     *time.Time         `json:"end_date,omitempty"`   // 项目结束时间
	CreatedAt   time.Time          `json:"created_at"`           // 创建时间
	UpdatedAt   time.Time          `json:"updated_at"`           // 更新时间
	OwnerID     uint64             `json:"owner_id"`             // 拥有者 ID
	Owner       *UserPublicProfile `json:"owner"`                // 拥有者信息
}

// ProjectListQuery 项目列表查询
type ProjectListQuery struct {
	PageQuery
	Status  string `form:"status" binding:"omitempty"` // 状态筛选
	Keyword string `form:"keyword"`                    // 关键字搜索
}

// ProjectListItem 项目列表项
type ProjectListItem struct {
	ID        uint64             `json:"id"`                   // 项目 ID
	Name      string             `json:"name"`                 // 项目名称
	Status    string             `json:"status"`               // 项目状态
	StartDate *time.Time         `json:"start_date,omitempty"` // 项目开始时间
	EndDate   *time.Time         `json:"end_date,omitempty"`   // 项目结束时间
	OwnerID   uint64             `json:"owner_id"`             // 拥有者 ID
	Owner     *UserPublicProfile `json:"owner"`                // 拥有者信息
}

// ProjectListResp 项目列表响应
type ProjectListResp struct {
	List     []*ProjectListItem `json:"list"`      // 项目列表
	Total    int                `json:"total"`     // 总数
	Page     int                `json:"page"`      // 当前页
	PageSize int                `json:"page_size"` // 每页条数
}

// UpdateProjectReq 更新项目请求
type UpdateProjectReq struct {
	Name        string `json:"name" binding:"omitempty"`        // 项目名称
	Description string `json:"description" binding:"omitempty"` // 项目描述
	Status      string `json:"status" binding:"omitempty"`      // 项目状态
	StartDate   string `json:"start_date"`                      // 项目开始时间
	EndDate     string `json:"end_date"`                        // 项目结束时间
}

// UpdateProjectResp 更新项目响应
type UpdateProjectResp struct {
	ID          uint64             `json:"id"`                   // 项目 ID
	Name        string             `json:"name"`                 // 项目名称
	Description string             `json:"description"`          // 项目描述
	Status      string             `json:"status"`               // 项目状态
	StartDate   *time.Time         `json:"start_date,omitempty"` // 项目开始时间
	EndDate     *time.Time         `json:"end_date,omitempty"`   // 项目结束时间
	UpdatedAt   time.Time          `json:"updated_at"`           // 更新时间
	OwnerID     uint64             `json:"owner_id"`             // 拥有者 ID
	Owner       *UserPublicProfile `json:"owner"`                // 拥有者信息
}

// ArchiveProjectReq 归档项目请求
// 当前接口无请求体，这里保留空结构体，便于统一接口风格与后续扩展
type ArchiveProjectReq struct{}

// ArchiveProjectResp 归档项目响应
type ArchiveProjectResp struct {
	ID     uint64 `json:"id"`     // 项目 ID
	Status string `json:"status"` // 项目状态
}
