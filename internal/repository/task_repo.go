// internal/repository/taks_repo.go
// Package repository
// 实现 tasks 表的仓储操作

package repository

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils/repoutil"
	"time"

	"gorm.io/gorm"
)

var (
	ErrInvalidTaskParam      = errors.New("invalid task param")          // 不合法的任务参数
	ErrCreateTaskIsEmpty     = errors.New("create task params is empty") // 创建任务参数为空
	ErrUpdateParamIsEmpty    = errors.New("update task params is empty") // 更新任务参数为空
	ErrTaskNotFound          = errors.New("task not found")              // 任务不存在/未找到
	ErrSearchTaskQueryEmpty  = errors.New("search task query is empty")  // 任务搜索条件为空
	ErrTaskSortItemsIsEmpty  = errors.New("task sort items is empty")    // 排序项列表为空
	ErrTaskSortNoRowsUpdated = errors.New("task sort no rows updated")   // 批量更新排序无数据受影响
)

// taskRepository 任务仓储
type taskRepository struct {
	db *gorm.DB
}

// NewTaskRepository 创建任务仓储
func NewTaskRepository(db *gorm.DB) *taskRepository {
	return &taskRepository{
		db: db,
	}
}

// CreateWithTx 在任务表中插入一条数据
//
// 必要字段: ProjectID、Title、Description、Status、Priority、CreatorID
//
// 可选字段(可为空): AssigneeID、DueDate、StartTime
func (r *taskRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, task *model.Task) error {
	if task == nil {
		return ErrCreateTaskIsEmpty
	}
	return getDB(ctx, r.db, tx).Create(task).Error
}

// SoftDeleteTaskByIDWithTx 软删除任务
//
// 用于接口：DELETE /api/v1/tasks/:id
func (r *taskRepository) SoftDeleteTaskByIDWithTx(ctx context.Context, tx *gorm.DB, taskID uint64) error {
	if taskID == 0 {
		return ErrInvalidTaskParam
	}

	now := time.Now()
	result := getDB(ctx, r.db, tx).
		Model(&model.Task{}).
		Where(model.TaskColumnID+" = ?", taskID).
		Where(model.TaskColumnDeletedAt + " IS NULL").
		Updates(map[string]interface{}{
			model.TaskColumnDeletedAt: now,
			model.TaskColumnUpdatedAt: now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}

	return nil
}

// UpdateTaskParam 业务层允许更新的字段
//
// 普通字段使用指针：nil 表示不更新，非 nil 表示更新。
//
// 可空字段使用 UpdateXXX + 指针：
//
//   - UpdateXXX = false 表示不更新该字段。
//   - UpdateXXX = true 且字段为 nil 表示更新为 NULL。
//   - UpdateXXX = true 且字段非 nil 表示更新为对应值。
type UpdateTaskParam struct {
	// 接口：PATCH /api/v1/tasks/:id
	Title             *string
	Priority          *string
	UpdateDescription bool // description 是可控字段
	Description       *string
	UpdateDueDate     bool // due_date 是可空字段
	DueDate           *time.Time

	// 接口：PATCH /api/v1/tasks/:id/status
	Status            *string
	UpdateStartTime   bool // start_time 为可控字段
	StartTime         *time.Time
	UpdateCompletedAt bool // completed_at 是可空字段
	CompletedAt       *time.Time

	// 接口：PATCH /api/v1/tasks/:id/assignee
	UpdateAssignee bool // assignee_id 是可空字段
	AssigneeID     *uint64
}

// UpdateTaskByIDWithTx 在任务表中更新单条数据
func (r *taskRepository) UpdateTaskByIDWithTx(ctx context.Context, tx *gorm.DB, taskID uint64, param *UpdateTaskParam) error {
	if param == nil {
		return ErrUpdateParamIsEmpty
	}

	updates := make(map[string]interface{})

	// 更新标题
	if param.Title != nil {
		updates[model.TaskColumnTitle] = *param.Title
	}

	// 更新或者清空描述
	if param.UpdateDescription {
		if param.Description == nil {
			updates[model.TaskColumnDescription] = nil
		} else {
			updates[model.TaskColumnDescription] = *param.Description
		}
	}

	// 更新优先级
	if param.Priority != nil {
		updates[model.TaskColumnPriority] = *param.Priority
	}

	// 更新或清空截止时间
	if param.UpdateDueDate {
		if param.DueDate == nil {
			updates[model.TaskColumnDueDate] = nil
		} else {
			updates[model.TaskColumnDueDate] = *param.DueDate
		}
	}

	// 更新任务状态
	if param.Status != nil {
		updates[model.TaskColumnStatus] = *param.Status
	}

	// 更新或者清空开启时间
	if param.UpdateStartTime {
		if param.StartTime == nil {
			updates[model.TaskColumnStartTime] = nil
		} else {
			updates[model.TaskColumnStartTime] = *param.StartTime
		}
	}

	// 更新或清空完成时间
	if param.UpdateCompletedAt {
		if param.CompletedAt == nil {
			updates[model.TaskColumnCompletedAt] = nil
		} else {
			updates[model.TaskColumnCompletedAt] = *param.CompletedAt
		}
	}

	// 更新或取消负责人
	if param.UpdateAssignee {
		if param.AssigneeID == nil {
			updates[model.TaskColumnAssigneeID] = nil
		} else {
			updates[model.TaskColumnAssigneeID] = *param.AssigneeID
		}
	}

	if len(updates) == 0 {
		return ErrUpdateParamIsEmpty
	}

	// 统一更新时间
	updates[model.TaskColumnUpdatedAt] = time.Now()

	result := getDB(ctx, r.db, tx).
		Model(&model.Task{}).
		Where(model.TaskColumnID+" = ?", taskID).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}

	return nil
}

// GetTaskByID 获取任务详情
func (r *taskRepository) GetTaskByID(ctx context.Context, taskID uint64) (*model.Task, error) {
	if taskID == 0 {
		return nil, ErrInvalidTaskParam
	}

	var task model.Task
	err := getDB(ctx, r.db, nil).Preload(model.TaskAssocProject).
		Preload(model.TaskAssocAssignee).
		Preload(model.TaskAssocCreator).
		Model(&model.Task{}).
		Where(model.TaskColumnID+" = ?", taskID).
		First(&task).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// TaskSearchQuery 项目成员列表查询参数
//
// 复用，确定assignee_id即可
// 接口：GET /api/v1/projects/:projectId/tasks
// 接口：GET /api/v1/tasks/my
type TaskSearchQuery struct {
	Page       int     // 页码
	PageSize   int     // 每页数量
	ProjectID  uint64  // 项目 ID（0表示查全量，其它表示查指定的项目ID）
	AssigneeID *uint64 // 负责人ID （用指针表示空，全量查询，0 表示查询没有分配的）
	Keyword    string  // 关键字
	Status     string  // 状态
	Priority   string  // 优先级
	SortBy     string  // 排序规则
	SortOrder  string  // 排序顺序（业务处理：要求全部小写）
}

// SearchTasks 按照规则批量查询任务
func (r *taskRepository) SearchTasks(ctx context.Context, query *TaskSearchQuery) ([]*model.Task, int64, error) {
	if query == nil {
		return nil, 0, ErrInvalidTaskParam
	}
	// 完全相信上层参数
	db := getDB(ctx, r.db, nil).
		Model(&model.Task{})

	if query.ProjectID != 0 { // 如果 projectID 不等于0，说明筛选条件
		db = db.Where(model.TaskColumnProjectID+" = ?", query.ProjectID)
	}

	if query.AssigneeID != nil {
		if *query.AssigneeID == 0 {
			// 查询未分配
			db = db.Where(model.TaskColumnAssigneeID + " IS NULL")
		} else {
			// 查询指定负责人
			db = db.Where(model.TaskColumnAssigneeID+" = ?", *query.AssigneeID)
		}
	} // else 为空不查询 不设置查询条件

	if query.Keyword != "" { // 按任务标题前缀匹配
		like := query.Keyword + "%"
		db = db.Where(model.TaskColumnTitle+" LIKE ?", like)
	}
	if query.Status != "" { // 状态筛选
		db = db.Where(model.TaskColumnStatus+" = ?", query.Status)
	}
	if query.Priority != "" { // 优先级筛选
		db = db.Where(model.TaskColumnPriority+" = ?", query.Priority)
	}

	// 统计总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total <= 0 {
		return []*model.Task{}, 0, nil
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	tasks := make([]*model.Task, 0, query.PageSize)

	listDB := db.
		Preload(model.TaskAssocAssignee).
		Preload(model.TaskAssocProject).
		Offset(offset).
		Limit(query.PageSize)

	// 默认排序规则（用户可选：标题、任务优先级、任务状态、预期时间、创建时间）：
	// 排序值 sort_order（内部使用：默认第一优先级）
	// 按照优先级(urgent、high、medium、low)
	// 任务状态(todo、in_progress、done、cancelled)
	// 标题升序
	// 预期时间升序
	// 创建时间降序
	// 构建最终 ORDER BY

	// 添加排序条件
	for _, order := range repoutil.BuildTaskOrders(query.SortBy, query.SortOrder) {
		listDB = listDB.Order(order)
	}

	if err := listDB.Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// TaskSortItem 项目任务排序项
//
// 用于接口：PATCH /api/v1/projects/:projectId/tasks/sort
//
// 字段说明：
//   - TaskID：需要更新排序值的任务 ID，必须属于当前 projectID
//   - SortOrder：新的排序值，数值越小越靠前
type TaskSortItem struct {
	TaskID    uint64
	SortOrder int
}

// BatchUpdateTaskSortWithTx 批量更新任务表中的排序值
func (r *taskRepository) BatchUpdateTaskSortWithTx(ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	items []*TaskSortItem,
	updatedAt time.Time,
) error {
	if projectID == 0 {
		return ErrInvalidTaskParam
	}

	if len(items) == 0 {
		return ErrTaskSortItemsIsEmpty
	}

	taskIDs := make([]uint64, 0, len(items))
	caseItems := make([]repoutil.CaseWhenItem, 0, len(items))

	// repository 层信任 service 已完成 nil、重复、归属等业务校验
	for _, item := range items {
		taskIDs = append(taskIDs, item.TaskID)

		caseItems = append(
			caseItems,
			repoutil.CaseWhenItem{
				ID:    item.TaskID,
				Value: item.SortOrder,
			},
		)
	}

	updates := map[string]interface{}{
		model.TaskColumnSortOrder: repoutil.BuildCaseWhenExpr(
			model.TaskColumnID,
			model.TaskColumnSortOrder,
			caseItems,
		),
		model.TaskColumnUpdatedAt: updatedAt,
	}

	result := getDB(ctx, r.db, tx).
		Model(&model.Task{}).
		Where(model.TaskColumnProjectID+" = ?", projectID).
		Where(model.TaskColumnID+" IN ?", taskIDs).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	// 防止 service 漏校验时静默成功，例如 task_id 不属于 project_id
	if result.RowsAffected == 0 {
		return ErrTaskSortNoRowsUpdated
	}

	return nil
}

// CountTasksByProjectIDAndIDs 统计指定项目下存在的任务数量
func (r *taskRepository) CountTasksByProjectIDAndIDs(ctx context.Context, projectID uint64, taskIDs []uint64) (int64, error) {
	if projectID == 0 || len(taskIDs) == 0 {
		return 0, ErrInvalidTaskParam
	}

	var count int64

	err := getDB(ctx, r.db, nil).
		Model(&model.Task{}).
		Where(model.TaskColumnProjectID+" = ?", projectID).
		Where(model.TaskColumnID+" IN ?", taskIDs).
		Count(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
}

// ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx 清空指定项目下指定负责人的任务负责人
//
// 使用场景：
//   - 项目成员被移除后，需要将该成员在当前项目下负责的任务 assignee_id 置为 NULL
//
// 注意：
//   - RowsAffected == 0 不视为错误，因为该成员可能没有负责任何任务
//   - 必须带 project_id 条件，避免误清空其它项目中的任务负责人
func (r *taskRepository) ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	assigneeID uint64,
	updatedAt time.Time,
) error {
	if projectID == 0 || assigneeID == 0 {
		return ErrInvalidTaskParam
	}

	err := getDB(ctx, r.db, tx).
		Model(&model.Task{}).
		Where(model.TaskColumnProjectID+" = ?", projectID).
		Where(model.TaskColumnAssigneeID+" = ?", assigneeID).
		Updates(map[string]interface{}{ // 使用 map 更新，确保 nil 可以被正确更新为 NULL
			model.TaskColumnAssigneeID: nil,
			model.TaskColumnUpdatedAt:  updatedAt,
		}).Error
	if err != nil {
		return err
	}

	return nil
}
