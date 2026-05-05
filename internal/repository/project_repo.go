// internal/repository/project_repo.go
// Package repository 实现封装 project 数据表的操作

package repository

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"time"

	"gorm.io/gorm"
)

var (
	ErrProjectIsEmpty      = errors.New("project cannot be empty")      // 项目对象为空
	ErrProjectQueryInvalid = errors.New("project query is invalid")     // 项目查询参数非法
	ErrProjectUpdateEmpty  = errors.New("project update data is empty") // 项目更新参数为空
	ErrProjectNotFound     = errors.New("project not found")            // 项目未找到
	ErrInvalidProjectState = errors.New("invalid project status")       // 非法项目状态
)

// projectRepository 项目仓储
type projectRepository struct {
	db *gorm.DB
}

// NewProjectRepository 创建用户仓储
func NewProjectRepository(db *gorm.DB) *projectRepository {
	return &projectRepository{db: db}
}

// CreateWithTx 在数据库中插入一个数据
func (r *projectRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, project *model.Project) error {
	if project == nil {
		return ErrProjectIsEmpty // 返回为空的错误
	}
	return getDB(ctx, r.db, tx).
		Create(project).Error // 执行事务
}

// GetByID 根据项目ID获取项目详情
func (r *projectRepository) GetByID(ctx context.Context, id uint64) (*model.Project, error) {
	var project model.Project
	// 查找
	err := getDB(ctx, r.db, nil).
		Where(model.ProjectColumnID+" = ?", id).
		First(&project).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	return &project, nil
}

// GetDetailByID 根据项目 ID 获取项目详情（预加载 owner）
//
// 说明：要求 model.Project 中存在 Owner 关联字段
func (r *projectRepository) GetDetailByID(ctx context.Context, id uint64) (*model.Project, error) {
	var project model.Project
	err := getDB(ctx, r.db, nil).
		Preload(model.ProjectAssocOwner, SelectUserFields).
		Where(model.ProjectColumnID+" = ?", id).
		First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	return &project, nil
}

// UpdateProjectData 项目更新数据
type UpdateProjectData struct {
	Name        string     // 项目名称
	Description string     // 项目描述
	Status      string     // 项目状态
	StartDate   *time.Time // 项目开始时间
	EndDate     *time.Time // 项目结束时间
}

// UpdateProjectInformationWithTx 根据项目 ID 更新项目基础信息
func (r *projectRepository) UpdateProjectInformationWithTx(ctx context.Context, tx *gorm.DB, id uint64, data *UpdateProjectData) error {
	if data == nil {
		return ErrProjectUpdateEmpty
	}

	updates := map[string]interface{}{ // 基础构造
		model.ProjectColumnUpdatedAt: time.Now(),
	}

	// 名称非空时候更新
	if data.Name != "" {
		updates[model.ProjectColumnName] = data.Name
	}

	// 描述非空的时候更新
	if data.Description != "" {
		updates[model.ProjectColumnDescription] = data.Description
	}

	// 状态非空时才更新，并做合法性校验
	if data.Status != "" {
		updates[model.ProjectColumnStatus] = data.Status
	}

	// 可选时间字段
	if data.StartDate != nil {
		updates[model.ProjectColumnStartDate] = *data.StartDate
	}
	if data.EndDate != nil {
		updates[model.ProjectColumnEndDate] = *data.EndDate
	}

	return getDB(ctx, r.db, tx).
		Model(&model.Project{}).
		Where(model.ProjectColumnID+" = ?", id).
		Updates(updates).Error
}

// ArchiveProjectWithTx 根据项目 ID 归档项目
func (r *projectRepository) ArchiveProjectWithTx(ctx context.Context, tx *gorm.DB, id uint64) error {
	return getDB(ctx, r.db, tx).
		Model(&model.Project{}).
		Where(model.ProjectColumnID+" = ?", id).
		Updates(map[string]interface{}{
			model.ProjectColumnStatus:    model.ProjectStatusArchived,
			model.ProjectColumnUpdatedAt: time.Now(),
		}).Error
}

// ProjectSearchQuery 项目列表查询参数。
type ProjectSearchQuery struct {
	SearchQuery
	Status     string   // 项目状态
	Keyword    string   // 关键词
	ProjectIDs []uint64 // 当前用户可见的项目 ID 列表
}

type SearchProjectResult = SearchResult[*model.Project]

// SearchProjects 获取当前用户可见项目列表。
// 说明：
// 1. 只查询当前用户可见的项目
// 2. 支持按状态筛选
// 3. 支持按项目名称前缀匹配
// 4. total 按需查询，避免每次 COUNT(*) 带来的性能开销
// 5. hasMore 通过 Limit(pageSize + 1) 判断，不依赖 total
// 6. 要求 model.Project 中存在 Owner 关联字段
func (r *projectRepository) SearchProjects(ctx context.Context, query *ProjectSearchQuery) (*SearchProjectResult, error) {
	if query == nil {
		return nil, ErrProjectQueryInvalid
	}

	// 参数的矫正交给上层，这里只做必要的兜底，避免非法分页导致 SQL 异常。
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, ErrProjectQueryInvalid
	}

	// 无可见项目时直接返回空列表，避免执行无意义 SQL。
	if len(query.ProjectIDs) == 0 {
		var totalPtr *int64

		// NeedTotal=true 时，返回明确的 total=0。
		if query.NeedTotal {
			totalPtr = utils.Int64Ptr(0)
		}

		return &SearchProjectResult{
			List:    []*model.Project{},
			Total:   totalPtr,
			HasMore: false,
		}, nil
	}

	// 构建基础查询条件。
	baseDB := getDB(ctx, r.db, nil).
		Model(&model.Project{}).
		Where(model.ProjectColumnID+" IN ?", query.ProjectIDs)

	// 按状态筛选。
	if query.Status != "" {
		baseDB = baseDB.Where(model.ProjectColumnStatus+" = ?", query.Status)
	}

	// 按项目名称前缀匹配。
	if query.Keyword != "" {
		baseDB = baseDB.Where(model.ProjectColumnName+" LIKE ?", query.Keyword+"%")
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
			return &SearchProjectResult{
				List:    []*model.Project{},
				Total:   totalPtr,
				HasMore: false,
			}, nil
		}
	}

	// 计算分页偏移量。
	offset := (query.Page - 1) * query.PageSize

	// 多查一条用于判断是否还有下一页。
	limit := query.PageSize + 1

	projects := make([]*model.Project, 0, limit)

	if err := baseDB.
		Preload(model.ProjectAssocOwner, SelectUserFields).
		Select([]string{
			model.ProjectColumnID,
			model.ProjectColumnName,
			model.ProjectColumnStatus,
			model.ProjectColumnStartDate,
			model.ProjectColumnEndDate,
			model.ProjectColumnOwnerID,
		}).
		Order(model.ProjectColumnCreatedAt + " DESC"). // 按创建时间倒序。
		Order(model.ProjectColumnName + " ASC").       // 创建时间相同时按项目名称正序。
		Order(model.ProjectColumnID + " DESC").        // 排序兜底，保证结果稳定。
		Offset(offset).
		Limit(limit).
		Find(&projects).Error; err != nil {
		return nil, err
	}

	// 通过多查的一条数据判断 hasMore，不依赖 total。
	hasMore := len(projects) > query.PageSize
	if hasMore {
		projects = projects[:query.PageSize]
	}

	return &SearchProjectResult{
		List:    projects,
		Total:   totalPtr,
		HasMore: hasMore,
	}, nil
}

// ExistsByProjectID 根据项目ID判断项目是否存在
func (r *projectRepository) ExistsByProjectID(ctx context.Context, projectID uint64) (bool, error) {
	if projectID == 0 {
		return false, ErrProjectQueryInvalid
	}

	var id int64
	err := getDB(ctx, r.db, nil).
		Model(&model.Project{}).
		Select(model.ProjectColumnID).
		Where(model.ProjectColumnID+" = ?", projectID).
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
