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
type SearchResult[T interface{}] struct {
	List    []T    // 当前页任务
	Total   *int64 // 总数，未查询时为 nil
	HasMore bool   // 是否还有下一页
}

// 只查询用户基础字段（供 Preload 使用）
func SelectUserFields(db *gorm.DB) *gorm.DB {
	return db.Select(model.UserColumnID, model.UserColumnUsername, model.UserColumnNickname, model.UserColumnAvatar)
}

// 只查询项目基础字段（供 Preload 使用）
func SelectProjectFields(db *gorm.DB) *gorm.DB {
	return db.Select(model.ProjectColumnID, model.ProjectColumnName)
}
