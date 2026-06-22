// internal/service/task_activities_service.go
// Package service
// 任务活动服务

package service

import (
	"context"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"

	"go.uber.org/zap"
)

// taskActivitySvcActivityRepo 任务活动接口
type taskActivitySvcActivityRepo interface {
	SearchTaskActivities(ctx context.Context, query *repository.SearchTaskActivitiesQuery) (*repository.SearchTaskActivitiesResult, error)
}

// cacheTaskActivityInvoker
type cacheTaskActivityInvoker interface {
	IsProjectMember(ctx context.Context, projectID, userID uint64) (bool, error)
	GetProjectMemberRole(ctx context.Context, projectID, userID uint64) (string, bool, error)

	GetTaskPermissionInfo(ctx context.Context, taskID uint64) (*model.Task, bool, error)
}

// TaskActivityService 任务活动服务
type TaskActivityService struct {
	tar   taskActivitySvcActivityRepo // 任务活动接口
	store cacheTaskActivityInvoker    // 缓存调用接口
}

// NewTaskActivityService 创建任务活动服务实例
func NewTaskActivityService(
	taskActivitiesRepo taskActivitySvcActivityRepo,
	store cacheTaskActivityInvoker,
) *TaskActivityService {
	return &TaskActivityService{
		tar:   taskActivitiesRepo,
		store: store,
	}
}

// ListTaskActivitiesParam 查询任务评论列表参数
type ListTaskActivitiesParam struct {
	UserID    uint64 // 用户ID 需要是该项目成员
	ProjectID uint64 // 项目ID
	TaskID    uint64 // 任务ID
	Page      int    // 页码
	PageSize  int    // 数量
	NeedTotal bool   // 是否查询总数
}

// ListTaskActivities 查询任务活动列表
func (s *TaskActivityService) ListTaskActivities(ctx context.Context, param *ListTaskActivitiesParam) (*dto.ListTaskActivitiesResp, error) {
	logger := zap.L()

	// 参数校验
	if param == nil {
		logger.Warn("list task activities failed: invalid nil param")
		return nil, ErrInvalidTaskActivityParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("task_id", param.TaskID),
	)

	if param.UserID == 0 ||
		param.ProjectID == 0 ||
		param.TaskID == 0 {
		logger.Warn("list task activities failed: invalid user_id, project_id or task_id")
		return nil, ErrInvalidTaskActivityParam
	}

	// 参数修正
	page, pageSize := fixPageParams(param.Page, param.PageSize)
	logger = logger.With(
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
	)

	// 数据库进行身份验证
	// 项目是否存在、任务是否存在、用户是否是项目成员
	// validTaskAccess 都完成了
	task, err := validTaskAccess(ctx, s.store, param.ProjectID, param.TaskID, param.UserID, logger)
	if err != nil {
		// 调用返回两个错误：ErrTaskNotFound、ErrTaskForbidden
		logger.Warn("list task activities failed: task activity access check error",
			zap.Error(err),
		)
		return nil, err
	}

	// 搜索状态
	result, err := s.tar.SearchTaskActivities(ctx, &repository.SearchTaskActivitiesQuery{
		TaskID: task.ID,
		SearchQuery: repository.SearchQuery{
			Page:      page,
			PageSize:  pageSize,
			NeedTotal: param.NeedTotal,
		},
	})
	if err != nil {
		logger.Error("list task activities failed: search activities error",
			zap.Error(err),
		)
		return nil, err
	}
	// 搜索成功 构造list
	list := make([]*dto.TaskActivityListItem, 0, len(result.List))
	for _, activity := range result.List {
		list = append(list, buildTaskActivityItem(activity))
	}

	logger.Info("list task activities success",
		zap.Int("list_count", len(list)),
		zap.Bool("has_more", result.HasMore),
	)

	return &dto.ListTaskActivitiesResp{
		List:     list,
		Total:    utils.SafePtrClone(result.Total),
		Page:     page,
		PageSize: pageSize,
		HasMore:  result.HasMore,
	}, nil
}
