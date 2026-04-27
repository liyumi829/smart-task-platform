// internal/service/project_service.go
// Package service 业务层，实现项目模块的服务
package service

import (
	"context"
	"errors"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/validator"
	"smart-task-platform/internal/repository"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ProjectService 项目服务
type ProjectService struct {
	txMgr *repository.TxManager       // 事务管理器
	ur    projectSvcUserRepo          // 用户仓储接口
	pr    projectSvcProjectRepo       // 项目仓储接口
	pmr   projectSvcProjectMemberRepo // 项目成员仓储接口
}

// NewProjectService 创建项目服务实例
func NewProjectService(
	txMgr *repository.TxManager,
	userRepo projectSvcUserRepo,
	projectRepo projectSvcProjectRepo,
	projectMemberRepo projectSvcProjectMemberRepo,
) *ProjectService {
	return &ProjectService{
		txMgr: txMgr,
		ur:    userRepo,
		pr:    projectRepo,
		pmr:   projectMemberRepo,
	}
}

// CreateProjectParams 创建项目需要的参数
type CreateProjectParams struct {
	UserID      uint64 // 创建项目的用户ID
	Name        string // 项目名称
	Description string // 项目描述
	StartTime   string // 起始时间
	EndTime     string // 结束时间
}

// CreateProject 创建项目
func (s *ProjectService) CreateProject(ctx context.Context, param *CreateProjectParams) (*dto.CreateProjectResp, error) {
	// 参数校验
	if param == nil || param.UserID <= 0 {
		zap.L().Warn("create project failed: invalid param")
		return nil, ErrInvalidProjectParam
	}
	param.Name = strings.TrimSpace(param.Name)
	param.Description = strings.TrimSpace(param.Description)
	param.StartTime = strings.TrimSpace(param.StartTime)
	param.EndTime = strings.TrimSpace(param.EndTime)

	// 使用 With 复用日志字段，避免后续日志重复写 user_id、project_name
	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.String("project_name", param.Name),
	)

	// 创建项目的时候项目名称不能为空且必须合法
	if param.Name == "" {
		logger.Warn("create project failed: project name is empty")
		return nil, ErrEmptyProjectName
	}

	if !validator.IsValidProjectName(param.Name) {
		logger.Warn("create project failed: project name is invalid")
		return nil, ErrInvalidProjectName
	}

	// 创建项目的项目描述可以为空，但是不能超过200词/字
	if param.Description != "" && !isValidDescription(param.Description) {
		logger.Warn("create project failed: project description is too long")
		return nil, ErrInvalidProjectDescription
	}

	// 参数转化
	startDate, err := parseOptionalISOTime(param.StartTime)
	if err != nil {
		logger.Warn("create project failed: invalid start time",
			zap.String("start_time", param.StartTime),
			zap.Error(err),
		)
		return nil, ErrInvalidTime
	}

	endDate, err := parseOptionalISOTime(param.EndTime)
	if err != nil {
		logger.Warn("create project failed: invalid end time",
			zap.String("end_time", param.EndTime),
			zap.Error(err),
		)
		return nil, ErrInvalidTime
	}

	// 结束时间不能早于开始时间
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		logger.Warn("create project failed: end time before start time",
			zap.Time("start_time", *startDate),
			zap.Time("end_time", *endDate),
		)
		return nil, ErrInvalidTimeRange
	}

	// 先检查用户信息
	user, err := s.ur.GetByID(ctx, param.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			logger.Warn("create project failed: user not found")
			return nil, ErrUserNotFound
		}
		logger.Error("create project failed: get user error", zap.Error(err))
		return nil, err
	}
	// 准备插入数据
	project := &model.Project{
		Name:        param.Name,
		Description: param.Description,
		Status:      model.ProjectStatusActive,
		StartDate:   startDate,
		EndDate:     endDate,
		OwnerID:     param.UserID,
	}

	// 事务执行插入功能
	err = s.txMgr.Transaction(ctx,
		func(tx *gorm.DB) error {
			// 创建项目
			if err := s.pr.CreateWithTx(ctx, tx, project); err != nil {
				logger.Error("create project failed: create project db error", zap.Error(err))
				return err
			}

			// 添加项目成员 owner
			if err := s.pmr.CreateWithTx(ctx, tx, &model.ProjectMember{
				ProjectID: project.ID,
				UserID:    project.OwnerID,
				Role:      model.ProjectMemberRoleOwner,
				JoinedAt:  time.Now(),
			}); err != nil {
				logger.Error("create project failed: create owner member db error",
					zap.Uint64("project_id", project.ID),
					zap.Error(err),
				)
				return err
			}

			return nil
		})
	if err != nil {
		logger.Error("create project failed: transaction rollback", zap.Error(err))
		return nil, err
	}

	logger.Info("create project success", zap.Uint64("project_id", project.ID))

	// 返回请求
	return &dto.CreateProjectResp{
		ID:          project.ID,
		Name:        project.Name,
		Description: project.Description,
		Status:      project.Status,
		StartDate:   project.StartDate,
		EndDate:     project.EndDate,
		CreatedAt:   project.CreatedAt,
		OwnerID:     project.OwnerID,
		Owner:       buildUserPublicProfile(user),
	}, nil
}

// ListProjectsParam 获取项目列表参数
type ListProjectsParam struct {
	UserID   uint64 // 当前登录用户 ID
	Page     int    // 页码
	PageSize int    // 每页数量
	Status   string // 项目状态
	Keyword  string // 搜索关键词
}

// ListProjects 获取项目列表
func (s *ProjectService) ListProjects(ctx context.Context, param *ListProjectsParam) (*dto.ProjectListResp, error) {
	// 参数校验
	if param == nil || param.UserID <= 0 {
		zap.L().Warn("list projects failed: invalid param")
		return nil, ErrInvalidProjectParam
	}
	keyword := strings.TrimSpace(param.Keyword) // 清理关键字空格
	status := strings.TrimSpace(param.Status)
	page, pageSize := fixPageParams(param.Page, param.PageSize) // 分页参数兜底。

	// 使用 With 复用日志字段，避免重复写入 user_id、page、page_size
	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
		zap.String("status", status),
		zap.String("keyword", keyword),
	)

	// 校验项目状态，空字符串表示不过滤状态
	if status != "" && status != model.ProjectStatusActive && status != model.ProjectStatusArchived {
		logger.Warn("list projects failed: invalid project status")
		return nil, ErrInvalidProjectStatus
	}

	// 查询当前用户可见的项目 ID，保证只能看到自己参与的项目
	projectIDs, err := s.pmr.ListProjectIDsByUserID(ctx, param.UserID)
	if err != nil {
		logger.Error("list projects failed: list visible project ids error", zap.Error(err))
		return nil, err
	}

	// 当前用户没有参与任何项目，直接返回空列表
	if len(projectIDs) == 0 {
		logger.Info("list projects success: no visible projects")

		return &dto.ProjectListResp{
			List:     []*dto.ProjectListItem{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	// 参数校验完成，进行搜索
	projects, total, err := s.pr.SearchProjects(ctx, &repository.ProjectSearchQuery{
		Page:       page,
		PageSize:   pageSize,
		Status:     status,
		Keyword:    keyword,
		ProjectIDs: projectIDs,
	})
	if err != nil {
		logger.Error("list projects failed: search projects error", zap.Error(err))
		return nil, err
	}

	// 搜索成功
	list := make([]*dto.ProjectListItem, 0, len(projects))
	for _, project := range projects {
		if project == nil {
			logger.Warn("list project skipped: nil project")
			continue
		}

		list = append(list, &dto.ProjectListItem{
			ID:        project.ID,
			Name:      project.Name,
			Status:    project.Status,
			StartDate: project.StartDate,
			EndDate:   project.EndDate,
			OwnerID:   project.OwnerID,
			Owner:     buildUserPublicProfile(&project.Owner),
		})
	}

	logger.Info("list projects success",
		zap.Int64("total", total),
		zap.Int("result_count", len(list)),
	)

	// 构造成功
	return &dto.ProjectListResp{
		List:     list,
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetProjectDetailParam 获取项目详情参数
type GetProjectDetailParam struct {
	UserID    uint64 // 当前登录用户 ID
	ProjectID uint64 // 项目 ID
}

// GetProjectDetail 获取项目详细情况
func (s *ProjectService) GetProjectDetail(ctx context.Context, param *GetProjectDetailParam) (*dto.ProjectDetailResp, error) {
	// 参数校验
	if param == nil || param.UserID <= 0 || param.ProjectID <= 0 {
		zap.L().Warn("get project detail failed: invalid param")
		return nil, ErrInvalidProjectParam
	}

	// 使用 With 复用日志字段，避免后续日志重复写 user_id、project_id
	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
	)

	// 先查询项目是否存在
	project, err := s.pr.GetDetailByID(ctx, param.ProjectID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectNotFound) {
			logger.Warn("get project detail failed: project not found")
			return nil, ErrProjectNotFound
		}

		logger.Error("get project detail failed: get project detail db error", zap.Error(err))
		return nil, err
	}

	// 再校验当前用户是否属于该项目
	joined, err := s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.UserID)
	if err != nil {
		logger.Error("get project detail failed: check project member error", zap.Error(err))
		return nil, err
	}
	if !joined {
		logger.Warn("get project detail failed: user has no permission")
		return nil, ErrProjectMemberNotFound
	}

	// 找到了
	resp := &dto.ProjectDetailResp{
		ID:          project.ID,
		Name:        project.Name,
		Description: project.Description,
		Status:      project.Status,
		StartDate:   project.StartDate,
		EndDate:     project.EndDate,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
		OwnerID:     project.OwnerID,
		Owner:       buildUserPublicProfile(&project.Owner),
	}

	logger.Info("get project detail success")

	return resp, nil
}

// UpdateProjectParam 更新项目数据参数
type UpdateProjectParam struct {
	ProjectID   uint64 // 项目ID
	UserID      uint64 // 当前操作用户ID
	Name        string // 项目名称
	Description string // 项目描述
	Status      string // 更新的状态
	StartTime   string // 起始时间
	EndTime     string // 结束时间
}

// UpdateProject 更新项目数据
func (s *ProjectService) UpdateProject(ctx context.Context, param *UpdateProjectParam) (*dto.UpdateProjectResp, error) {
	// 参数校验
	if param == nil || param.UserID <= 0 || param.ProjectID <= 0 {
		zap.L().Warn("update project failed: invalid param")
		return nil, ErrInvalidProjectParam
	}
	param.Name = strings.TrimSpace(param.Name)
	param.Description = strings.TrimSpace(param.Description)
	param.StartTime = strings.TrimSpace(param.StartTime)
	param.EndTime = strings.TrimSpace(param.EndTime)
	status := strings.TrimSpace(param.Status)

	// 使用 With 复用日志字段，避免重复写 user_id、project_id
	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
		zap.String("project_name", param.Name),
		zap.String("status", status),
	)

	// 校验项目状态
	if status != "" && !isValidProjectStatus(status) {
		logger.Warn("update project failed: invalid project status")
		return nil, ErrInvalidProjectStatus
	}

	// 项目名称合法 / 如果为空就不设置
	if param.Name != "" && !validator.IsValidProjectName(param.Name) {
		logger.Warn("update project failed: project name is invalid")
		return nil, ErrInvalidProjectName
	}

	// 创建项目的项目描述可以为空，但是不能超过200词/字
	if param.Description != "" && !isValidDescription(param.Description) {
		logger.Warn("update project failed: project description is too long")
		return nil, ErrInvalidProjectDescription
	}

	// 参数转化
	startDate, err := parseOptionalISOTime(param.StartTime)
	if err != nil {
		logger.Warn("update project failed: invalid start time",
			zap.String("start_time", param.StartTime),
			zap.Error(err),
		)
		return nil, ErrInvalidTime
	}

	endDate, err := parseOptionalISOTime(param.EndTime)
	if err != nil {
		logger.Warn("update project failed: invalid end time",
			zap.String("end_time", param.EndTime),
			zap.Error(err),
		)
		return nil, ErrInvalidTime
	}

	// 结束时间不能早于开始时间
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		logger.Warn("update project failed: invalid time range",
			zap.Time("start_time", *startDate),
			zap.Time("end_time", *endDate),
		)
		return nil, ErrInvalidTimeRange
	}

	// 进行 user 的身份验证，只有 owner 和 admin 可以更新项目数据
	role, level, err := getProjectMemberRoleLevel(ctx, s.pmr, param.ProjectID, param.UserID, logger)
	if err != nil {
		logger.Warn("project call getProjectMemberRoleLevel error")
		return nil, err
	}
	if level > model.RoleLevel[model.ProjectMemberRoleAdmin] {
		logger.Warn("project permission check failed: permission denied",
			zap.String("member_role", role))
		return nil, ErrProjectForbidden // 权限不足
	}

	// 权限通过
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		// 更新项目基础信息，UpdatedAt 建议由 repository 层统一维护
		return s.pr.UpdateProjectInformationWithTx(ctx, tx, param.ProjectID, &repository.UpdateProjectData{
			Name:        param.Name,
			Description: param.Description,
			Status:      status,
			StartDate:   startDate,
			EndDate:     endDate,
		})
	})
	if err != nil {
		if errors.Is(err, repository.ErrProjectNotFound) {
			logger.Warn("update project failed: project not found")
			return nil, ErrProjectNotFound
		}

		logger.Error("update project failed: update project transaction error", zap.Error(err))
		return nil, err
	}

	// 更新成功后重新查询项目详情，OwnerID 和 Owner 必须以后端数据库为准
	project, err := s.pr.GetDetailByID(ctx, param.ProjectID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectNotFound) {
			logger.Warn("update project failed: project not found after update")
			return nil, ErrProjectNotFound
		}

		logger.Error("update project failed: get updated project detail error", zap.Error(err))
		return nil, err
	}

	resp := &dto.UpdateProjectResp{
		ID:          project.ID,
		Name:        project.Name,
		Description: project.Description,
		Status:      project.Status,
		StartDate:   project.StartDate,
		EndDate:     project.EndDate,
		UpdatedAt:   project.UpdatedAt,
		OwnerID:     project.OwnerID,
		Owner:       buildUserPublicProfile(&project.Owner),
	}

	logger.Info("update project success",
		zap.Uint64("owner_id", project.OwnerID),
	)

	return resp, nil
}

// ArchiveProject 归档项目
func (s *ProjectService) ArchiveProject(ctx context.Context, userID, projectID uint64) (*dto.ArchiveProjectResp, error) {
	// 参数校验
	if userID <= 0 || projectID <= 0 {
		zap.L().Warn("archive project failed: invalid param")
		return nil, ErrInvalidProjectParam
	}

	// 使用 With 复用日志字段，避免重复写 user_id、project_id
	logger := zap.L().With(
		zap.Uint64("user_id", userID),
		zap.Uint64("project_id", projectID),
	)

	// 进行 user 的身份验证，只有 owner 和 admin 可以归档项目
	role, level, err := getProjectMemberRoleLevel(ctx, s.pmr, projectID, userID, logger)
	if err != nil {
		logger.Warn("project call getProjectMemberRoleLevel error")
		return nil, err
	}
	if level > model.RoleLevel[model.ProjectMemberRoleAdmin] {
		logger.Warn("project permission check failed: permission denied",
			zap.String("member_role", role))
		return nil, ErrProjectForbidden // 权限不足
	}

	// 权限通过，执行归档操作
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		// 归档项目，状态由后端固定设置为 archived，不能由前端传入
		return s.pr.ArchiveProjectWithTx(ctx, tx, projectID)
	})
	if err != nil {
		if errors.Is(err, repository.ErrProjectNotFound) {
			logger.Warn("archive project failed: project not found")
			return nil, ErrProjectNotFound
		}

		logger.Error("archive project failed: archive project transaction error", zap.Error(err))
		return nil, err
	}

	logger.Info("archive project success", zap.String("status", model.ProjectStatusArchived))

	return &dto.ArchiveProjectResp{
		ID:     projectID,
		Status: model.ProjectStatusArchived,
	}, nil
}

type projectSvcUserRepo interface {
	GetByID(ctx context.Context, id uint64) (*model.User, error)
}

type projectSvcProjectRepo interface {
	CreateWithTx(ctx context.Context, tx *gorm.DB, project *model.Project) error
	SearchProjects(ctx context.Context, query *repository.ProjectSearchQuery) ([]*model.Project, int64, error)
	GetDetailByID(ctx context.Context, id uint64) (*model.Project, error)
	UpdateProjectInformationWithTx(ctx context.Context, tx *gorm.DB, id uint64, data *repository.UpdateProjectData) error
	ArchiveProjectWithTx(ctx context.Context, tx *gorm.DB, id uint64) error
}

type projectSvcProjectMemberRepo interface {
	CreateWithTx(ctx context.Context, tx *gorm.DB, projectMember *model.ProjectMember) error
	ListProjectIDsByUserID(ctx context.Context, userID uint64) ([]uint64, error)
	ExistsByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (bool, error)
	GetProjectMemberByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (*model.ProjectMember, error)
}
