// internal/pkg/utils/repoutil/task_order.go
// 构造 task 任务排序规则操作
package repoutil

import (
	"fmt"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"strings"
	"sync"
)

// BuildTaskOrders 构建任务排序规则
func BuildTaskOrders(sortBy, sortOrder string) []string {
	initTaskOrderOnce.Do(initDefaultTaskOrders)

	orderClauses := make([]string, 0, len(defaultTaskOrders)+2)

	sortField, fieldOK := taskSortFieldMap[sortBy]
	orderOK := sortOrder == "ASC" || sortOrder == "DESC" || sortOrder == "asc" || sortOrder == "desc"
	fmt.Println(sortBy, "  ", sortOrder)
	if fieldOK && orderOK {
		// 用户指定排序放在第一优先级
		fmt.Println(1111)
		orderClauses = append(orderClauses, buildUserTaskOrder(sortField, sortOrder)...)

		// 默认排序中跳过用户已经指定的字段，避免重复排序
		for _, field := range defaultTaskSortFields {
			if field == sortField {
				continue
			}
			orderClauses = append(orderClauses, buildDefaultTaskOrderByField(field)...)
		}

		// 追加稳定排序字段
		orderClauses = append(orderClauses, fmt.Sprintf("%s ASC", model.TaskColumnID))
		fmt.Println(orderClauses)
		return orderClauses
	}

	// 用户未传排序或排序参数非法，使用默认排序
	return defaultTaskOrders
}

var (
	// 允许排序的字段映射，key 是接口排序参数，value 是数据库字段名
	taskSortFieldMap = map[string]string{
		dto.SortByPriority:   model.TaskColumnPriority,
		dto.SortByStatus:     model.TaskColumnStatus,
		dto.SortByTitle:      model.TaskColumnTitle,
		dto.SortByDueDate:    model.TaskColumnDueDate,
		dto.SortByCreateTime: model.TaskColumnCreatedAt,
	}

	// 默认排序字段顺序
	defaultTaskSortFields = []string{
		model.TaskColumnSortOrder,
		model.TaskColumnPriority,
		model.TaskColumnStatus,
		model.TaskColumnTitle,
		model.TaskColumnDueDate,
		model.TaskColumnCreatedAt,
	}

	// 默认的任务顺序的 SQL 语句
	defaultTaskOrders []string

	// 初始化
	initTaskOrderOnce sync.Once
)

// initDefaultTaskOrders 初始化默认排序规则
func initDefaultTaskOrders() {
	defaultTaskOrders = make([]string, 0, len(defaultTaskSortFields)+2)

	for _, field := range defaultTaskSortFields {
		defaultTaskOrders = append(defaultTaskOrders, buildDefaultTaskOrderByField(field)...)
	}

	// 追加稳定排序字段，避免分页时顺序不稳定
	defaultTaskOrders = append(defaultTaskOrders, fmt.Sprintf("%s ASC", model.TaskColumnID))
}

// buildDefaultTaskOrderByField 根据字段构建默认排序
func buildDefaultTaskOrderByField(field string) []string {
	switch field {
	case model.TaskColumnPriority:
		return []string{
			fmt.Sprintf(
				"FIELD(%s, '%s', '%s', '%s', '%s') ASC",
				field,
				model.TaskPriorityUrgent,
				model.TaskPriorityHigh,
				model.TaskPriorityMedium,
				model.TaskPriorityLow,
			),
		}

	case model.TaskColumnStatus:
		return []string{
			fmt.Sprintf(
				"FIELD(%s, '%s', '%s', '%s', '%s') ASC",
				field,
				model.TaskStatusTodo,
				model.TaskStatusInProgress,
				model.TaskStatusDone,
				model.TaskStatusCancelled,
			),
		}

	case model.TaskColumnDueDate:
		return []string{
			// NULL 排在最后，有截止时间的任务优先展示
			fmt.Sprintf("%s IS NULL ASC", field),
			fmt.Sprintf("%s ASC", field),
		}

	case model.TaskColumnCreatedAt:
		return []string{
			fmt.Sprintf("%s DESC", field),
		}

	case model.TaskColumnTitle:
		// 中文按拼音排序
		return []string{fmt.Sprintf("%s COLLATE utf8mb4_unicode_ci ASC", field)}

	case model.TaskColumnSortOrder:
		return []string{
			fmt.Sprintf("%s ASC", field),
		}

	default:
		return []string{}
	}
}

// buildUserTaskOrder 构建用户指定排序
func buildUserTaskOrder(field string, sortOrder string) []string {
	switch field {
	case model.TaskColumnPriority:
		return []string{
			fmt.Sprintf(
				"FIELD(%s, '%s', '%s', '%s', '%s') %s",
				field,
				model.TaskPriorityUrgent,
				model.TaskPriorityHigh,
				model.TaskPriorityMedium,
				model.TaskPriorityLow,
				strings.ToUpper(sortOrder),
			),
		}

	case model.TaskColumnStatus:
		return []string{
			fmt.Sprintf(
				"FIELD(%s, '%s', '%s', '%s', '%s') %s",
				field,
				model.TaskStatusTodo,
				model.TaskStatusInProgress,
				model.TaskStatusDone,
				model.TaskStatusCancelled,
				strings.ToUpper(sortOrder),
			),
		}

	case model.TaskColumnDueDate:
		return []string{
			// NULL 排在最后
			fmt.Sprintf("%s IS NULL ASC", field),
			fmt.Sprintf("%s %s", field, strings.ToUpper(sortOrder)),
		}

	case model.TaskColumnTitle:
		// 中文按拼音排序
		return []string{
			fmt.Sprintf("%s COLLATE utf8mb4_unicode_ci %s", field, strings.ToUpper(sortOrder)),
		}

	case model.TaskColumnCreatedAt:
		return []string{
			fmt.Sprintf("%s %s", field, strings.ToUpper(sortOrder)),
		}

	default:
		return []string{}
	}
}
