// internal/repository/task_comment_repo.go
// Package repository
// 任务评论表仓储
package repository

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"
	"time"

	"gorm.io/gorm"
)

var (
	ErrInvalidTaskCommentParam = errors.New("invalid task comment param")          // 不合法的任务评论参数
	ErrCreateTaskCommentEmpty  = errors.New("create task params comment is empty") // 创建任务评论的参数为空
	ErrTaskCommentNotFound     = errors.New("task comment not found")              // 任务评论不存在
)

// taskCommentRepository 任务评论仓储
type taskCommentRepository struct {
	db *gorm.DB
}

// NewTaskCommentRepository 创建任务评论仓储实例
func NewTaskCommentRepository(db *gorm.DB) *taskCommentRepository {
	return &taskCommentRepository{
		db: db,
	}
}

// CreateCommentWithTx 用事务创建一条任务评论
func (r *taskCommentRepository) CreateCommentWithTx(ctx context.Context, tx *gorm.DB, comment *model.TaskComment) error {
	if comment == nil {
		return ErrCreateTaskCommentEmpty
	}

	return getDB(ctx, r.db, tx).
		Create(comment).Error
}

// SearchTaskCommentsQuery 搜索任务列表参数
type SearchTaskCommentsQuery struct {
	TaskID uint64 // 任务ID
	SearchQuery
}

type SearchTaskCommentResult = SearchResult[*model.TaskComment]

// SearchComments 搜索任务评论列表。
// 说明：
// 1. 按任务 ID 查询评论
// 2. 默认按照创建时间倒序排序
// 3. total 按需查询，避免每次 COUNT(*) 带来的性能开销
// 4. hasMore 通过 Limit(pageSize + 1) 判断，不依赖 total
func (r *taskCommentRepository) SearchComments(ctx context.Context, query *SearchTaskCommentsQuery) (*SearchTaskCommentResult, error) {
	// 参数校验：query 不能为空，TaskID 必须有效。
	if query == nil || query.TaskID <= 0 {
		return nil, ErrInvalidTaskCommentParam
	}

	// 参数的矫正交给上层，这里只做必要兜底，避免非法分页导致异常 SQL。
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, ErrInvalidTaskCommentParam
	}

	// 构建基础查询条件。
	baseDB := getDB(ctx, r.db, nil).
		Model(&model.TaskComment{}).
		Where(model.TaskCommentColumnTaskID+" = ?", query.TaskID)

	var totalPtr *int64

	// total 按需查询：
	// 通常首次进入、手动刷新、筛选条件变化时 NeedTotal=true；
	// 普通翻页时 NeedTotal=false，避免重复 COUNT(*)。
	if query.NeedTotal {
		var total int64
		if err := baseDB.Count(&total).Error; err != nil {
			return nil, err
		}

		totalPtr = &total

		// 如果总数为 0，直接返回空结果，避免继续查询列表。
		if total == 0 {
			return &SearchTaskCommentResult{
				List:    []*model.TaskComment{},
				Total:   totalPtr,
				HasMore: false,
			}, nil
		}
	}

	// 计算分页偏移量。
	offset := (query.Page - 1) * query.PageSize

	// 多查一条用于判断是否还有下一页。
	limit := query.PageSize + 1

	comments := make([]*model.TaskComment, 0, limit)

	if err := baseDB.
		Preload(model.TaskCommentAssocAuthor, SelectUserFields).
		Preload(model.TaskCommentAssocReplyToUser, SelectUserFields).
		Select([]string{
			model.TaskCommentColumnID,
			model.TaskCommentColumnTaskID,
			model.TaskCommentColumnContent,
			model.TaskCommentColumnAuthorID,
			model.TaskCommentColumnParentID,
			model.TaskCommentColumnReplyToUserID,
			model.TaskCommentColumnCreatedAt,
		}).
		Order(model.TaskCommentColumnCreatedAt + " DESC").
		Order(model.TaskCommentColumnID + " DESC"). // 创建时间相同时使用 ID 兜底排序，保证分页稳定。
		Offset(offset).
		Limit(limit).
		Find(&comments).Error; err != nil {
		return nil, err
	}

	// 通过多查的一条数据判断 hasMore，不依赖 total。
	hasMore := len(comments) > query.PageSize
	if hasMore {
		comments = comments[:query.PageSize]
	}

	return &SearchTaskCommentResult{
		List:    comments,
		Total:   totalPtr,
		HasMore: hasMore,
	}, nil
}

// SoftDeleteCommentWithTx 软删除评论
func (r *taskCommentRepository) SoftDeleteCommentWithTx(ctx context.Context, tx *gorm.DB, commentID uint64) error {
	if commentID == 0 {
		return ErrInvalidTaskCommentParam
	}

	now := time.Now()
	result := getDB(ctx, r.db, tx).
		Model(&model.TaskComment{}).
		Where(model.TaskCommentColumnID+" = ?", commentID).
		Where(model.TaskCommentColumnDeletedAt + " IS NULL").
		Updates(map[string]interface{}{
			model.TaskCommentColumnDeletedAt: now,
			model.TaskCommentColumnUpdatedAt: now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrTaskCommentNotFound
	}

	return nil
}

// GetCommentByID 根据评论 ID 获取评论
func (r *taskCommentRepository) GetCommentByID(ctx context.Context, commentID uint64) (*model.TaskComment, error) {
	if commentID == 0 {
		return nil, ErrInvalidTaskCommentParam
	}

	var comment model.TaskComment
	err := getDB(ctx, r.db, nil).
		Where(model.TaskCommentColumnID+" = ?", commentID).
		First(&comment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskCommentNotFound
		}
		return nil, err
	}

	return &comment, nil
}

// GetTaskCommentDeleteStatusByIDs  批量查询评论，包括已软删除评论
func (r *taskCommentRepository) GetTaskCommentDeleteStatusByIDs(
	ctx context.Context,
	ids []uint64,
) ([]*model.TaskComment, error) {
	if len(ids) == 0 {
		return []*model.TaskComment{}, nil
	}

	var comments []*model.TaskComment

	err := r.db.WithContext(ctx).
		Unscoped().      // 查询包含软删除数据
		Select([]string{ // 查询ID和deleteAt
			model.TaskCommentColumnID,
			model.TaskCommentColumnDeletedAt,
		}).
		Where(model.TaskCommentColumnID+" IN ?", ids).
		Find(&comments).Error

	if err != nil {
		return nil, err
	}

	return comments, nil
}
