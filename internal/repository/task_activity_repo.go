// internal/repository/task_activity_repo.go
// Package repository
// 实现 task_activities 表的仓储操作

package repository

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"

	"gorm.io/gorm"
)

var (
	ErrInvalidTaskActivityParam = errors.New("invalid task activity param")          // 不合法的任务动态参数
	ErrCreateTaskActivityEmpty  = errors.New("create task activity params is empty") // 创建任务动态参数为空
)

// taskActivityRepository 任务动态仓储
type taskActivityRepository struct {
	db *gorm.DB
}

// NewTaskActivityRepository 创建任务动态仓储
func NewTaskActivityRepository(db *gorm.DB) *taskActivityRepository {
	return &taskActivityRepository{
		db: db,
	}
}

// CreateWithTx 在任务动态表中插入一条数据
//
// 用于业务事务：创建任务动态 + 创建通知 + 创建 outbox message
func (r *taskActivityRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, activity *model.TaskActivity) error {
	if activity == nil {
		return ErrCreateTaskActivityEmpty
	}

	return getDB(ctx, r.db, tx).Create(activity).Error
}

// SearchTaskActivitiesQuery 任务动态列表查询参数
type SearchTaskActivitiesQuery struct {
	TaskID uint64 // 任务ID
	SearchQuery
}

// SearchTaskActivitiesResult 任务动态搜索结果
type SearchTaskActivitiesResult = SearchResult[*model.TaskActivity]

// SearchTaskActivities 查询任务动态列表
//
// 用于接口：GET /api/v1/tasks/:id/activities
func (r *taskActivityRepository) SearchTaskActivities(ctx context.Context, query *SearchTaskActivitiesQuery) (*SearchTaskActivitiesResult, error) {
	if query == nil || query.TaskID == 0 {
		return nil, ErrInvalidTaskActivityParam
	}

	db := getDB(ctx, r.db, nil).
		Model(&model.TaskActivity{}).
		Where(model.TaskActivityColumnTaskID+" = ?", query.TaskID)

	result := &SearchTaskActivitiesResult{
		List:    []*model.TaskActivity{},
		Total:   nil,
		HasMore: false,
	}

	// 只有 need_total=true 时才执行 COUNT(*)
	if query.NeedTotal {
		var total int64
		if err := db.Count(&total).Error; err != nil {
			return nil, err
		}

		result.Total = &total
		if total == 0 {
			return result, nil
		}
	}

	offset := (query.Page - 1) * query.PageSize
	limit := query.PageSize + 1 // 多查一条判断是否还有下一页

	activities := make([]*model.TaskActivity, 0, query.PageSize)

	err := db.
		Preload(model.ActivityAssocOperator, getUserSelectColumns()).
		Order(model.TaskActivityColumnCreatedAt + " DESC").
		Order(model.TaskActivityColumnID + " DESC").
		Offset(offset).
		Limit(limit).
		Find(&activities).Error
	if err != nil {
		return nil, err
	}

	if len(activities) > query.PageSize {
		result.HasMore = true
		activities = activities[:query.PageSize]
	}

	result.List = activities

	return result, nil
}
