package repository

import (
	"smart-task-platform/internal/model"

	"gorm.io/gorm"
)

// SearchQuery 公共的查询参数
type SearchQuery struct {
	Page      int
	PageSize  int
	NeedTotal bool // 是否需要统计总数，通常首次进入、刷新、筛选条件变化时为 true
}

// SearchResult 任务搜索结果。
type SearchResult[T any] struct {
	List    []T    // 当前页任务
	Total   *int64 // 总数，未查询时为 nil
	HasMore bool   // 是否还有下一页
}

func getUserSelectColumns() []string {
	return []string{
		model.UserColumnID,
		model.UserColumnUsername,
		model.UserColumnNickname,
		model.UserColumnAvatar,
	}
}

// 只查询用户基础字段（供 Preload 使用）
func SelectUserFields(db *gorm.DB) *gorm.DB {
	return db.Select(getUserSelectColumns())
}

func getProjectSelectColumns() []string {
	return []string{
		model.ProjectColumnID,
		model.ProjectColumnName,
		model.ProjectColumnStatus,
		model.ProjectColumnStartDate,
		model.ProjectColumnEndDate,
		model.ProjectColumnOwnerID,
	}
}

// 只查询项目基础字段（供 Preload 使用）
func SelectProjectFields(db *gorm.DB) *gorm.DB {
	return db.Select(getProjectSelectColumns())
}

// getTaskSelectColumns 返回任务基础查询字段（公共复用，避免重复）
func getTaskSelectColumns() []string {
	return []string{
		model.TaskColumnID,
		model.TaskColumnProjectID,
		model.TaskColumnTitle,
		model.TaskColumnCreatorID,
		model.TaskColumnAssigneeID,
		model.TaskColumnStatus,
		model.TaskColumnPriority,
		model.TaskColumnDueDate,
		model.TaskColumnStartTime,
		model.TaskColumnCompletedAt,
		model.TaskColumnUpdatedAt,
	}
}
