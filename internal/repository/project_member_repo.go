// internal/repository/project_member_repo.go
// Package repository 实现 project_member 数据表相关操作
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	SearchQuery
	ProjectID uint64 // 项目 ID
	Role      string // 成员角色
	Keyword   string // 关键词：用户名前缀匹配
}

type SearchProjectMemberResult = SearchResult[*model.ProjectMember]

// SearchProjectMembers 搜索项目成员。
// 支持：
//  1. 按项目 ID 查询
//  2. 按角色筛选
//  3. 按用户名前缀查询
//  4. 分页
//  5. total 按需查询，避免每次 COUNT(*) 带来的性能开销
//  6. hasMore 通过 Limit(pageSize + 1) 判断，不依赖 total
func (r *projectMemberRepository) SearchProjectMembers(ctx context.Context, query *ProjectMemberSearchQuery) (*SearchProjectMemberResult, error) {
	// 参数校验：ProjectID 必须有效。
	if query == nil || query.ProjectID <= 0 {
		return nil, ErrProjectMemberQueryInvalid
	}

	// 参数的矫正交给上层，这里只做必要兜底，避免非法分页导致异常 SQL。
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, ErrProjectMemberQueryInvalid
	}

	// 构建基础查询条件。
	baseDB := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberTableName+"."+model.ProjectMemberColumnProjectID+" = ?", query.ProjectID)

	// 按角色筛选。
	if query.Role != "" {
		baseDB = baseDB.Where(model.ProjectMemberTableName+"."+model.ProjectMemberColumnRole+" = ?", query.Role)
	}

	// 按用户名前缀匹配。
	// 只有需要按 username 查询时才 JOIN users，避免无意义 JOIN。
	if query.Keyword != "" {
		baseDB = baseDB.
			Joins("INNER JOIN "+model.UserTableName+" ON "+model.UserTableName+"."+model.UserColumnID+" = "+model.ProjectMemberTableName+"."+model.ProjectMemberColumnUserID).
			Where(model.UserTableName+"."+model.UserColumnUsername+" LIKE ?", query.Keyword+"%")
	}

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
			return &SearchProjectMemberResult{
				List:    []*model.ProjectMember{},
				Total:   totalPtr,
				HasMore: false,
			}, nil
		}
	}

	// 计算分页偏移量。
	offset := (query.Page - 1) * query.PageSize

	// 多查一条用于判断是否还有下一页。
	limit := query.PageSize + 1

	projectMembers := make([]*model.ProjectMember, 0, limit)

	// 角色排序：owner > admin > member。
	roleOrder := fmt.Sprintf(
		"FIELD(%s.%s, '%s', '%s', '%s')",
		model.ProjectMemberTableName,
		model.ProjectMemberColumnRole,
		model.ProjectMemberRoleOwner,
		model.ProjectMemberRoleAdmin,
		model.ProjectMemberRoleMember,
	)

	if err := baseDB.
		// JOIN users 时只查询 project_members 字段，避免 users 同名字段影响扫描结果。
		Select(model.ProjectMemberTableName+".*").
		Preload(model.ProjectMemberAssocUser, SelectUserFields).
		Order(roleOrder).
		Order(model.ProjectMemberTableName + "." + model.ProjectMemberColumnJoinedAt + " DESC").
		Order(model.ProjectMemberTableName + "." + model.ProjectMemberColumnID + " DESC"). // 排序兜底，保证分页稳定。
		Offset(offset).
		Limit(limit).
		Find(&projectMembers).Error; err != nil {
		return nil, err
	}

	// 通过多查的一条数据判断 hasMore，不依赖 total。
	hasMore := len(projectMembers) > query.PageSize
	if hasMore {
		projectMembers = projectMembers[:query.PageSize]
	}

	return &SearchProjectMemberResult{
		List:    projectMembers,
		Total:   totalPtr,
		HasMore: hasMore,
	}, nil
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

// SoftDeleteProjectMember 从项目中移除成员
func (r *projectMemberRepository) SoftDeleteProjectMember(ctx context.Context, tx *gorm.DB, projectID, userID uint64) error {
	if projectID <= 0 || userID <= 0 {
		return ErrProjectMemberQueryInvalid
	}

	now := time.Now()
	result := getDB(ctx, r.db, tx).
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Where(model.ProjectMemberColumnDeletedAt + " IS NULL").
		Updates(map[string]interface{}{
			model.ProjectMemberColumnDeletedAt: now,
			model.ProjectMemberColumnUpdatedAt: now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrProjectMemberNotFound
	}

	return nil
}

// GetProjectMemberByProjectIDAndUserIDUnscoped 查询项目成员，包括软删除数据
func (r *projectMemberRepository) GetProjectMemberByProjectIDAndUserIDUnscoped(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (*model.ProjectMember, error) {
	var projectMember model.ProjectMember

	err := getDB(ctx, r.db, nil).
		Unscoped().
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		First(&projectMember).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrProjectMemberNotFound
	}

	if err != nil {
		return nil, err
	}

	return &projectMember, nil
}

// RestoreProjectMemberWithTxParam 回复软删除成员参数
type RestoreProjectMemberWithTxParam struct {
	ProjectID uint64
	UserID    uint64
	Role      string
	InvitedBy *uint64
	JoinedAt  time.Time
	UpdatedAt time.Time
}

// RestoreProjectMemberWithTx 恢复软删除项目成员
func (r *projectMemberRepository) RestoreProjectMemberWithTx(ctx context.Context, tx *gorm.DB, param *RestoreProjectMemberWithTxParam) error {

	// 先定义当前时间
	now := time.Now()

	// 使用 Unscoped() 恢复已软删除的项目成员
	result := getDB(ctx, r.db, tx).
		Unscoped().
		Model(&model.ProjectMember{}).
		Where(model.ProjectMemberColumnProjectID+" = ?", param.ProjectID).
		Where(model.ProjectMemberColumnUserID+" = ?", param.UserID).
		Where(model.ProjectMemberColumnDeletedAt + " IS NOT NULL").
		Updates(map[string]interface{}{
			model.ProjectMemberColumnRole:      param.Role,
			model.ProjectMemberColumnInvitedBy: param.InvitedBy,
			model.ProjectMemberColumnJoinedAt:  param.JoinedAt,
			model.ProjectMemberColumnUpdatedAt: now,
			model.ProjectMemberColumnDeletedAt: nil, // 恢复：清空软删除标记
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrProjectMemberNotFound
	}

	return nil
}

// GetProjectMemberRoleByProjectIDAndUserID 获取指定成员在项目中的角色
// 上层若只关心角色，可避免加载完整对象
func (r *projectMemberRepository) GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error) {
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

	var projectMember model.ProjectMember
	err := getDB(ctx, r.db, nil).
		Preload(model.ProjectMemberAssocUser, SelectUserFields).
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
	var id uint64

	err := getDB(ctx, r.db, nil).
		Model(&model.ProjectMember{}).
		Select(model.ProjectMemberColumnID).
		Where(model.ProjectMemberColumnProjectID+" = ?", projectID).
		Where(model.ProjectMemberColumnUserID+" = ?", userID).
		Limit(1).
		Take(&id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
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
