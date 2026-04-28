// internal/repository/project_member_repo.go
// Package repository 实现 project_member 数据表相关操作
package repository

import (
	"context"
	"errors"
	"fmt"

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

// ProjectMemberSearchQuery 项目成员列表查询参数
type ProjectMemberSearchQuery struct {
	Page      int    // 页码
	PageSize  int    // 每页数量
	ProjectID uint64 // 项目 ID
	Role      string // 成员角色
	Keyword   string // 关键词：用户名/昵称模糊匹配
}

// SearchProjectMembers 搜索项目成员
// 支持：
//  1. 按项目 ID 查询
//  2. 按角色筛选
//  3. 按用户名/昵称模糊查询
//  4. 分页
func (r *projectMemberRepository) SearchProjectMembers(ctx context.Context, query *ProjectMemberSearchQuery) ([]*model.ProjectMember, int64, error) {
	if query == nil || query.ProjectID <= 0 {
		return nil, 0, ErrProjectMemberQueryInvalid
	}
	// 相信上层传入的参数

	db := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Joins("LEFT JOIN users ON users.id = project_members.user_id").
		Where("project_members.project_id = ?", query.ProjectID)

	// 按角色筛选
	if query.Role != "" {
		db = db.Where("project_members.role = ?", query.Role)
	}

	// 按用户名 / 昵称模糊匹配
	if query.Keyword != "" {
		like := query.Keyword + "%"
		db = db.Where("(users.username LIKE ?)", like)
	}

	// 统计总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total <= 0 {
		return []*model.ProjectMember{}, 0, nil
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	projectMembers := make([]*model.ProjectMember, 0, query.PageSize)

	// 角色排序：owner > admin > member
	roleOrder := fmt.Sprintf(
		"FIELD(%s.%s, '%s', '%s', '%s')",
		model.ProjectMemberTableName,
		model.ProjectMemberColumnRole,
		model.ProjectMemberRoleOwner,
		model.ProjectMemberRoleAdmin,
		model.ProjectMemberRoleMember,
	)

	err := db.
		Preload(model.ProjectMemberAssocUser).
		Order(roleOrder).
		Order(model.ProjectMemberTableName + "." + model.ProjectMemberColumnJoinedAt + " DESC").
		Offset(offset).
		Limit(query.PageSize).
		Find(&projectMembers).Error
	if err != nil {
		return nil, 0, err
	}

	return projectMembers, total, nil
}

// UpdateProjectMemberRole 更新指定项目成员角色
func (r *projectMemberRepository) UpdateProjectMemberRole(ctx context.Context, tx *gorm.DB, projectID, userID uint64, role string) error {
	if projectID <= 0 || userID <= 0 {
		return ErrProjectMemberQueryInvalid
	}

	if role == "" {
		return nil
	}

	return getDB(ctx, r.db, tx).
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Update(model.ProjectMemberColumnRole, role).Error
}

// RemoveProjectMember 从项目中移除成员
func (r *projectMemberRepository) RemoveProjectMember(ctx context.Context, tx *gorm.DB, projectID, userID uint64) error {
	if projectID <= 0 || userID <= 0 {
		return ErrProjectMemberQueryInvalid
	}

	return getDB(ctx, r.db, tx).
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Delete(&model.ProjectMember{}).Error
}

// GetProjectMemberRoleByProjectIDAndUserID 获取指定成员在项目中的角色
// 上层若只关心角色，可避免加载完整对象
func (r *projectMemberRepository) GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error) {
	if projectID <= 0 || userID <= 0 {
		return "", ErrProjectMemberQueryInvalid
	}

	var role string
	err := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Select(model.ProjectMemberColumnRole).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Take(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrProjectMemberNotFound
		}
		return "", err
	}

	return role, nil
}

// CountByProjectIDAndRole 统计指定项目下某个角色的成员数量
// 上层可用于：判断 owner 是否至少保留一个
func (r *projectMemberRepository) CountByProjectIDAndRole(ctx context.Context, projectID uint64, role string) (int64, error) {
	if projectID <= 0 {
		return 0, ErrProjectMemberQueryInvalid
	}
	if role == "" {
		return 0, ErrInvalidProjectMemberRole
	}

	var count int64
	err := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnRole+" = ?", role).
		Count(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
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
	if projectID <= 0 || userID <= 0 {
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
		Select(model.ProjectMemberColumnProjectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Pluck(model.ProjectMemberColumnProjectID, &projectIDs).Error

	if err != nil {
		return nil, err
	}

	return projectIDs, nil
}
