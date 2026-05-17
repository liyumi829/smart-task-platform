// internal/service/cachesvc/cache_util.go
// Package cachesvc
// 工具函数
package cachesvc

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"

	"go.uber.org/zap"
)

//==========
// 用户模块
//==========

// BuildBriefUserModelsFromPage 根据缓存页获取用户的简要信息。
// 设计说明：
//   - 先批量查缓存。
//   - 缓存未命中的，再批量查 DB。
//   - DB 回源结果可顺带回填缓存。
//   - 最终按 page.List 的顺序返回。
//
// 注意：
//   - 这里返回的是“简要用户信息”，不会把完整 user 都塞进来。
//   - 如果缓存值脏了，会删除脏缓存并按 miss 处理。
func (s *CacheService) BuildBriefUserModelsFromPage(ctx context.Context, userIDs []uint64) ([]*model.User, error) {
	if s == nil {
		return nil, nil
	}

	if len(userIDs) == 0 {
		return []*model.User{}, nil
	}

	// 1. 批量查缓存
	briefUserMap, missingUserIDs, err := s.userCache.BatchGetBriefInfo(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	// 进行转化
	userMap := utils.MapToOtherMap(briefUserMap, func(val *cacheObj.UserBriefInfo) *model.User {
		return val.ToModel()
	})

	// 2. 缓存未命中的，批量查 DB
	if len(missingUserIDs) > 0 {
		users, err := s.ur.BatchGetUsersByIDs(ctx, missingUserIDs)
		if err != nil {
			return nil, err
		}

		usersToCache := make([]*model.User, 0, len(users))
		for _, user := range users {
			if user == nil || user.ID == 0 {
				continue
			}

			// 回源结果加入返回 map
			userMap[user.ID] = utils.SafePtrClone(user)

			// 收集有效用户，后面统一批量写缓存。
			usersToCache = append(usersToCache, user)
		}

		// 3. 批量回填缓存。
		// 注意：缓存失败由 UserCache 内部记录日志，不影响主流程。
		if len(usersToCache) > 0 {
			s.userCache.MSetBriefInfo(ctx, usersToCache)
		}
	}

	// 4. 按 page.List 顺序组装返回值
	res := make([]*model.User, 0, len(userIDs))
	for _, userID := range userIDs {
		user := userMap[userID]
		if user == nil || user.ID == 0 {
			continue
		}
		res = append(res, user)
	}

	return res, nil
}

//==========
// 项目模块
//==========

// BuildBriefProjectModelsFromPage 根据缓存页获取项目的简要信息。
func (s *CacheService) BuildBriefProjectModelsFromPage(ctx context.Context, projectIDs []uint64, preload bool) ([]*model.Project, error) {
	if s == nil {
		return nil, nil
	}

	if len(projectIDs) == 0 {
		return []*model.Project{}, nil
	}

	// 1. 批量查缓存
	briefProjectMap, missingProjectIDs, err := s.projectCache.BatchGetBriefInfo(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	// 进行转化
	projectMap := utils.MapToOtherMap(briefProjectMap, func(val *cacheObj.ProjectBriefInfo) *model.Project {
		return val.ToModel()
	})

	// 2. 缓存未命中的，批量查 DB
	if len(missingProjectIDs) > 0 {
		projects, err := s.pr.BatchGetProjectsByIDs(ctx, missingProjectIDs)
		if err != nil {
			return nil, err
		}

		projectsToCache := make([]*model.Project, 0, len(projects))
		for _, project := range projects {
			if project == nil || project.ID == 0 {
				continue
			}

			// 回源结果加入返回 map
			projectMap[project.ID] = utils.SafePtrClone(project)

			// 收集有效项目，后面统一批量写缓存。
			projectsToCache = append(projectsToCache, project)
		}

		// 3. 批量回填缓存。
		// 注意：缓存失败由 UserCache 内部记录日志，不影响主流程。
		if len(projectsToCache) > 0 {
			s.projectCache.MSetBriefInfo(ctx, projectsToCache)
		}
	}

	// 判断是否预加载
	var userMap map[uint64]*model.User
	if preload { // 预加载用户信息
		// 4. 收集需要补齐的 owner 信息
		needUserIDs := collectMissingIDs(projectMap, func(project *model.Project) (uint64, bool) {
			// 空任务直接跳过（不需要收集）
			// 已经有 owner 对象
			if project == nil || project.Owner != nil || project.OwnerID == 0 {
				return 0, true
			}

			return project.OwnerID, false // 需要收集的有效 userID
		})

		// 5. 复用用户模块获取 assignee 简要信息
		userMap = make(map[uint64]*model.User, len(needUserIDs))
		if len(needUserIDs) > 0 {
			users, err := s.BuildBriefUserModelsFromPage(ctx, needUserIDs)
			if err != nil {
				return nil, err
			}
			userMap = utils.SliceToMap(users, func(user *model.User) uint64 {
				if user == nil {
					return 0
				}
				return user.ID
			})
		}
	}

	// 4. 按 page.List 顺序组装返回值
	res := make([]*model.Project, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		project := projectMap[projectID]
		if project == nil || project.ID == 0 {
			continue
		}
		// 缓存命中时 project. 通常为空，这里补齐项目信息
		if preload && project.Owner == nil && project.OwnerID != 0 {
			if user := userMap[project.OwnerID]; user != nil {
				project.Owner = utils.SafePtrClone(user)
			}
		}
		res = append(res, project)
	}

	return res, nil
}

//==========
// 任务模块
//==========

// BuildTaskDetailModelsFromCache 根据缓存构造任务详细信息
func (s *CacheService) BuildTaskDetailModelsFromCache(ctx context.Context, detailInfo *cacheObj.TaskDetailInfo) (*model.Task, error) {
	if s == nil || detailInfo == nil {
		return nil, nil
	}

	task := detailInfo.ToModel()
	if task == nil || task.ID == 0 {
		return nil, nil
	}

	// 1. 恢复 Project brief 信息
	projectID := task.ProjectID
	if projectID != 0 {
		projectInfo, exists, hit := s.projectCache.GetBriefInfo(ctx, projectID)
		if hit {
			if exists && projectInfo != nil {
				task.Project = projectInfo.ToModel()
			}
		} else {
			project, err := s.pr.GetByID(ctx, projectID)
			if err != nil {
				if errors.Is(err, repository.ErrProjectNotFound) {
					s.projectCache.SetBriefInfoNull(ctx, projectID)
				} else {
					zap.L().Warn("get project by id failed when rebuild task detail from cache",
						zap.Uint64("task_id", task.ID),
						zap.Uint64("project_id", projectID),
						zap.Error(err),
					)
					return nil, err
				}
			} else if project != nil && project.ID != 0 {
				task.Project = project
				s.projectCache.SetBriefInfo(ctx, project)
			}
		}
	}

	// 2. 恢复 Creator / Assignee brief 信息
	userIDs := make([]uint64, 0, 2)
	if task.CreatorID != 0 {
		userIDs = append(userIDs, task.CreatorID)
	}
	if task.AssigneeID != nil && *task.AssigneeID != 0 && *task.AssigneeID != task.CreatorID {
		userIDs = append(userIDs, *task.AssigneeID)
	}

	if len(userIDs) > 0 {
		briefInfoMap, userMissingIDs, err := s.userCache.BatchGetBriefInfo(ctx, userIDs)
		if err != nil {
			zap.L().Warn("batch get user brief info cache failed when rebuild task detail from cache",
				zap.Uint64("task_id", task.ID),
				zap.Uint64s("user_ids", userIDs),
				zap.Error(err),
			)
			return nil, err
		}

		// 直接转成 model map
		userMap := utils.MapToOtherMap(briefInfoMap, func(info *cacheObj.UserBriefInfo) *model.User {
			return info.ToModel()
		})

		if len(userMissingIDs) > 0 {
			users, err := s.ur.BatchGetUsersByIDs(ctx, userMissingIDs)
			if err != nil {
				zap.L().Warn("batch get users by ids failed when rebuild task detail from cache",
					zap.Uint64("task_id", task.ID),
					zap.Uint64s("missing_user_ids", userMissingIDs),
					zap.Error(err),
				)
				return nil, err
			}

			for _, user := range users {
				if user == nil || user.ID == 0 {
					continue
				}
				userMap[user.ID] = user
			}

			if len(users) > 0 {
				s.userCache.MSetBriefInfo(ctx, users)
			}
		}

		if user := userMap[task.CreatorID]; user != nil && user.ID != 0 {
			task.Creator = user
		}

		if task.AssigneeID != nil {
			if user := userMap[*task.AssigneeID]; user != nil && user.ID != 0 {
				task.Assignee = user
			}
		}
	}

	return task, nil
}

// BuildProjectTaskModelsFromPage 根据缓存页恢复项目任务列表 model。
func (s *CacheService) BuildProjectTaskModelsFromPage(ctx context.Context, taskIDs []uint64) ([]*model.Task, error) {
	if s == nil {
		return nil, nil
	}

	if len(taskIDs) == 0 {
		return []*model.Task{}, nil
	}

	// 1、批量查缓存
	// 保持语义干净
	taskItemMap, missingTaskIDs, err := s.taskCache.BatchGetListItem(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	// 转化
	taskMap := utils.MapToOtherMap(taskItemMap, func(t *cacheObj.TaskListItem) *model.Task {
		return t.ToModel()
	})

	// 2、未命中的缓存，批量查 DB
	if len(missingTaskIDs) > 0 {
		// 对于没有命中的
		tasks, err := s.tr.BatchGetTasksByIDs(ctx, missingTaskIDs) // 会预加载用户信息，不用管
		if err != nil {
			return nil, err
		}
		taskToCache := make([]*model.Task, 0, len(tasks))
		for _, task := range tasks {
			if task == nil || task.ID == 0 {
				continue
			}

			// 回源结构带给map
			taskMap[task.ID] = utils.SafePtrClone(task)

			// 顺带写回缓存
			taskToCache = append(taskToCache, task)
		}
		// 3. 批量写回缓存
		if len(taskToCache) > 0 {
			s.taskCache.MSetListItem(ctx, taskToCache)
		}
	}

	// 4. 收集需要补齐的 assigneeID
	needUserIDs := collectMissingIDs(taskMap, func(task *model.Task) (uint64, bool) {
		// 空任务直接跳过（不需要收集）
		// 已经有 Assignee 对象（不需要收集）
		// 没有 AssigneeID / ID=0（不需要收集）
		if task == nil || task.Assignee != nil || task.AssigneeID == nil || *task.AssigneeID == 0 {
			return 0, true
		}

		return *task.AssigneeID, false // 需要收集的有效 userID
	})

	// 5. 复用用户模块获取 assignee 简要信息
	userMap := make(map[uint64]*model.User, len(needUserIDs))
	if len(needUserIDs) > 0 {
		users, err := s.BuildBriefUserModelsFromPage(ctx, needUserIDs)
		if err != nil {
			return nil, err
		}
		userMap = utils.SliceToMap(users, func(user *model.User) uint64 {
			if user == nil {
				return 0
			}
			return user.ID
		})
	}
	// 6. 按 page.List 顺序组装返回值
	res := make([]*model.Task, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		task := taskMap[taskID]
		if task == nil || task.ID == 0 {
			continue
		}

		// 缓存命中时 task.Assignee 通常为空，这里补齐
		if task.Assignee == nil && task.AssigneeID != nil && *task.AssigneeID != 0 {
			if user := userMap[*task.AssigneeID]; user != nil {
				task.Assignee = utils.SafePtrClone(user)
			}
		}

		res = append(res, task)
	}

	return res, nil
}

// BuildUserTaskModelsFromPage 根据缓存页恢复用户任务列表 model
func (s *CacheService) BuildUserTaskModelsFromPage(ctx context.Context, taskIDs []uint64) ([]*model.Task, error) {
	if s == nil {
		return nil, nil
	}

	if len(taskIDs) == 0 {
		return []*model.Task{}, nil
	}

	// 1、批量查缓存
	taskItemMap, missingTaskIDs, err := s.taskCache.BatchGetListItem(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	// 转化
	taskMap := utils.MapToOtherMap(taskItemMap, func(t *cacheObj.TaskListItem) *model.Task {
		return t.ToModel()
	})

	// 2、未命中的缓存，批量查 DB
	if len(missingTaskIDs) > 0 {
		// 对于没有命中的
		tasks, err := s.tr.BatchGetTasksByIDs(ctx, missingTaskIDs) // 会预加载项目信息
		if err != nil {
			return nil, err
		}
		taskToCache := make([]*model.Task, 0, len(tasks))
		for _, task := range tasks {
			if task == nil || task.ID == 0 {
				continue
			}

			// 回源结构带给map
			taskMap[task.ID] = utils.SafePtrClone(task)

			// 顺带写回缓存
			taskToCache = append(taskToCache, task)
		}
		// 3. 批量写回缓存
		if len(taskToCache) > 0 {
			s.taskCache.MSetListItem(ctx, taskToCache)
		}
	}

	// 4. 收集需要补齐的 projectID
	needProjectIDs := collectMissingIDs(taskMap, func(task *model.Task) (uint64, bool) {
		// 不需要收集的：任务为空，项目信息不为空，不合法的项目ID
		if task == nil || task.Project != nil || task.ProjectID == 0 {
			return 0, true
		}
		return task.ProjectID, false
	})

	// 5. 复用项目模块获取 project 简要信息
	projectMap := make(map[uint64]*model.Project, len(needProjectIDs))
	if len(needProjectIDs) > 0 {
		projects, err := s.BuildBriefProjectModelsFromPage(ctx, needProjectIDs, false)
		if err != nil {
			return nil, err
		}
		projectMap = utils.SliceToMap(projects, func(project *model.Project) uint64 {
			if project == nil {
				return 0
			}
			return project.ID
		})
	}

	// 6. 按 page.List 顺序组装返回值
	res := make([]*model.Task, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		task := taskMap[taskID]
		if task == nil || task.ID == 0 {
			continue
		}

		// 缓存命中时 task.Project 通常为空，这里补齐项目信息
		if task.Project == nil && task.ProjectID != 0 {
			if project := projectMap[task.ProjectID]; project != nil {
				task.Project = utils.SafePtrClone(project)
			}
		}
		res = append(res, task)
	}

	return res, nil
}

// collectMissingIDs 从 map 中收集需要补齐的关联 ID。
// 规则：
//   - fn 返回 (id, true) 表示跳过，不收集；
//   - fn 返回 (id, false) 表示收集该 id；
//   - 返回结果会自动去重。
func collectMissingIDs[T any](m map[uint64]T, fn func(val T) (uint64, bool)) []uint64 {
	if len(m) == 0 {
		return nil
	}

	seen := make(map[uint64]struct{}, len(m))
	ids := make([]uint64, 0, len(m))

	for _, val := range m {
		id, ignore := fn(val)
		if ignore {
			continue
		}

		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return ids
}
