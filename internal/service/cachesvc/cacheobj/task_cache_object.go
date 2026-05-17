// internal/service/cachesvc/cacheobj/task_cache_object.go
// Package cacheobj 定义任务模块缓存对象。
package cacheobj

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	"strings"
	"time"
)

// TaskPermissionInfo 任务权限判定缓存对象。
// 注意：该缓存只用于权限判断、删除任务前校验等只读场景，不能用于更新任务。
type TaskPermissionInfo struct {
	ID         uint64  `json:"id"`
	ProjectID  uint64  `json:"project_id"`
	CreatorID  uint64  `json:"creator_id"`
	AssigneeID *uint64 `json:"assignee_id"`
	Status     string  `json:"status"`
}

// ToModel 将任务权限判定缓存对象转换为 model.Task。
func (info *TaskPermissionInfo) ToModel() *model.Task {
	if info == nil || info.ID == 0 {
		return nil
	}

	return &model.Task{
		ID:         info.ID,
		ProjectID:  info.ProjectID,
		CreatorID:  info.CreatorID,
		AssigneeID: info.AssigneeID,
		Status:     info.Status,
	}
}

// TaskDetailInfo 任务详细信息
type TaskDetailInfo struct {
	ID          uint64     `json:"id"`
	ProjectID   uint64     `json:"project_id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	AssigneeID  *uint64    `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Description string     `json:"description"` // 描述
	CreatorID   uint64     `json:"creator_id"`  // 创建者
}

// ToModel 将任务权限判定缓存对象转换为 model.Task。
func (task *TaskDetailInfo) ToModel() *model.Task {
	if task == nil || task.ID == 0 {
		return nil
	}

	return &model.Task{
		ID:          task.ID,
		ProjectID:   task.ProjectID,
		Title:       task.Title,
		Status:      task.Status,
		Priority:    task.Priority,
		AssigneeID:  task.AssigneeID,
		DueDate:     task.DueDate,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
		Description: utils.StringPtr(task.Description),
		CreatorID:   task.CreatorID,
	}
}

// TaskListItem 项目任务列表缓存表项
type TaskListItem struct {
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

// ToModel 将任务权限判定缓存对象转换为 model.Task。
func (item *TaskListItem) ToModel() *model.Task {
	if item == nil || item.ID == 0 {
		return nil
	}

	return &model.Task{
		ID:         item.ID,
		ProjectID:  item.ProjectID,
		Title:      item.Title,
		Status:     item.Status,
		Priority:   item.Priority,
		AssigneeID: item.AssigneeID,
		DueDate:    item.DueDate,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}

// TaskListPage 任务列表缓存结构
type TaskListPage = ListPage

// TaskListQuery 项目任务列表查询参数。
// 说明：
//  1. 这是 cache 层专用查询结构，和 service 入参解耦。
//  2. 只要 CacheKeyParts 稳定，key 就稳定。
type TaskListQuery struct {
	ProjectID  uint64
	AssigneeID *uint64
	Page       int
	PageSize   int
	NeedTotal  bool

	Keyword   string
	Status    string
	Priority  string
	SortBy    string
	SortOrder string
}

// 进行深拷贝
func (q TaskListQuery) ToRepositoryQuery() *repository.TaskSearchQuery {
	return &repository.TaskSearchQuery{
		Page:       q.Page,
		PageSize:   q.PageSize,
		NeedTotal:  q.NeedTotal,
		ProjectID:  q.ProjectID,
		AssigneeID: q.AssigneeID,
		Keyword:    q.Keyword,
		Status:     q.Status,
		Priority:   q.Priority,
		SortBy:     q.SortBy,
		SortOrder:  q.SortOrder,
	}
}

// CacheKeyParts 输出稳定的 key 片段。
// 说明：
// - keyword 做 hash，避免 key 过长
// - 其它字段统一做 normalize
func (q *TaskListQuery) CacheKeyParts() []string {
	if q == nil {
		return nil
	}

	return []string{
		fmt.Sprintf("project=%d", q.ProjectID),
		fmt.Sprintf("assignee=%d", utils.SafeValue(q.AssigneeID)),
		fmt.Sprintf("page=%d", q.Page),
		fmt.Sprintf("size=%d", q.PageSize),
		fmt.Sprintf("need_total=%t", q.NeedTotal),
		"keyword=" + hashKeyword(q.Keyword),
		"status=" + normalizeKeyPart(q.Status),
		"priority=" + normalizeKeyPart(q.Priority),
		"sortby=" + normalizeKeyPart(q.SortBy),
		"order=" + normalizeKeyPart(q.SortOrder),
	}
}

func normalizeKeyPart(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "0"
	}
	return s
}

func hashKeyword(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "0"
	}

	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
