// internal/repository/project_member_repo.go
// Package repository 实现 project_member 数据表相关操作
package repository

import (
	"context"
	"errors"

	"smart-task-platform/internal/model"

	"gorm.io/gorm"
)

var (
	ErrProjectMemberIsEmpty      = errors.New("project member cannot be empty")      // 项目成员对象为空
	ErrProjectMemberQueryInvalid = errors.New("project member query is invalid")     // 项目成员查询参数非法
	ErrProjectMemberUpdateEmpty  = errors.New("project member update data is empty") // 项目成员更新参数为空
	ErrInvalidProjectMemberRole  = errors.New("invalid project member role")         // 非法的成员角色
	ErrProjectMemberNotFound     = errors.New("project member not found")            // 项目成员未找到
)

// projectMemberRepository 项目成员仓储
type projectMemberRepository struct {
	db *gorm.DB
}

// NewProjectMemberRepository 创建项目成员仓储
func NewProjectMemberRepository(db *gorm.DB) *projectMemberRepository {
	return &projectMemberRepository{db: db}
}

// CreateWithTx 在项目成员表中新增一条记录
func (r *projectMemberRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, projectMember *model.ProjectMember) error {
	if projectMember == nil {
		return ErrProjectMemberIsEmpty
	}

	return getDB(ctx, r.db, tx).
		Create(projectMember).
		Error
}

// GetProjectMemberByProjectIDAndUserID 根据项目 ID 和用户 ID 获取项目成员详情
//
// 该方法非常适合上层做鉴权：判断当前用户是否属于项目、角色是什么
func (r *projectMemberRepository) GetProjectMemberByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (*model.ProjectMember, error) {
	if projectID <= 0 || userID <= 0 {
		return nil, ErrProjectMemberQueryInvalid
	}

	var projectMember model.ProjectMember
	err := getDB(ctx, r.db, nil).
		Preload(model.ProjectMemberAssocUser).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		First(&projectMember).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectMemberNotFound
		}
		return nil, err
	}

	return &projectMember, nil
}

// ExistsByProjectIDAndUserID 判断用户是否为项目成员
// 上层在做权限前置校验时会经常用到
func (r *projectMemberRepository) ExistsByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (bool, error) {
	if projectID == 0 || userID == 0 {
		return false, ErrProjectMemberQueryInvalid
	}

	var count int64
	err := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// ListProjectIDsByUserID 根据用户ID查询该用户相关的所有项目ID
func (r *projectMemberRepository) ListProjectIDsByUserID(ctx context.Context, userID uint64) ([]uint64, error) {
	if userID <= 0 {
		return nil, ErrProjectMemberQueryInvalid
	}

	var projectIDs []uint64

	// 从 project_members 表查询该用户的所有项目ID
	err := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Select(model.ProjectColumnID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Pluck(model.ProjectMemberColumnProjectID, &projectIDs).Error

	if err != nil {
		return nil, err
	}

	return projectIDs, nil
}
