// internal/repository/project_repo.go
// Package repository 实现封装 project 数据表的操作

package repository

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/model"
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
		Preload(model.ProjectAssocOwner).
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

// ProjectSearchQuery 项目列表查询参数
type ProjectSearchQuery struct {
	Page       int      // 页码
	PageSize   int      // 每页数量
	Status     string   // 项目状态
	Keyword    string   // 关键词
	ProjectIDs []uint64 // 当前用户可见的项目 ID 列表
}

// SearchProjects 获取当前用户可见项目列表
// 说明：要求 model.Project 中存在 Owner 关联字段
func (r *projectRepository) SearchProjects(ctx context.Context, query *ProjectSearchQuery) ([]*model.Project, int64, error) {
	if query == nil {
		return nil, 0, ErrProjectQueryInvalid
	}
	// 参数的矫正交给上层，这里完全相信上层

	// 无可见项目时直接返回空列表，避免执行无意义 SQL
	if len(query.ProjectIDs) == 0 {
		return []*model.Project{}, 0, nil
	}

	db := getDB(ctx, r.db, nil).
		Model(&model.Project{}).
		Where(fmt.Sprintf("%s IN ?", model.ProjectColumnID), query.ProjectIDs)

	// 按状态筛选
	if query.Status != "" {
		db = db.Where(model.ProjectColumnStatus+" = ?", query.Status) // 如果状态不为空，就新增筛选
	}

	// 按关键词模糊匹配
	if query.Keyword != "" {
		likePattern := query.Keyword + "%"
		db = db.Where(model.ProjectColumnName+" LIKE ?", likePattern)
	}
	// 没有关键词就找当前可用查找的全量

	// 统计总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*model.Project{}, 0, nil
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	projects := make([]*model.Project, 0, query.PageSize)

	err := db.
		Preload(model.ProjectAssocOwner).
		Order(model.ProjectColumnCreatedAt + " DESC"). // 按照创建时间排序
		Order(model.ProjectColumnName + " ASC").       // 创建时间相同按照项目名称排序
		Offset(offset).
		Limit(query.PageSize).
		Find(&projects).Error
	if err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}
