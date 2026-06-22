// internal/service/cachesvc/cache_convert.go
// Package cachesvc
// 实现从 model 对象到 cache 对象的转化

package cachesvc

import (
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"
	"strings"
)

// Converter 缓存对象转换器
type Converter struct{}

// UserBriefInfo 从 model.User 构造用户简要信息缓存对象。
func (c Converter) UserBriefInfo(user *model.User) *cacheObj.UserBriefInfo {
	if user == nil || user.ID == 0 {
		return nil
	}

	return &cacheObj.UserBriefInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}
}

// ProjectBriefInfo 从 model.Project 构造用户简要信息缓存对象。
// 注意：这里只提取缓存需要的公共字段。
func (c Converter) ProjectBriefInfo(project *model.Project) *cacheObj.ProjectBriefInfo {
	if project == nil || project.ID == 0 {
		return nil
	}

	return &cacheObj.ProjectBriefInfo{
		ID:   project.ID,
		Name: project.Name,
	}
}

// TaskPermissionInfo 从 model.Task 构造任务权限信息缓存对象。
//   - 注意：这里只提取权限判断需要的字段。
func (c Converter) TaskPermissionInfo(task *model.Task) *cacheObj.TaskPermissionInfo {
	if task == nil || task.ID == 0 {
		return nil
	}

	return &cacheObj.TaskPermissionInfo{
		ID:         task.ID,
		Title:      task.Title,
		ProjectID:  task.ProjectID,
		CreatorID:  task.CreatorID,
		AssigneeID: task.AssigneeID,
		Status:     strings.TrimSpace(task.Status),
	}
}

// TaskDetailInfo 从 model.Task 构造任务详细信息
func (c Converter) TaskDetailInfo(task *model.Task) *cacheObj.TaskDetailInfo {
	if task == nil || task.ID == 0 {
		return nil
	}

	return &cacheObj.TaskDetailInfo{
		ID:          task.ID,
		ProjectID:   task.ProjectID,
		Title:       task.Title,
		Status:      task.Status,
		Priority:    task.Priority,
		AssigneeID:  task.AssigneeID,
		DueDate:     task.DueDate,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
		Description: utils.SafeStringValue(task.Description),
		CreatorID:   task.CreatorID,
	}
}

// TaskListItem 从 model.Task 构造任务列表缓存对象。
// - 注意：这里只提取 task 基础字段；Project / Assignee 由业务组装时按需填充。
func (c Converter) TaskListItem(task *model.Task) *cacheObj.TaskListItem {
	if task == nil || task.ID == 0 {
		return nil
	}

	return &cacheObj.TaskListItem{
		ID:         task.ID,
		ProjectID:  task.ProjectID,
		Title:      task.Title,
		Status:     task.Status,
		Priority:   task.Priority,
		AssigneeID: task.AssigneeID,
		DueDate:    task.DueDate,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
	}
}

// TaskListPage 根据 SearchTasks 结果构造缓存页。
// 说明：
//   - 缓存页只保存 taskID。
//   - result.List 中的 model 不进入 page cache。
//   - result.List 会通过 bundle.Tasks 返回给本次请求复用。
func (c Converter) TaskListPage(query *cacheObj.TaskListQuery, result *repository.TaskSearchResult) *cacheObj.TaskListPage {
	if query == nil {
		return nil
	}

	return buildListPage(query.Page, query.PageSize, result, func(task *model.Task) uint64 {
		if task == nil {
			return 0
		}
		return task.ID
	})
}

// UserListPage 根据 SearchUsers 结果构造缓存页。
func (c Converter) UserListPage(query *cacheObj.UserListQuery, result *repository.UserSearchResult) *cacheObj.UserListPage {
	if query == nil {
		return nil
	}

	return buildListPage(query.Page, query.PageSize, result, func(user *model.User) uint64 {
		if user == nil {
			return 0
		}
		return user.ID
	})
}

func buildListPage[T any](page, pageSize int, result *repository.SearchResult[T], fn func(val T) uint64) *cacheObj.ListPage {
	if result == nil {
		return nil
	}

	ids := make([]uint64, 0, len(result.List))
	for _, item := range result.List {
		id := fn(item)
		if id == 0 {
			continue
		}
		ids = append(ids, id)
	}

	return &cacheObj.ListPage{
		Page:     page,
		PageSize: pageSize,
		List:     ids,
		Total:    result.Total,
		HasMore:  result.HasMore,
	}
}
