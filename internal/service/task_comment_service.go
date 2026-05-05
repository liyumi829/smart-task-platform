// internal/service/comment_service.go
// Package service 实现评论服务

package service

import (
	"context"
	"errors"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/pkg/validator"
	"smart-task-platform/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TaskCommentService 任务评论服务
type TaskCommentService struct {
	txMgr *repository.TxManager           // 事务管理器
	ur    taskCommentSvcUserRepo          // 用户仓储接口
	pmr   taskCommentSVCProjectMemberRepo // 项目成员仓储接口
	tr    taskCommentSVCTaskRepo          // 任务仓储接口
	tcr   taskCommentSVCCommentRepo       // 评论仓储接口
}

// NewTaskCommentService 创建评论服务实例
func NewTaskCommentService(
	txMgr *repository.TxManager,
	userRepo taskCommentSvcUserRepo,
	projectMemberRepo taskCommentSVCProjectMemberRepo,
	taskRepo taskCommentSVCTaskRepo,
	taskCommentRepo taskCommentSVCCommentRepo,
) *TaskCommentService {
	return &TaskCommentService{
		txMgr: txMgr,
		ur:    userRepo,
		pmr:   projectMemberRepo,
		tr:    taskRepo,
		tcr:   taskCommentRepo,
	}
}

// CreateTaskCommentParam 创建评论的参数
type CreateTaskCommentParam struct {
	CreatorID       uint64  // 创建人ID 需要是该项目成员
	ProjectID       uint64  // 项目ID
	TaskID          uint64  // 任务ID
	Content         string  // 评论内容
	ParentCommentID *uint64 // 父评论ID：nil表示空
}

// CreateTaskComment 创建任务评论
func (s *TaskCommentService) CreateTaskComment(ctx context.Context, param *CreateTaskCommentParam) (*dto.CreateTaskCommentResp, error) {
	logger := zap.L()

	// 参数校验
	if param == nil {
		logger.Warn("create task comment failed: invalid nil param")
		return nil, ErrInvalidTaskCommentParam
	}

	logger = logger.With(
		zap.Uint64("creator_id", param.CreatorID),
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("task_id", param.TaskID),
	)

	if param.CreatorID == 0 ||
		param.ProjectID == 0 ||
		param.TaskID == 0 {
		logger.Warn("create task comment failed: invalid creator_id, project_id or task_id")
		return nil, ErrInvalidTaskCommentParam
	}

	if param.ParentCommentID != nil {
		logger = logger.With(zap.Uint64("parent_comment_id", *param.ParentCommentID))

		if *param.ParentCommentID == 0 {
			logger.Warn("create task comment failed: invalid parent_comment_id")
			return nil, ErrInvalidTaskCommentParam
		}
	}

	// 评论合法性检查
	if param.Content == "" {
		logger.Warn("create task comment failed: empty content")
		return nil, ErrEmptyTaskCommentContent
	}

	if !validator.IsValidCommentContent(param.Content) {
		logger.Warn("create task comment failed: invalid content")
		return nil, ErrInvalidTaskCommentContent
	}

	var parentCommentID *uint64
	if param.ParentCommentID != nil {
		v := *param.ParentCommentID
		parentCommentID = &v
	}

	// 数据库查询，进行身份验证
	// 获取用户创建者用户信息
	user, err := s.ur.GetByID(ctx, param.CreatorID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			logger.Warn("create task comment failed: creator user not found",
				zap.Error(err),
			)
			return nil, ErrUserNotFound
		}

		logger.Error("create task comment failed: get creator user error",
			zap.Error(err),
		)
		return nil, err
	}

	// 查任务、再看是否是当前项目成员
	task, err := validateTaskCommentAccess(ctx, s.pmr, s.tr, param.ProjectID, param.TaskID, param.CreatorID, logger)
	if err != nil {
		// 返回两个错误：TaskNotFound 和 TaskForbidden
		logger.Warn("create task comment failed: task comment access check error",
			zap.Error(err),
		)
		return nil, err
	}

	var parentComment *model.TaskComment
	if parentCommentID != nil {
		// 说明任务正确且是项目成员，下一步判断父评论是否是当前任务
		parentComment, err = s.tcr.GetCommentByID(ctx, *parentCommentID)
		if err != nil {
			if errors.Is(err, repository.ErrTaskCommentNotFound) {
				// 父评论不存在
				logger.Warn("create task comment failed: parentComment comment not found",
					zap.Error(err),
				)
				return nil, ErrParentCommentNotFound
			}

			logger.Error("create task comment failed: get parentComment comment error",
				zap.Error(err),
			)
			return nil, err
		}

		// 父评论存在，检查父评论是否是当前任务
		if parentComment.TaskID != param.TaskID {
			logger.Warn("create task comment failed: parentComment comment does not belong to task",
				zap.Uint64("parent_comment_task_id", parentComment.TaskID),
			)
			return nil, ErrInvalidParentComment // 返回不合法的父评论
		}
	}

	var replyToUserID *uint64
	var replyToUser *dto.UserPublicProfile
	if parentComment != nil {
		v := parentComment.AuthorID
		replyToUserID = &v

		// 如果有父评论
		repliedUser, err := s.ur.GetByID(ctx, *replyToUserID)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				logger.Warn("create task comment failed: reply to user not found",
					zap.Uint64("reply_to_user_id", *replyToUserID),
					zap.Error(err),
				)
				return nil, ErrUserNotFound
			}

			logger.Error("create task comment failed: get reply to user error",
				zap.Uint64("reply_to_user_id", *replyToUserID),
				zap.Error(err),
			)
			return nil, err
		}

		replyToUser = buildUserPublicProfile(repliedUser)
	}

	// 检验通过，事务创建评论
	comment := &model.TaskComment{
		TaskID:        task.ID,
		AuthorID:      user.ID,
		ParentID:      parentCommentID,
		ReplyToUserID: replyToUserID,
		Content:       param.Content,
	}

	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tcr.CreateCommentWithTx(ctx, tx, comment)
	})
	if err != nil {
		logger.Error("create task comment failed: create comment transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	resp := buildTaskCommentBaseFields(comment) // comment的Author、ReplyToUser不会自动查询
	resp.Author = buildUserPublicProfile(user)
	resp.ReplyToUser = replyToUser

	logger.Info("create task comment success",
		zap.Uint64("comment_id", comment.ID),
		zap.Bool("is_reply", parentCommentID != nil),
	)

	return &dto.CreateTaskCommentResp{
		TaskCommentBaseFields: resp,
	}, nil
}

// ListTaskCommentsParam 查询任务评论列表参数
type ListTaskCommentsParam struct {
	UserID    uint64 // 用户ID 需要是该项目成员
	ProjectID uint64 // 项目ID
	TaskID    uint64 // 任务ID
	Page      int    // 页码
	PageSize  int    // 数量
	NeedTotal bool   // 是否查询总数
}

// ListTaskComments 查询任务评论列表
func (s *TaskCommentService) ListTaskComments(ctx context.Context, param *ListTaskCommentsParam) (*dto.ListTaskCommentsResp, error) {
	logger := zap.L()

	// 参数校验
	if param == nil {
		logger.Warn("list task comments failed: invalid nil param")
		return nil, ErrInvalidTaskCommentParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("task_id", param.TaskID),
	)

	if param.UserID == 0 ||
		param.ProjectID == 0 ||
		param.TaskID == 0 {
		logger.Warn("list task comments failed: invalid user_id, project_id or task_id")
		return nil, ErrInvalidTaskCommentParam
	}

	// 参数修正
	page, pageSize := fixPageParams(param.Page, param.PageSize)
	logger = logger.With(
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
	)

	// 数据库进行身份验证
	// 用户是否存在、项目是否存在、任务是否存在、用户是否是项目成员
	// validateTaskCommentAccess 都完成了
	task, err := validateTaskCommentAccess(ctx, s.pmr, s.tr, param.ProjectID, param.TaskID, param.UserID, logger)
	if err != nil {
		// 调用返回两个错误：ErrTaskNotFound、ErrTaskForbidden
		logger.Warn("list task comments failed: task comment access check error",
			zap.Error(err),
		)
		return nil, err
	}

	// 搜索评论
	result, err := s.tcr.SearchComments(ctx, &repository.SearchTaskCommentsQuery{
		TaskID: task.ID,
		SearchQuery: repository.SearchQuery{
			Page:      page,
			PageSize:  pageSize,
			NeedTotal: param.NeedTotal,
		},
	})
	if err != nil {
		logger.Error("list task comments failed: search comments error",
			zap.Error(err),
		)
		return nil, err
	}

	// 收集父评论 ID
	parentIDs := collectTaskCommentParentIDs(result.List)

	// 查询父评论删除状态
	parentDeletedMap, err := s.buildParentDeletedMap(ctx, parentIDs)
	if err != nil {
		return nil, err
	}

	// 搜索成功 构造list
	list := make([]*dto.TaskCommentListItem, 0, len(result.List))
	for _, comment := range result.List {
		list = append(list, buildTaskCommentItem(comment, parentDeletedMap))
	}

	logger.Info("list task comments success",
		zap.Int("list_count", len(list)),
		zap.Bool("has_more", result.HasMore),
	)

	return &dto.ListTaskCommentsResp{
		List:     list,
		Total:    utils.SafePtrClone(result.Total),
		Page:     page,
		PageSize: pageSize,
		HasMore:  result.HasMore,
	}, nil
}

// RemoveTaskCommentParam 删除评论参数
type RemoveTaskCommentParam struct {
	UserID    uint64 // 用户ID 需要是该项目成员
	ProjectID uint64 // 项目ID
	TaskID    uint64 // 任务ID
	CommentID uint64 // 评论ID
}

// RemoveTaskComment 删除评论
func (s *TaskCommentService) RemoveTaskComment(ctx context.Context, param *RemoveTaskCommentParam) (*dto.RemoveTaskCommentResp, error) {
	logger := zap.L()

	// 参数校验
	if param == nil {
		logger.Warn("remove task comment failed: invalid nil param")
		return nil, ErrInvalidTaskCommentParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("task_id", param.TaskID),
		zap.Uint64("comment_id", param.CommentID),
	)

	if param.UserID == 0 ||
		param.ProjectID == 0 ||
		param.TaskID == 0 ||
		param.CommentID == 0 {
		logger.Warn("remove task comment failed: invalid user_id, project_id, task_id or comment_id")
		return nil, ErrInvalidTaskCommentParam
	}

	// 数据库查询权限判定
	// 用户是否存在、项目是否存在、任务是否存在
	// 用户是否是项目的owner/admin，或者是任务创始人

	task, err := validateTaskCommentAccess(ctx, s.pmr, s.tr, param.ProjectID, param.TaskID, param.UserID, logger)
	if err != nil {
		// 返回 ErrTaskNotFound/ErrTaskForbidden/查询数据库失败的err
		logger.Warn("remove task comment failed: task comment access check error",
			zap.Error(err),
		)
		return nil, err
	}

	// 查询评论，校验评论是否存在
	comment, err := s.tcr.GetCommentByID(ctx, param.CommentID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskCommentNotFound) {
			logger.Warn("remove task comment failed: comment not found",
				zap.Error(err),
			)
			return nil, ErrTaskCommentNotFound
		}

		logger.Error("remove task comment failed: get comment error",
			zap.Error(err),
		)
		return nil, err
	}

	// 校验评论是否属于当前任务，避免通过 task_id + comment_id 组合越权删除其他任务评论
	if comment.TaskID != param.TaskID {
		logger.Warn("remove task comment failed: comment does not belong to task",
			zap.Uint64("comment_task_id", comment.TaskID),
		)
		return nil, ErrTaskCommentNotFound
	}

	// 检验用户是否有权限
	hasPermission := task.CreatorID == param.UserID

	if !hasPermission {
		// 如果不是创建者，进行数据库查询，看看是否是owner/admin
		// 返回错误：ProjectMemberNotFound/err
		hasPermission, err = hasProjectManagePermission(ctx, s.pmr, task.ProjectID, param.UserID, logger)
		if err != nil {
			logger.Warn("remove task comment failed: project manage permission check error",
				zap.Error(err),
			)
			return nil, err
		}
	}

	if !hasPermission {
		logger.Warn("remove task comment failed: permission denied",
			zap.Uint64("task_creator_id", task.CreatorID),
			zap.Uint64("comment_author_id", comment.AuthorID),
		)
		return nil, ErrTaskForbidden
	}

	// 事务执行删除操作
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tcr.SoftDeleteCommentWithTx(ctx, tx, param.CommentID)
	})
	if err != nil {
		if errors.Is(err, repository.ErrTaskCommentNotFound) {
			logger.Warn("remove task comment failed: no comment deleted",
				zap.Error(err),
			)
			return nil, ErrTaskCommentNotFound
		}

		logger.Error("remove task comment failed: soft delete comment transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	logger.Info("remove task comment success")

	return &dto.RemoveTaskCommentResp{}, nil
}

// taskCommentSvcUserRepo 用户仓储接口
type taskCommentSvcUserRepo interface {
	GetByID(ctx context.Context, id uint64) (*model.User, error)
}

// taskCommentSVCProjectMemberRepo  判断是否是项目成员接口
type taskCommentSVCProjectMemberRepo interface {
	ExistsByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (bool, error)
	GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error)
}

// taskCommentSVCTaskRepo 任务获取接口
type taskCommentSVCTaskRepo interface {
	GetTaskByID(ctx context.Context, taskID uint64) (*model.Task, error)
}

// taskCommentSVCCommentRepo 评论服务依赖的评论仓储能力
type taskCommentSVCCommentRepo interface {
	CreateCommentWithTx(ctx context.Context, tx *gorm.DB, comment *model.TaskComment) error
	SearchComments(ctx context.Context, query *repository.SearchTaskCommentsQuery) (*repository.SearchTaskCommentResult, error)
	GetCommentByID(ctx context.Context, commentID uint64) (*model.TaskComment, error)
	SoftDeleteCommentWithTx(ctx context.Context, tx *gorm.DB, commentID uint64) error

	GetTaskCommentDeleteStatusByIDs(ctx context.Context, ids []uint64) ([]*model.TaskComment, error)
}

// buildParentDeletedMap 构造父评论删除状态映射
func (s *TaskCommentService) buildParentDeletedMap(
	ctx context.Context,
	parentIDs []uint64,
) (map[uint64]bool, error) {
	parentDeletedMap := make(map[uint64]bool)

	if len(parentIDs) == 0 {
		return parentDeletedMap, nil
	}

	// 注意：这里需要 repository 支持查询包括软删除在内的评论
	parentComments, err := s.tcr.GetTaskCommentDeleteStatusByIDs(ctx, parentIDs)
	if err != nil {
		return nil, err
	}

	parentMap := make(map[uint64]*model.TaskComment, len(parentComments))
	for _, parentComment := range parentComments {
		if parentComment == nil {
			continue
		}
		parentMap[parentComment.ID] = parentComment
	}

	for _, parentID := range parentIDs {
		parentComment, ok := parentMap[parentID]
		if !ok {
			// 理论上不应该出现，除非数据异常或父评论被物理删除
			parentDeletedMap[parentID] = true
			continue
		}

		parentDeletedMap[parentID] = parentComment.DeletedAt.Valid
	}

	return parentDeletedMap, nil
}
