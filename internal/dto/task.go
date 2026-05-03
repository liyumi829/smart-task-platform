// internal/dto/task.go
// 任务模块的数据传输对象
package dto

import "time"

// ====================
// 公共
// ====================

// TaskIDUri 任务 ID 路径参数
type TaskIDUri struct {
	TaskID    *uint64 `uri:"taskId" binding:"omitempty"`    // 任务 ID
	ProjectID *uint64 `uri:"projectId" binding:"omitempty"` // 项目ID
}

// TaskBaseFields 任务公共基础字段
type TaskBaseFields struct {
	ID         uint64     `json:"id"`
	ProjectID  uint64     `json:"project_id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Priority   string     `json:"priority"`
	AssigneeID *uint64    `json:"assignee_id"`
	DueDate    *time.Time `json:"due_date"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateTaskReq 创建任务请求
type CreateTaskReq struct {
	Title       string  `json:"title" binding:"required"`        // 标题
	Description *string `json:"description" binding:"omitempty"` // 描述
	Priority    string  `json:"priority" binding:"required"`     // 优先级
	AssigneeID  *uint64 `json:"assignee_id" binding:"omitempty"` // 负责人 ID
	DueDate     *string `json:"due_date" binding:"omitempty"`    // 截止时间
}

// CreateTaskResp 创建任务响应
type CreateTaskResp struct {
	*TaskBaseFields
	Description string             `json:"description"` // 描述
	Assignee    *UserPublicProfile `json:"assignee"`    // 负责人信息
	CreatorID   uint64             `json:"creator_id"`  // 创建者
	Creator     *UserPublicProfile `json:"creator"`     // 创建人信息
}

// 排序规则

const (
	SortByPriority   = "priority"
	SortByStatus     = "status"
	SortByTitle      = "title"
	SortByDueDate    = "due_date"
	SortByCreateTime = "created_at"
)

const (
	UpperAsc  = "ASC"
	UpperDesc = "DESC"
	LowerAsc  = "asc"
	LowerDesc = "desc"
)

// ListTasksQuery 查找任务列表请求
type ListTaskQuery struct {
	PageQuery
	Status    string  `form:"status" binding:"omitempty"`      // 状态
	Priority  string  `form:"priority" binding:"omitempty"`    // 优先级
	Keyword   string  `form:"keyword" binding:"omitempty"`     // 搜索关键词
	SortBy    string  `form:"sort_by" binding:"omitempty"`     // 排序字段
	SortOrder string  `form:"sort_order" binding:"omitempty"`  // 排序方向
	ProjectID *uint64 `form:"project_id" binding:"omitempty"`  // 任务ID
	AssigeeID *uint64 `form:"assignee_id" binding:"omitempty"` // 负责人。规定nil全量查找、0查找未分配
}

// ProjectTaskListItem 项目任务列表项
type ProjectTaskListItem struct {
	*TaskBaseFields
	Assignee *UserPublicProfile `json:"assignee"`
}

// ProjectTaskListResp 项目任务列表返回
type ProjectTaskListResp = PageResp[*ProjectTaskListItem]

// UserTaskListItem 我的任务列表项
type UserTaskListItem struct {
	*TaskBaseFields
	Project *ProjectPublicProfile `json:"project"`
}

// UserTaskListResp 我的任务列表返回
type UserTaskListResp = PageResp[*UserTaskListItem]

// GetTaskDetailReq 获取任务详情占位
type GetTaskDetailReq struct{}

// GetTaskDetailResp 获取任务详情响应
type GetTaskDetailResp struct {
	*TaskBaseFields
	Description string                `json:"description"` // 描述
	Project     *ProjectPublicProfile `json:"project"`     // 项目信息
	Assignee    *UserPublicProfile    `json:"assignee"`    // 负责人信息
	CreatorID   uint64                `json:"creator_id"`  // 创建者
	Creator     *UserPublicProfile    `json:"creator"`     // 创建人信息
}

// UpdateTaskBaseInfoReq 更新任务基础信息请求
type UpdateTaskBaseInfoReq struct {
	TiTle       string  `json:"title" binding:"omitempty"`
	Description *string `json:"description" binding:"omitempty"`
	Priority    string  `json:"priority" binding:"omitempty"`
	DueDate     *string `json:"due_date" binding:"omitempty"`
}

// UpdateTaskBaseInfoResp 更新任务基础信息响应
type UpdateTaskBaseInfoResp struct {
	*TaskBaseFields
	Description string     `json:"description"`
	CreatorID   uint64     `json:"creator_id"`
	StartTime   *time.Time `json:"start_time"`
	CompletedAt *time.Time `json:"completed_at"`
	SortOrder   int        `json:"sort_order"`
}

// UpdateTaskStatusReq 更新任务状态请求
type UpdateTaskStatusReq struct {
	Status string `json:"status" binding:"required"`
}

// UpdateTaskStatusResp 更新任务状态响应
type UpdateTaskStatusResp struct {
	ID          uint64     `json:"id"`
	ProjectID   uint64     `json:"project_id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	AssigneeID  *uint64    `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
	StartTime   *time.Time `json:"start_time"`
	CompletedAt *time.Time `json:"completed_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// UpdateTaskAssigneeReq 更新任务的负责人请求
type UpdateTaskAssigneeReq struct {
	AssigneeID *uint64 `json:"assignee_id" binding:"omitempty"`
}

// UpdateTaskAssigneeResp 更新任务的负责人响应
type UpdateTaskAssigneeResp struct {
	ID         uint64             `json:"id"`
	ProjectID  uint64             `json:"project_id"`
	Title      string             `json:"title"`
	Status     string             `json:"status"`
	Priority   string             `json:"priority"`
	AssigneeID *uint64            `json:"assignee_id"`
	Assignee   *UserPublicProfile `json:"assignee"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

// TaskSortItem 任务排序表项
type TaskSortItem struct {
	TaskID    uint64 `json:"task_id" binding:"required"`
	SortOrder int    `json:"sort_order" binding:"required"`
}

// UpdateTaskSortOrderReq 更新任务的排序值操作请求
type UpdateTaskSortOrderReq struct {
	Items []*TaskSortItem `json:"items" binding:"required"`
}

// UpdateTaskSortOrderResp 更新任务的排序值操作响应
type UpdateTaskSortOrderResp struct {
	Items     []*TaskSortItem `json:"items"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// RemoveTaskReq 删除任务的请求
type RemoveTaskReq struct{} // 占位

// RemoveTaskResp 删除任务的响应
type RemoveTaskResp struct{} // 占位
