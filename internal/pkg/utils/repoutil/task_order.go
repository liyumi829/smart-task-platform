// internal/pkg/utils/repoutil/task_order.go
// 构造 task 任务排序规则操作
package repoutil

import (
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"strings"
	"sync"
)

const (
	orderASC  = "ASC"
	orderDESC = "DESC"
)

// BuildTaskOrders 构建任务排序规则
func BuildTaskOrders(sortBy, sortOrder string) []string {
	initTaskOrderOnce.Do(initDefaultTaskOrders)

	normalizedOrder, ok := normalizeSortOrder(sortOrder) // 标准化
	if !ok {
		// 用户未传排序或排序参数非法，使用默认排序
		return defaultTaskOrders
	}

	sortField, ok := taskSortFieldMap[sortBy]
	if !ok {
		// 用户未传排序或排序字段非法，使用默认排序
		return defaultTaskOrders
	}

	orderClauses := make([]string, 0, len(defaultTaskOrders)+2)

	// 用户指定排序放在第一优先级
	orderClauses = append(orderClauses, buildUserTaskOrder(sortField, normalizedOrder)...)

	// 默认排序中跳过用户已经指定的字段，避免重复排序
	for _, field := range defaultTaskSortFields {
		if field == sortField {
			continue
		}

		orderClauses = append(orderClauses, buildDefaultTaskOrderByField(field)...)
	}

	// 追加稳定排序字段，避免分页时顺序不稳定
	orderClauses = append(orderClauses, model.TaskColumnID+" DESC")

	return orderClauses
}

var (
	// taskSortFieldMap 接口排序字段到数据库排序字段的安全映射
	//
	// 注意：
	// 1. priority/status 不再使用 FIELD() 排序，改为 priority_order/status_order
	// 2. title 不再使用 COLLATE 表达式排序，直接使用 title 或 title_sort
	// 3. due_date NULL 排序不再使用 due_date IS NULL 表达式，改为 due_date_null_order
	taskSortFieldMap = map[string]string{
		dto.SortByPriority:   model.TaskColumnPriorityOrder,
		dto.SortByStatus:     model.TaskColumnStatusOrder,
		dto.SortByTitle:      model.TaskColumnTitle,
		dto.SortByDueDate:    model.TaskColumnDueDate,
		dto.SortByCreateTime: model.TaskColumnCreatedAt,
	}

	// defaultTaskSortFields 默认排序字段顺序
	defaultTaskSortFields = []string{
		model.TaskColumnSortOrder,
		model.TaskColumnPriorityOrder,
		model.TaskColumnStatusOrder,
		model.TaskColumnTitle,
		model.TaskColumnDueDate,
		model.TaskColumnCreatedAt,
	}

	// defaultTaskOrders 默认任务排序 SQL 片段
	defaultTaskOrders []string

	// initTaskOrderOnce 保证默认排序只初始化一次
	initTaskOrderOnce sync.Once
)

// / initDefaultTaskOrders 初始化默认排序规则
func initDefaultTaskOrders() {
	defaultTaskOrders = make([]string, 0, len(defaultTaskSortFields)+2)

	for _, field := range defaultTaskSortFields {
		defaultTaskOrders = append(defaultTaskOrders, buildDefaultTaskOrderByField(field)...)
	}

	// 追加稳定排序字段，避免分页时顺序不稳定
	defaultTaskOrders = append(defaultTaskOrders, model.TaskColumnID+" DESC")
}

// normalizeSortOrder 标准化排序方向
func normalizeSortOrder(sortOrder string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(sortOrder)) {
	case orderASC:
		return orderASC, true
	case orderDESC:
		return orderDESC, true
	default:
		return "", false
	}
}

// buildDefaultTaskOrderByField 根据字段构建默认排序
func buildDefaultTaskOrderByField(field string) []string {
	switch field {
	case model.TaskColumnSortOrder:
		return []string{
			model.TaskColumnSortOrder + " ASC",
		}

	case model.TaskColumnPriorityOrder:
		return []string{
			model.TaskColumnPriorityOrder + " ASC",
		}

	case model.TaskColumnStatusOrder:
		return []string{
			model.TaskColumnStatusOrder + " ASC",
		}

	case model.TaskColumnTitle:
		return []string{
			model.TaskColumnTitle + " ASC",
		}

	case model.TaskColumnDueDate:
		return []string{
			// NULL 排在最后，避免使用 due_date IS NULL 表达式排序
			model.TaskColumnDueDateNullOrder + " ASC",
			model.TaskColumnDueDate + " ASC",
		}

	case model.TaskColumnCreatedAt:
		return []string{
			model.TaskColumnCreatedAt + " DESC",
		}

	default:
		return nil
	}
}

// buildUserTaskOrder 构建用户指定排序
func buildUserTaskOrder(field string, sortOrder string) []string {
	switch field {
	case model.TaskColumnPriorityOrder:
		return []string{
			model.TaskColumnPriorityOrder + " " + sortOrder,
		}

	case model.TaskColumnStatusOrder:
		return []string{
			model.TaskColumnStatusOrder + " " + sortOrder,
		}

	case model.TaskColumnTitle:
		return []string{
			model.TaskColumnTitle + " " + sortOrder,
		}

	case model.TaskColumnDueDate:
		return []string{
			// NULL 始终排在最后
			model.TaskColumnDueDateNullOrder + " ASC",
			model.TaskColumnDueDate + " " + sortOrder,
		}

	case model.TaskColumnCreatedAt:
		return []string{
			model.TaskColumnCreatedAt + " " + sortOrder,
		}

	default:
		return nil
	}
}
