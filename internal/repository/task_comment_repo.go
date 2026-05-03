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
	TaskID   uint64 // 任务ID
	Page     int    // 页码
	PageSize int    // 页码大小
}

// SearchComments 搜索任务评论列表
//   - 默认按照创建时间排序
func (r *taskCommentRepository) SearchComments(ctx context.Context, query *SearchTaskCommentsQuery) ([]*model.TaskComment, int64, error) {
	if query == nil {
		return nil, 0, ErrInvalidTaskCommentParam
	}
	// 完全相信上层参数
	db := getDB(ctx, r.db, nil).
		Model(&model.TaskComment{}).
		Where(model.TaskCommentColumnTaskID+" = ?", query.TaskID)

	// 统计总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total <= 0 {
		return []*model.TaskComment{}, 0, nil
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	comments := make([]*model.TaskComment, 0, query.PageSize)

	listDB := db.
		Preload(model.TaskCommentAssocAuthor).
		Preload(model.TaskCommentAssocReplyToUser).
		Offset(offset).
		Limit(query.PageSize).
		Order(model.TaskCommentColumnCreatedAt + " DESC")

	if err := listDB.Find(&comments).Error; err != nil {
		return nil, 0, err
	}

	return comments, total, nil
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

// GetCommentsByIDsIncludeDeleted 批量查询评论，包括已软删除评论
func (r *taskCommentRepository) GetCommentsByIDsIncludeDeleted(
	ctx context.Context,
	ids []uint64,
) ([]*model.TaskComment, error) {
	if len(ids) == 0 {
		return []*model.TaskComment{}, nil
	}

	var comments []*model.TaskComment

	err := r.db.WithContext(ctx).
		Unscoped(). // 查询包含软删除数据
		Where("id IN ?", ids).
		Find(&comments).Error

	if err != nil {
		return nil, err
	}

	return comments, nil
}
