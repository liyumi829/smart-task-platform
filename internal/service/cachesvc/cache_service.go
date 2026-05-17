// internal/service/cachesvc/service.go
// Package cachesvc
// 缓存服务实例的定义和构造
package cachesvc

import (
	"context"
	"errors"
	cacheStore "smart-task-platform/internal/cache"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service/cachesvc/cacheobj"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

var (
	errInvalidTaskListBundle   = errors.New("invalid task list bundle")
	errInvalidTaskDetailBundle = errors.New("invalid task detail bundle")
)

// userRepo 用户仓储接口
type userRepo interface {
	ExistsByUserID(ctx context.Context, userID uint64) (bool, error)
	GetByID(ctx context.Context, id uint64) (*model.User, error)
	BatchGetUsersByIDs(ctx context.Context, userIDs []uint64) ([]*model.User, error)
	SearchUsers(ctx context.Context, query *repository.UserSearchQuery) (*repository.UserSearchResult, error)
}

type projectRepo interface {
	ExistsByProjectID(ctx context.Context, projectID uint64) (bool, error)
	GetByID(ctx context.Context, id uint64) (*model.Project, error)
	BatchGetProjectsByIDs(ctx context.Context, projectIDs []uint64) ([]*model.Project, error)
	SearchProjects(ctx context.Context, query *repository.ProjectSearchQuery) (*repository.SearchProjectResult, error)
}

// projectMemberRepo 项目成员仓储接口
//   - 注意：GetProjectMemberRoleByProjectIDAndUserID 返回空字符串表示不是项目成员
type projectMemberRepo interface {
	GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error)
	ListProjectIDsByUserID(ctx context.Context, userID uint64) ([]uint64, error)
}

// taskRepo 任务仓储接口
// 注意：
//  1. GetByID 只查询任务权限判断需要的基础字段
//  2. 该接口服务于缓存回源，不用于任务更新
//  3. 更新任务时应该直接在 task_service 中查询 DB
type taskRepo interface {
	GetDetailByID(ctx context.Context, taskID uint64) (*model.Task, error)
	GetByID(ctx context.Context, taskID uint64) (*model.Task, error)
	BatchGetTasksByIDs(ctx context.Context, taskIDs []uint64) ([]*model.Task, error)
	SearchTasks(ctx context.Context, query *repository.TaskSearchQuery) (*repository.TaskSearchResult, error)
}

// CacheService 缓存服务实例
//   - 第一版实现简单的权限级别的缓存
type CacheService struct {
	store              cacheStore.Store   // 操作句柄 Set/Del/Get/MGet
	sf                 singleflight.Group // 单飞模式控制
	kb                 *keyBuilder        // key建造者
	conv               *Converter         // 类型转化器
	ur                 userRepo
	pr                 projectRepo
	pmr                projectMemberRepo
	tr                 taskRepo
	userCache          *UserCache          // 用户缓存
	projectCache       *ProjectCache       // 项目缓存
	projectMemberCache *ProjectMemberCache // 项目成员缓存
	taskCache          *TaskCache          // 任务缓存
}

// NewCacheService 创建缓存服务实例
func NewCacheService(
	store cacheStore.Store,
	ur userRepo,
	pr projectRepo,
	pmr projectMemberRepo,
	tr taskRepo,
	vc VersionController,
) *CacheService {
	return &CacheService{
		store:              store,
		ur:                 ur,
		pr:                 pr,
		pmr:                pmr,
		tr:                 tr,
		conv:               &Converter{},
		userCache:          NewUserCache(store, NewkeyBuilder(vc), &Converter{}),
		projectCache:       NewProjectCache(store, NewkeyBuilder(vc), &Converter{}),
		projectMemberCache: NewProjectMemberCache(store, NewkeyBuilder(vc)),
		taskCache:          NewTaskCache(store, NewkeyBuilder(vc), &Converter{}),
	}
}

//==========
// 用户模块
//==========

// UserExists 判断用户是否存在
//   - 逻辑：先查缓存，缓存未命中进入 singleflight，singleflight 内二次查缓存，仍未命中再查 DB，最后写回缓存
//   - 注意：缓存 Get / Set / Del 失败只记录日志，不影响正常业务逻辑
func (s *CacheService) UserExists(ctx context.Context, userID uint64) (bool, error) {
	if s == nil || userID == 0 {
		return false, nil
	}

	key := s.userCache.ExistsKey(userID) // 构建key

	return loadExistsWithSF(
		ctx,
		&s.sf,
		key,
		func(ctx context.Context) (bool, bool) {
			return s.userCache.GetExists(ctx, userID)
		},
		func(ctx context.Context, exists bool) {
			s.userCache.SetExists(ctx, userID, exists)
		},
		func(ctx context.Context) (bool, error) {
			return s.ur.ExistsByUserID(ctx, userID)
		},
		repository.ErrUserNotFound,
		sfLogMeta{
			Message: "invalid user exists singleflight result",
			Fields: []zap.Field{
				zap.Uint64("user_id", userID),
				zap.String("cache_key", key),
			},
		},
	)
}

// DeleteUserExistsCache 删除用户是否存在缓存
func (s *CacheService) DeleteUserExistsCache(ctx context.Context, userID uint64) error {
	return s.userCache.DeleteExists(ctx, userID)
}

// GetUserBriefInfo 获取用户简要信息
//   - 返回值：user 表示用户简要信息，exists 表示用户是否存在
//   - 逻辑：先查缓存，缓存未命中进入 singleflight，singleflight 内二次查缓存，仍未命中再查 DB，最后写回缓存
//   - 注意：缓存 Get / Set / Del 失败只记录日志，不影响正常业务逻辑
func (s *CacheService) GetUserBriefInfo(ctx context.Context, userID uint64) (*model.User, bool, error) {
	if s == nil || userID == 0 {
		return nil, false, nil
	}

	key := s.userCache.BriefInfoKey(userID)

	config := &objectSFConfig[*model.User, *cacheObj.UserBriefInfo]{
		SF:  &s.sf,
		Key: key,
		GetCache: func(ctx context.Context) (*cacheobj.UserBriefInfo, bool, bool) {
			return s.userCache.GetBriefInfo(ctx, userID)
		},
		IsCacheValid: func(ubi *cacheObj.UserBriefInfo) bool {
			return ubi != nil && ubi.ID != 0
		},
		LoadDB: func(ctx context.Context) (*model.User, error) {
			return s.ur.GetByID(ctx, userID)
		},
		IsItemValid: func(u *model.User) bool {
			return u != nil && u.ID != 0
		},
		SetCache: s.userCache.SetBriefInfo,
		SetNull: func(ctx context.Context) {
			s.userCache.SetBriefInfoNull(ctx, userID)
		},
		DelCache: func(ctx context.Context) {
			s.userCache.DeleteBriefInfo(ctx, userID)
		},
		AfterLoadDB: nil,
		BuildFromCache: func(ctx context.Context, cache *cacheObj.UserBriefInfo) (*model.User, error) {
			if cache == nil {
				return nil, nil
			}
			return cache.ToModel(), nil
		},
		NotFoundErr: repository.ErrUserNotFound,
		LogMeta: sfLogMeta{
			Message: "invalid user brief info singleflight result",
			Fields: []zap.Field{
				zap.Uint64("user_id", userID),
				zap.String("cache_key", key),
			},
		},
	}
	return loadObjectByBundleWithSF(ctx, config)
}

// DeleteUserBriefInfoCache 删除用户简要信息缓存
func (s *CacheService) DeleteUserBriefInfoCache(ctx context.Context, userID uint64) error {
	return s.userCache.DeleteBriefInfo(ctx, userID)
}

// userListSFBundle 用户列表加载结果
type userListSFBundle = listSFBundle[*model.User]

// GetUserList 获取用户列表 model
// 说明：
//   - 这是对外真正返回业务 model 的方法
//   - miss 时直接复用 SearchUsers 查出来的 Users
//   - hit 时根据 Page.List 批量恢复 Users
func (s *CacheService) GetUserList(ctx context.Context, query *cacheObj.UserListQuery) ([]*model.User, *int64, bool, error) {
	if query == nil {
		return []*model.User{}, utils.Int64Ptr(0), false, nil
	}

	querySnapshot := utils.SafePtrClone(query)
	key := s.userCache.ListKey(querySnapshot) // 构造 key

	return loadAndReturnListPage(
		ctx,
		&s.sf,
		key,
		querySnapshot.ToRepository(),
		s.userCache.GetListPage,
		s.userCache.SetListPage,
		s.ur.SearchUsers,
		buildListPage,
		func(ctx context.Context, items []*model.User) {
			if len(items) == 0 {
				return
			}
			s.userCache.MSetBriefInfo(ctx, items)
		},
		s.BuildBriefUserModelsFromPage,
		querySnapshot.Page,
		query.PageSize,
		func(user *model.User) uint64 {
			if user == nil {
				return 0
			}
			return user.ID
		},
		sfLogMeta{
			Message: "invalid user list bundle",
			Fields: []zap.Field{
				zap.String("cache_key", key),
			},
		},
	)
}

// BumpUserListPage 更新版本号（删除缓存）
func (s *CacheService) BumpUserListPage(ctx context.Context) error {
	return s.userCache.BumpListPage(ctx)
}

// GetUserProjectIDs 获取当前用户的项目ID集合
func (s *CacheService) GetUserProjectIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if s == nil || userID == 0 {
		return nil, nil
	}

	key := s.userCache.ProjectIDsKey(userID)

	config := &objectSFConfig[[]uint64, *cacheObj.UserProjectIDs]{
		SF:  &s.sf,
		Key: key,

		GetCache: func(ctx context.Context) (*cacheObj.UserProjectIDs, bool, bool) {
			value, hit := s.userCache.GetProjectIDs(ctx, userID)
			if !hit {
				return nil, false, false
			}
			if value == nil {
				return nil, false, false
			}

			return value, true, true
		},

		IsCacheValid: func(upi *cacheObj.UserProjectIDs) bool {
			// ProjectIDs == []uint64{} 是合法缓存，表示当前用户没有项目。
			// ProjectIDs == nil 视为无效缓存。
			return upi != nil && upi.ProjectIDs != nil
		},

		DelCache: func(ctx context.Context) {
			s.userCache.BumpUserProjectList(ctx)
		},

		LoadDB: func(ctx context.Context) ([]uint64, error) {
			projectIDs, err := s.pmr.ListProjectIDsByUserID(ctx, userID)
			if err != nil {
				return nil, err
			}

			// 统一语义：没有项目时返回空切片，而不是 nil。
			if projectIDs == nil {
				projectIDs = []uint64{}
			}

			return projectIDs, nil
		},

		IsItemValid: func(projectIDs []uint64) bool {
			// 空切片是合法值。
			// nil 表示没有正常构造出结果。
			return projectIDs != nil
		},

		SetNull: nil, // 不写空值
		SetCache: func(ctx context.Context, projectIDs []uint64) {
			if projectIDs == nil {
				projectIDs = []uint64{}
			}

			s.userCache.SetProjectIDs(ctx, userID, &cacheObj.UserProjectIDs{
				ProjectIDs: projectIDs,
			})
		},

		BuildFromCache: func(ctx context.Context, cache *cacheObj.UserProjectIDs) ([]uint64, error) {
			if cache == nil || len(cache.ProjectIDs) == 0 {
				return []uint64{}, nil
			}

			return cache.ProjectIDs, nil
		},
		NotFoundErr: nil,
		LogMeta: sfLogMeta{
			"get user's projects id list",
			[]zap.Field{
				zap.String("cache_key", key),
				zap.Uint64("user_id", userID),
			},
		},
	}

	projectIDs, _, err := loadObjectByBundleWithSF(ctx, config)
	if projectIDs == nil {
		projectIDs = []uint64{}
	}
	return projectIDs, err
}

// GetUserProjectList 获取用户的项目查询列表
func (s *CacheService) GetUserProjectList(ctx context.Context, query *cacheObj.UserProjectListQuery) ([]*model.Project, *int64, bool, error) {
	if query == nil {
		return []*model.Project{}, utils.Int64Ptr(0), false, nil
	}

	querySnapshot := utils.SafePtrClone(query)
	key := s.userCache.ProjectListKey(querySnapshot) // 构造 key

	return loadAndReturnListPage(
		ctx,
		&s.sf,
		key,
		querySnapshot.ToRepository(),
		s.userCache.GetProjectListPage,
		s.userCache.SetProjectListPage,
		s.pr.SearchProjects,
		buildListPage,
		func(ctx context.Context, items []*model.Project) {
			// 获取 owner 信息写入缓存
			users := make([]*model.User, 0, len(items))
			userSeen := make(map[uint64]struct{}, len(items))
			for _, project := range items {
				if project == nil || project.Owner == nil {
					continue
				}
				if _, ok := userSeen[project.Owner.ID]; !ok {
					users = append(users, project.Owner)
					userSeen[project.Owner.ID] = struct{}{}
				}
			}
			// 批量写入
			s.userCache.MSetBriefInfo(ctx, users)
		},
		func(ctx context.Context, projectIDs []uint64) ([]*model.Project, error) {
			return s.BuildBriefProjectModelsFromPage(ctx, projectIDs, true)
		},
		querySnapshot.Page,
		querySnapshot.PageSize,
		func(val *model.Project) uint64 {
			if val == nil {
				return 0
			}
			return val.ID
		},
		sfLogMeta{
			Message: "invalid user project list bundle",
			Fields: []zap.Field{
				zap.String("cache_key", key),
			},
		},
	)
}

// BumpUserProjectList 更新版本号
func (s *CacheService) BumpUserProjectList(ctx context.Context) error {
	return s.userCache.BumpUserProjectList(ctx)
}

// DeleteUserCache 删除用户模块相关缓存
func (s *CacheService) DeleteUserCache(ctx context.Context, userID uint64) error {
	return s.userCache.DeleteAll(ctx, userID)
}

//==========
// 项目模块
//==========

// ProjectExists 判断项目是否存在
func (s *CacheService) ProjectExists(ctx context.Context, projectID uint64) (bool, error) {
	if projectID == 0 {
		return false, nil
	}

	key := s.projectCache.ExistsKey(projectID)

	return loadExistsWithSF(
		ctx,
		&s.sf,
		key,
		func(ctx context.Context) (bool, bool) {
			return s.projectCache.GetExists(ctx, projectID)
		},
		func(ctx context.Context, exists bool) {
			s.projectCache.SetExists(ctx, projectID, exists)
		},
		func(ctx context.Context) (bool, error) {
			return s.pr.ExistsByProjectID(ctx, projectID)
		},
		repository.ErrProjectNotFound,
		sfLogMeta{
			Message: "invalid project exists singleflight result",
			Fields: []zap.Field{
				zap.Uint64("project_id", projectID),
				zap.String("cache_key", key),
			},
		},
	)
}

// DeleteProjectExistsCache 删除项目是否存在缓存
//   - 注意：删除缓存失败由 ProjectCache 内部记录日志
func (s *CacheService) DeleteProjectExistsCache(ctx context.Context, projectID uint64) error {
	return s.projectCache.DeleteExists(ctx, projectID)
}

// GetProjectBriefInfo 获取项目简要信息
func (s *CacheService) GetProjectBriefInfo(ctx context.Context, projectID uint64) (*model.Project, bool, error) {
	if s == nil || projectID == 0 {
		return nil, false, nil
	}

	key := s.projectCache.BriefInfoKey(projectID)

	config := &objectSFConfig[*model.Project, *cacheObj.ProjectBriefInfo]{
		SF:  &s.sf,
		Key: key,

		GetCache: func(ctx context.Context) (*cacheObj.ProjectBriefInfo, bool, bool) {
			return s.projectCache.GetBriefInfo(ctx, projectID)
		},

		IsCacheValid: func(info *cacheObj.ProjectBriefInfo) bool {
			return info != nil
		},

		LoadDB: func(ctx context.Context) (*model.Project, error) {
			return s.pr.GetByID(ctx, projectID)
		},

		IsItemValid: func(project *model.Project) bool {
			return project != nil && project.ID != 0
		},

		SetCache: func(ctx context.Context, project *model.Project) {
			s.projectCache.SetBriefInfo(ctx, project)
		},

		SetNull: func(ctx context.Context) {
			s.projectCache.SetBriefInfoNull(ctx, projectID)
		},

		DelCache: func(ctx context.Context) {
			s.projectCache.DeleteBriefInfo(ctx, projectID)
		},

		BuildFromCache: func(ctx context.Context, info *cacheObj.ProjectBriefInfo) (*model.Project, error) {
			if info == nil {
				return nil, nil
			}

			return info.ToModel(), nil
		},

		NotFoundErr: repository.ErrProjectNotFound,

		LogMeta: sfLogMeta{
			Message: "invalid project brief info bundle",
			Fields: []zap.Field{
				zap.Uint64("project_id", projectID),
				zap.String("cache_key", key),
			},
		},
	}

	return loadObjectByBundleWithSF(ctx, config)
}

// DeleteProjectBriefInfoCache 删除项目简要信息缓存
//   - 注意：删除缓存失败由 ProjectCache 内部记录日志
func (s *CacheService) DeleteProjectBriefInfoCache(ctx context.Context, projectID uint64) error {
	return s.projectCache.DeleteBriefInfo(ctx, projectID)
}

// DeleteProjectCache 删除项目模块相关缓存
//   - 注意：删除缓存失败由 ProjectCache 内部记录日志
func (s *CacheService) DeleteProjectCache(ctx context.Context, projectID uint64) error {
	return s.projectCache.DeleteAll(ctx, projectID)
}

//==========
// 项目成员模块
//==========

// projectMemberRoleSFResult 项目成员角色单飞结果
type projectMemberRoleSFResult struct {
	Role   string
	Exists bool
}

// IsProjectMember 判断用户是否是项目成员
//   - 逻辑：复用 GetProjectMemberRole，底层只维护一份 role 缓存
//   - 注意：role 存在即表示用户是项目成员
func (s *CacheService) IsProjectMember(ctx context.Context, projectID, userID uint64) (bool, error) {
	_, exists, err := s.GetProjectMemberRole(ctx, projectID, userID)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// GetProjectMemberRole 获取项目成员角色
func (s *CacheService) GetProjectMemberRole(ctx context.Context, projectID, userID uint64) (string, bool, error) {
	if projectID == 0 || userID == 0 {
		return "", false, nil
	}

	key := s.projectMemberCache.RoleKey(projectID, userID)

	// 第一次读取缓存
	if role, exists, hit := s.projectMemberCache.GetRole(ctx, projectID, userID); hit {
		return role, exists, nil
	}

	// 缓存未命中后，使用 singleflight 合并同一个 key 的并发回源请求
	value, err, _ := s.sf.Do(key, func() (any, error) {
		// singleflight 内部二次读取缓存，避免等待期间其它请求已经写回缓存
		if role, exists, hit := s.projectMemberCache.GetRole(ctx, projectID, userID); hit {
			return projectMemberRoleSFResult{
				Role:   role,
				Exists: exists,
			}, nil
		}

		// 二次缓存仍未命中，查询 DB
		role, err := s.pmr.GetProjectMemberRoleByProjectIDAndUserID(ctx, projectID, userID)
		if err != nil {
			if errors.Is(err, repository.ErrProjectMemberNotFound) {
				s.projectMemberCache.SetRoleNull(ctx, projectID, userID)

				return projectMemberRoleSFResult{
					Role:   "",
					Exists: false,
				}, nil
			}

			return nil, err
		}

		role = strings.TrimSpace(role)

		// DB 中不存在该项目成员，写入空值缓存
		if role == "" {
			s.projectMemberCache.SetRoleNull(ctx, projectID, userID)

			return projectMemberRoleSFResult{
				Role:   "",
				Exists: false,
			}, nil
		}

		// 写入项目成员角色缓存
		s.projectMemberCache.SetRole(ctx, projectID, userID, role)

		return projectMemberRoleSFResult{
			Role:   role,
			Exists: true,
		}, nil
	})
	if err != nil {
		return "", false, err
	}

	result, ok := value.(projectMemberRoleSFResult)
	if !ok {
		zap.L().Warn("invalid project member role singleflight result",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
		)
		return "", false, nil
	}

	return result.Role, result.Exists, nil
}

// DeleteProjectMemberRoleCache 删除项目成员角色缓存
//   - 注意：删除缓存失败由 ProjectMemberCache 内部记录日志
func (s *CacheService) DeleteProjectMemberRoleCache(ctx context.Context, projectID, userID uint64) error {
	return s.projectMemberCache.DeleteRole(ctx, projectID, userID)
}

// DeleteProjectMemberCache 删除项目成员模块相关缓存
//   - 注意：当前项目成员模块底层只维护 role 缓存
//   - 注意：删除缓存失败由 ProjectMemberCache 内部记录日志
func (s *CacheService) DeleteProjectMemberCache(ctx context.Context, projectID, userID uint64) error {
	return s.projectMemberCache.DeleteAll(ctx, projectID, userID)
}

//================
// 任务模块
//================

// GetTaskPermissionInfo 获取任务权限信息
func (s *CacheService) GetTaskPermissionInfo(ctx context.Context, taskID uint64) (*model.Task, bool, error) {
	if s == nil || taskID == 0 {
		return nil, false, nil
	}

	key := s.taskCache.PermissionInfoKey(taskID)

	config := &objectSFConfig[*model.Task, *cacheObj.TaskPermissionInfo]{
		SF:  &s.sf,
		Key: key,

		GetCache: func(ctx context.Context) (*cacheObj.TaskPermissionInfo, bool, bool) {
			return s.taskCache.GetPermissionInfo(ctx, taskID)
		},

		IsCacheValid: func(info *cacheObj.TaskPermissionInfo) bool {
			return info != nil
		},

		LoadDB: func(ctx context.Context) (*model.Task, error) {
			return s.tr.GetByID(ctx, taskID)
		},

		IsItemValid: func(task *model.Task) bool {
			return task != nil && task.ID != 0
		},

		SetCache: func(ctx context.Context, task *model.Task) {
			s.taskCache.SetPermissionInfo(ctx, task)
		},

		SetNull: func(ctx context.Context) {
			s.taskCache.SetPermissionInfoNull(ctx, taskID)
		},

		DelCache: func(ctx context.Context) {
			s.taskCache.DeletePermissionInfo(ctx, taskID)
		},

		BuildFromCache: func(ctx context.Context, info *cacheObj.TaskPermissionInfo) (*model.Task, error) {
			if info == nil {
				return nil, nil
			}

			return info.ToModel(), nil
		},

		NotFoundErr: repository.ErrTaskNotFound,

		LogMeta: sfLogMeta{
			Message: "invalid task permission info bundle",
			Fields: []zap.Field{
				zap.Uint64("task_id", taskID),
				zap.String("cache_key", key),
			},
		},
	}

	return loadObjectByBundleWithSF(ctx, config)
}

// DeleteTaskPermissionInfoCache 删除任务权限信息缓存
//   - 注意：删除缓存失败由 TaskCache 内部记录日志
func (s *CacheService) DeleteTaskPermissionInfoCache(ctx context.Context, taskID uint64) error {
	return s.taskCache.DeletePermissionInfo(ctx, taskID)
}

// GetTaskDetailInfo 获取任务详情信息
//   - 逻辑：先查缓存，缓存未命中进入 singleflight，singleflight 内二次查缓存，仍未命中再查 DB，最后写回缓存
//   - 注意：
//     1. 返回值 task 表示任务详情信息
//     2. 返回值 exists 表示任务是否存在
//     3. 返回值 err 只表示系统错误
//     4. repository.ErrTaskNotFound 不作为错误返回，对外返回 nil, false, nil
//     5. 缓存 Get / Set / Del 失败只记录日志，不影响正常业务逻辑
func (s *CacheService) GetTaskDetailInfo(ctx context.Context, taskID uint64) (*model.Task, bool, error) {
	if taskID == 0 {
		return nil, false, nil
	}

	key := s.taskCache.DetailInfoKey(taskID)
	config := &objectSFConfig[*model.Task, *cacheObj.TaskDetailInfo]{
		SF:  &s.sf,
		Key: key,
		GetCache: func(ctx context.Context) (*cacheObj.TaskDetailInfo, bool, bool) {
			return s.taskCache.GetDetailInfo(ctx, taskID)
		},
		IsCacheValid: func(tdi *cacheObj.TaskDetailInfo) bool {
			return tdi != nil
		},
		LoadDB: func(ctx context.Context) (*model.Task, error) {
			return s.tr.GetDetailByID(ctx, taskID)
		},
		IsItemValid: func(t *model.Task) bool {
			return t != nil && t.ID != 0
		},
		SetCache: s.taskCache.SetDetailInfo,
		SetNull: func(ctx context.Context) {
			s.taskCache.SetDetailInfoNull(ctx, taskID)
		},
		DelCache: func(ctx context.Context) {
			s.taskCache.DeleteDetailInfo(ctx, taskID)
		},
		AfterLoadDB: func(ctx context.Context, item *model.Task) {
			if item.Project != nil {
				s.projectCache.SetBriefInfo(ctx, item.Project)
			}
			if item.Assignee != nil {
				s.userCache.SetBriefInfo(ctx, item.Assignee)
			}
			if item.Creator != nil {
				s.userCache.SetBriefInfo(ctx, item.Creator)
			}
		},
		BuildFromCache: s.BuildTaskDetailModelsFromCache,
		NotFoundErr:    repository.ErrTaskNotFound,
		LogMeta: sfLogMeta{
			Message: "invalid task detail bundle",
			Fields: []zap.Field{
				zap.Uint64("task_id", taskID),
				zap.String("cache_key", key),
			},
		},
	}

	return loadObjectByBundleWithSF(ctx, config)
}

// DeleteTaskDetailInfoCache 删除任务权限信息缓存
//   - 注意：删除缓存失败由 TaskCache 内部记录日志
func (s *CacheService) DeleteTaskDetailInfoCache(ctx context.Context, taskID uint64) error {
	return s.taskCache.DeleteDetailInfo(ctx, taskID)
}

// GetTaskListItem 获取任务列表 item
func (s *CacheService) GetTaskListItem(ctx context.Context, taskID uint64) (*model.Task, bool, error) {
	if s == nil || taskID == 0 {
		return nil, false, nil
	}

	key := s.taskCache.ListItemKey(taskID)

	config := &objectSFConfig[*model.Task, *cacheObj.TaskListItem]{
		SF:  &s.sf,
		Key: key,

		GetCache: func(ctx context.Context) (*cacheObj.TaskListItem, bool, bool) {
			return s.taskCache.GetListItem(ctx, taskID)
		},

		IsCacheValid: func(item *cacheObj.TaskListItem) bool {
			return item != nil
		},

		LoadDB: func(ctx context.Context) (*model.Task, error) {
			return s.tr.GetByID(ctx, taskID)
		},

		IsItemValid: func(task *model.Task) bool {
			return task != nil && task.ID != 0
		},

		SetCache: func(ctx context.Context, task *model.Task) {
			s.taskCache.SetListItem(ctx, task)
		},

		SetNull: func(ctx context.Context) {
			s.taskCache.SetListItemNull(ctx, taskID)
		},

		DelCache: func(ctx context.Context) {
			s.taskCache.DeleteListItem(ctx, taskID)
		},

		BuildFromCache: func(ctx context.Context, item *cacheObj.TaskListItem) (*model.Task, error) {
			if item == nil {
				return nil, nil
			}

			return item.ToModel(), nil
		},

		NotFoundErr: repository.ErrTaskNotFound,

		LogMeta: sfLogMeta{
			Message: "invalid task list item bundle",
			Fields: []zap.Field{
				zap.Uint64("task_id", taskID),
				zap.String("cache_key", key),
			},
		},
	}

	return loadObjectByBundleWithSF(ctx, config)
}

// DeleteTaskListItemCache 删除任务列表 item 缓存
//   - 注意：删除缓存失败由 TaskCache 内部记录日志
func (s *CacheService) DeleteTaskListItemCache(ctx context.Context, taskID uint64) error {
	return s.taskCache.DeleteListItem(ctx, taskID)
}

// taskListSFBundle 项目任务列表加载结果
type taskListSFBundle = listSFBundle[*model.Task]

// GetProjectTaskList 获取项目任务列表 model
// 说明：
//   - 这是对外真正返回业务 model 的方法
func (s *CacheService) GetProjectTaskList(ctx context.Context, query *cacheObj.TaskListQuery) ([]*model.Task, *int64, bool, error) {
	if query == nil {
		return []*model.Task{}, utils.Int64Ptr(0), false, nil
	}

	querySnapshot := utils.SafePtrClone(query)
	key := s.taskCache.ListKey(querySnapshot) // 构造 key

	return loadAndReturnListPage(
		ctx,
		&s.sf,
		key,
		querySnapshot.ToRepositoryQuery(),
		s.taskCache.GetListPage,
		s.taskCache.SetListPage,
		s.tr.SearchTasks,
		buildListPage,
		func(ctx context.Context, tasks []*model.Task) { // 加载缓存
			if len(tasks) == 0 {
				return
			}

			// 1. 缓存 task item
			s.taskCache.MSetListItem(ctx, tasks)

			// 2. 顺手缓存预加载 brief + 去重
			projects := make([]*model.Project, 0, len(tasks))
			projectSeen := make(map[uint64]struct{}, len(tasks))
			users := make([]*model.User, 0, len(tasks))
			userSeen := make(map[uint64]struct{}, len(tasks))

			for _, task := range tasks {
				if task == nil {
					continue
				}
				if task.Project != nil {
					if _, ok := projectSeen[task.Project.ID]; !ok {
						projects = append(projects, task.Project)
						projectSeen[task.Project.ID] = struct{}{}
					}
				}
				if task.Creator != nil {
					if _, ok := userSeen[task.Creator.ID]; !ok {
						users = append(users, task.Creator)
						userSeen[task.Creator.ID] = struct{}{}
					}
				}
				if task.Assignee != nil {
					if _, ok := userSeen[task.Assignee.ID]; !ok {
						users = append(users, task.Assignee)
						userSeen[task.Assignee.ID] = struct{}{}
					}
				}
			}

			s.projectCache.MSetBriefInfo(ctx, projects)
			s.userCache.MSetBriefInfo(ctx, users)
		},
		s.BuildProjectTaskModelsFromPage,
		querySnapshot.Page,
		querySnapshot.PageSize,
		func(task *model.Task) uint64 {
			if task == nil {
				return 0
			}
			return task.ID
		},
		sfLogMeta{
			Message: "invalid project task list bundle",
			Fields: []zap.Field{
				zap.String("cache_key", key),
			},
		},
	)
}

// GetUserTaskList 获取用户任务列表 model
func (s *CacheService) GetUserTaskList(ctx context.Context, query *cacheObj.TaskListQuery) ([]*model.Task, *int64, bool, error) {
	if query == nil {
		return []*model.Task{}, utils.Int64Ptr(0), false, nil
	}

	querySnapshot := utils.SafePtrClone(query)
	key := s.taskCache.ListKey(querySnapshot) // 构造 key

	return loadAndReturnListPage(
		ctx,
		&s.sf,
		key,
		querySnapshot.ToRepositoryQuery(),
		s.taskCache.GetListPage,
		s.taskCache.SetListPage,
		s.tr.SearchTasks,
		buildListPage,
		func(ctx context.Context, tasks []*model.Task) { // 加载缓存
			if len(tasks) == 0 {
				return
			}

			// 1. 缓存 task item
			s.taskCache.MSetListItem(ctx, tasks)

			// 2. 顺手缓存预加载 brief + 去重
			projects := make([]*model.Project, 0, len(tasks))
			projectSeen := make(map[uint64]struct{}, len(tasks))
			users := make([]*model.User, 0, len(tasks))
			userSeen := make(map[uint64]struct{}, len(tasks))

			for _, task := range tasks {
				if task == nil {
					continue
				}
				if task.Project != nil {
					if _, ok := projectSeen[task.Project.ID]; !ok {
						projects = append(projects, task.Project)
						projectSeen[task.Project.ID] = struct{}{}
					}
				}
				if task.Creator != nil {
					if _, ok := userSeen[task.Creator.ID]; !ok {
						users = append(users, task.Creator)
						userSeen[task.Creator.ID] = struct{}{}
					}
				}
				if task.Assignee != nil {
					if _, ok := userSeen[task.Assignee.ID]; !ok {
						users = append(users, task.Assignee)
						userSeen[task.Assignee.ID] = struct{}{}
					}
				}
			}

			s.projectCache.MSetBriefInfo(ctx, projects)
			s.userCache.MSetBriefInfo(ctx, users)
		},
		s.BuildUserTaskModelsFromPage,
		querySnapshot.Page,
		querySnapshot.PageSize,
		func(task *model.Task) uint64 {
			if task == nil {
				return 0
			}
			return task.ID
		},
		sfLogMeta{
			Message: "invalid user task list bundle",
			Fields: []zap.Field{
				zap.String("cache_key", key),
			},
		},
	)
}

// BumpTaskListPage 更新版本号（删除缓存）
func (s *CacheService) BumpTaskListPage(ctx context.Context) error {
	return s.taskCache.BumpListPage(ctx)
}

// DeleteTaskAllCache 删除任务的所有缓存
func (s *CacheService) DeleteTaskAllCache(ctx context.Context, taskID uint64) error {
	return s.taskCache.DeleteAll(ctx, taskID)
}
