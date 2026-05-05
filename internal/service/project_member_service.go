// internal/service/project_member_service.go
// Package service 实现项目成员模块的操作
package service

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	AdminCount = 5 // 管理者人数不超过 5 个
)

// ProjectMemberService 项目成员服务
type ProjectMemberService struct {
	txMgr *repository.TxManager             // 事务管理器
	ur    projectMemberSvcUserRepo          // 用户仓储接口
	pr    projectMemberSvcProjectRepo       // 项目仓储接口
	pmr   projectMemberSvcProjectMemberRepo // 项目成员仓储接口
	tr    projectMemberSvcTaskRepo          // 任务仓储接口
}

// NewProjectMemberService 创建项目成员服务实例
func NewProjectMemberService(
	txMgr *repository.TxManager,
	userRepo projectMemberSvcUserRepo,
	projectRepo projectMemberSvcProjectRepo,
	projectMemberRepo projectMemberSvcProjectMemberRepo,
	taskRepo projectMemberSvcTaskRepo,
) *ProjectMemberService {
	return &ProjectMemberService{
		txMgr: txMgr,
		ur:    userRepo,
		pr:    projectRepo,
		pmr:   projectMemberRepo,
		tr:    taskRepo,
	}
}

// AddProjectMemberParam 添加项目成员需要的参数
type AddProjectMemberParam struct {
	ProjectID     uint64
	InvitedUserID uint64
	Role          string
	InvitorID     uint64
}

// AddProjectMember 添加项目成员
func (s *ProjectMemberService) AddProjectMember(ctx context.Context, param *AddProjectMemberParam) (*dto.AddProjectMemberResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("add project member failed: invalid param")
		return nil, ErrInvalidProjectMemberParam
	}

	if param.InvitedUserID <= 0 || param.ProjectID <= 0 || param.InvitorID <= 0 {
		// 非法的用户ID、项目ID、邀请人ID
		zap.L().Warn("add project member failed: invalid ids",
			zap.Uint64("project_id", param.ProjectID),
			zap.Uint64("invited_user_id", param.InvitedUserID),
			zap.Uint64("invitor_id", param.InvitorID),
		)
		return nil, ErrInvalidProjectMemberParam
	}

	role := strings.TrimSpace(param.Role)

	// 使用 With 复用日志字段
	logger := zap.L().With(
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("invited_user_id", param.InvitedUserID),
		zap.Uint64("invitor_id", param.InvitorID),
		zap.String("role", role),
	)

	// role 不能为空
	if role == "" {
		logger.Warn("add project member failed: role is empty")
		return nil, ErrEmptyProjectMemberRole
	}

	// role 必须合法
	if !isValidMemberRole(role) {
		logger.Warn("add project member failed: role is invalid")
		return nil, ErrInvalidProjectMemberRole
	}

	// 添加成员接口不允许直接添加 owner
	// owner 应该通过创建项目或 owner 转让产生
	if role == model.ProjectMemberRoleOwner {
		logger.Warn("add project member failed: cannot add owner directly")
		return nil, ErrProjectForbidden // 返回无权限
	}

	// 校验被邀请用户是否存在
	user, err := s.ur.GetByID(ctx, param.InvitedUserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			logger.Warn("add project member failed: invited user not found")
			return nil, ErrUserNotFound
		}

		logger.Error("add project member failed: get invited user error", zap.Error(err))
		return nil, err
	}

	// 校验项目是否存在
	ok, err := s.pr.ExistsByProjectID(ctx, param.ProjectID)
	if err != nil {
		logger.Error("add project member failed: check project exists error", zap.Error(err))
		return nil, err
	}
	if !ok {
		logger.Warn("add project member failed: project not found")
		return nil, ErrProjectNotFound
	}

	// 获取邀请人的项目角色和权限等级
	invitorRole, invitorLevel, err := getProjectMemberRoleLevel(ctx, s.pmr, param.ProjectID, param.InvitorID, logger)
	if err != nil {
		return nil, err
	}

	// 获取目标角色的权限等级
	targetLevel, ok := model.RoleLevel[role]
	if !ok {
		// 正常情况下前面的 isValidMemberRole 已经拦截，这里做防御性处理
		logger.Warn("add project member failed: target role level not found")
		return nil, ErrInvalidProjectMemberRole
	}

	// 权限规则：
	// 1. owner 可以添加 admin、member
	// 2. admin 只能添加 member
	// 3. member 不能添加任何成员
	//
	// RoleLevel 数字越小，权限越高。
	// 所以邀请人的权限等级必须严格小于目标角色权限等级。
	if invitorLevel >= targetLevel {
		logger.Warn("add project member failed: permission denied",
			zap.String("invitor_role", invitorRole),
			zap.Int("invitor_level", invitorLevel),
			zap.Int("target_level", targetLevel),
		)
		return nil, ErrProjectForbidden
	}

	// 判断被邀请用户是否已经是有效项目成员
	ok, err = s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.InvitedUserID)
	if err != nil {
		logger.Error("add project member failed: check project member exists error", zap.Error(err))
		return nil, err
	}
	if ok {
		logger.Warn("add project member failed: user already exists in project")
		return nil, ErrProjectMemberAlreadyExists
	}

	// 查询是否存在软删除的项目成员记录
	deletedProjectMember, err := s.pmr.GetProjectMemberByProjectIDAndUserIDUnscoped(
		ctx,
		param.ProjectID,
		param.InvitedUserID,
	)
	if err != nil {
		if !errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Error("add project member failed: get project member unscoped error", zap.Error(err))
			return nil, err
		}

		// 没有历史记录，后续直接创建
		deletedProjectMember = nil
	}

	// 如果存在未删除记录，说明成员已经存在，做防御性判断
	if deletedProjectMember != nil && !deletedProjectMember.DeletedAt.Valid {
		logger.Warn("add project member failed: user already exists in project by unscoped query")
		return nil, ErrProjectMemberAlreadyExists
	}

	// 如果添加 admin，需要校验 admin 人数上限
	if role == model.ProjectMemberRoleAdmin {
		count, err := s.pmr.CountByProjectIDAndRole(ctx, param.ProjectID, model.ProjectMemberRoleAdmin)
		if err != nil {
			logger.Error("add project member failed: count admin members error", zap.Error(err))
			return nil, err
		}

		if count >= int64(AdminCount) {
			logger.Warn("add project member failed: admin member limit exceeded",
				zap.Int64("admin_count", count),
				zap.Int("admin_limit", AdminCount),
			)
			return nil, ErrExceedsAdminMemberLimit
		}
	}

	now := time.Now()

	// 执行事务：
	// 1. 如果存在软删除记录，则恢复 deleted_at = NULL，并更新角色、邀请人、加入时间、更新时间
	// 2. 如果不存在历史记录，则创建新的项目成员记录
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		if deletedProjectMember != nil && deletedProjectMember.DeletedAt.Valid {
			if err := s.pmr.RestoreProjectMemberWithTx(ctx, tx,
				&repository.RestoreProjectMemberWithTxParam{
					ProjectID: param.ProjectID,
					UserID:    param.InvitedUserID,
					Role:      role,
					InvitedBy: &param.InvitorID,
					UpdatedAt: now,
					JoinedAt:  now,
				},
			); err != nil {
				logger.Error("add project member failed: restore deleted project member db error", zap.Error(err))
				return err
			}

			return nil
		}

		if err := s.pmr.CreateWithTx(ctx, tx, &model.ProjectMember{
			ProjectID: param.ProjectID,
			UserID:    param.InvitedUserID,
			InvitedBy: &param.InvitorID, // 添加成员一定是被邀请的
			Role:      role,
			JoinedAt:  now,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			logger.Error("add project member failed: create project member db error", zap.Error(err))
			return err
		}

		return nil
	})
	if err != nil {
		logger.Error("add project member failed: transaction rollback", zap.Error(err))
		return nil, err
	}

	logger.Info("add project member success",
		zap.String("invitor_role", invitorRole),
	)

	return &dto.AddProjectMemberResp{
		ProjectID: param.ProjectID,
		UserID:    param.InvitedUserID,
		Role:      role,
		User:      buildUserPublicProfile(user),
		JoinedAt:  now,
	}, nil
}

// ListProjectMembersParam 获取项目成员列表参数
type ListProjectMembersParam struct {
	UserID    uint64
	ProjectID uint64
	Page      int
	PageSize  int
	NeedTotal bool
	Role      string
	Keyword   string
}

// ListProjectMembers 获取项目成员列表
func (s *ProjectMemberService) ListProjectMembers(ctx context.Context, param *ListProjectMembersParam) (*dto.ProjectMemberListResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("list project members failed: invalid param")
		return nil, ErrInvalidProjectMemberParam
	}
	if param.ProjectID <= 0 || param.UserID <= 0 {
		zap.L().Warn("list project members failed: invalid ids",
			zap.Uint64("project_id", param.ProjectID),
			zap.Uint64("user_id", param.UserID),
		)
		return nil, ErrInvalidProjectMemberParam
	}
	// 检查字段
	keyword := strings.TrimSpace(param.Keyword) // 清理关键字空格
	role := strings.TrimSpace(param.Role)
	page, pageSize := fixPageParams(param.Page, param.PageSize) // 分页参数兜底。
	// 使用 With 复用日志字段，避免后续日志重复写 project_id、user_id、role、keyword、page、page_size
	logger := zap.L().With(
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("user_id", param.UserID),
		zap.String("role", role),
		zap.String("keyword", keyword),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
	)
	// role、key 为空表示没有对应的条件查询
	if role != "" && !isValidMemberRole(role) {
		logger.Warn("list project members failed: role is invalid")
		return nil, ErrInvalidProjectMemberRole
	}

	// 参数校验完成，逻辑判断
	// 项目要存在、获取项目成员列表的人要在项目中
	ok, err := s.pr.ExistsByProjectID(ctx, param.ProjectID)
	if err != nil {
		logger.Error("list project members failed: check project exists error", zap.Error(err))
		return nil, err
	}
	if !ok {
		logger.Warn("list project members failed: project not found")
		return nil, ErrProjectNotFound
	}

	// 项目存在，下一步判断成员是否在项目中
	ok, err = s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.UserID)
	if err != nil {
		logger.Error("list project members failed: check current user member exists error", zap.Error(err))
		return nil, err
	}
	if !ok {
		logger.Warn("list project members failed: current user is not project member")
		return nil, ErrProjectMemberNotFound // 权限不足（前后端交互当作无权限）
	}

	// 参数校验完成，进行搜索
	// 需要内置搜索完成 preload User
	result, err := s.pmr.SearchProjectMembers(ctx, &repository.ProjectMemberSearchQuery{
		SearchQuery: repository.SearchQuery{
			Page:      page,
			PageSize:  pageSize,
			NeedTotal: param.NeedTotal,
		},
		ProjectID: param.ProjectID,
		Role:      role,
		Keyword:   keyword,
	})
	if err != nil {
		logger.Error("list project members failed: search project members error", zap.Error(err))
		return nil, err
	}
	// 搜索成功 构造list
	list := make([]*dto.ProjectMemberListItem, 0, len(result.List))
	for _, projectMember := range result.List {
		if projectMember == nil {
			// 正常情况下 repository 不应该返回 nil，这里做防御性处理
			logger.Warn("list project members skipped: nil project member")
			continue
		}
		item := &dto.ProjectMemberListItem{
			ProjectID: projectMember.ProjectID,
			UserID:    projectMember.UserID,
			Role:      projectMember.Role,
			User:      buildUserPublicProfile(projectMember.User),
			JoinedAt:  projectMember.JoinedAt,
		}
		if projectMember.InvitedBy != nil {
			item.InvitedBy = projectMember.InvitedBy
		}
		list = append(list, item)
	}
	// 构造成功返回响应
	logger.Info("list project members success",
		zap.Bool("has_more", result.HasMore),
		zap.Int("list_size", len(list)),
	)

	return &dto.ProjectMemberListResp{
		List:     list,
		Page:     page,
		PageSize: pageSize,
		Total:    utils.SafePtrClone(result.Total),
		HasMore:  result.HasMore,
	}, nil
}

// UpdateProjectMemberParam 修改项目成员属性参数
//
// 指针空值表示不修改
type UpdateProjectMemberParam struct {
	ProjectID      uint64 // 项目ID
	ModifierID     uint64 // 发起操作的人
	ModifiedUserID uint64 // 被修改的人
	Role           *string
}

// UpdateProjectMember 修改项目成员属性
func (s *ProjectMemberService) UpdateProjectMember(ctx context.Context, param *UpdateProjectMemberParam) (*dto.UpdateProjectMemberResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("update project member failed: invalid param")
		return nil, ErrInvalidProjectMemberParam
	}
	if param.ProjectID <= 0 || param.ModifierID <= 0 || param.ModifiedUserID <= 0 {
		zap.L().Warn("update project member failed: invalid ids",
			zap.Uint64("project_id", param.ProjectID),
			zap.Uint64("modifier_id", param.ModifierID),
			zap.Uint64("modified_user_id", param.ModifiedUserID),
		)
		return nil, ErrInvalidProjectMemberParam
	}
	role := ""
	if param.Role != nil {
		fmt.Println(11111)
		role = strings.TrimSpace(*param.Role) // 清除前后空格
		// 检验
		if role != "" && !isValidMemberRole(role) {
			//  role 身份不合法
			zap.L().Warn("update project member failed: role is invalid",
				zap.Uint64("project_id", param.ProjectID),
				zap.Uint64("modifier_id", param.ModifierID),
				zap.Uint64("modified_user_id", param.ModifiedUserID),
				zap.String("role", role),
			)
			return nil, ErrInvalidProjectMemberRole
		}
	}
	// 使用 With 复用日志字段，避免后续日志重复写 project_id、modifier_id、modified_user_id、role
	logger := zap.L().With(
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("modifier_id", param.ModifierID),
		zap.Uint64("modified_user_id", param.ModifiedUserID),
		zap.String("role", role),
	)

	// 参数校验完成，逻辑判断
	// 项目要存在、修改者和被修改者都要在项目中
	ok, err := s.pr.ExistsByProjectID(ctx, param.ProjectID)
	if err != nil {
		logger.Error("update project member failed: check project exists error", zap.Error(err))
		return nil, err
	}
	if !ok {
		logger.Warn("update project member failed: project not found")
		return nil, ErrProjectNotFound
	}

	// 项目存在，下一步判断成员是否在项目中
	// 修改者
	modifierMember, err := s.pmr.GetProjectMemberByProjectIDAndUserID(ctx, param.ProjectID, param.ModifierID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Warn("update project member failed: modifier is not project member")
			return nil, ErrProjectMemberNotFound // 当作权限不足
		}

		logger.Error("update project member failed: get modifier member error", zap.Error(err))
		return nil, err
	}
	// 被修改者
	projectMember, err := s.pmr.GetProjectMemberByProjectIDAndUserID(ctx, param.ProjectID, param.ModifiedUserID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Warn("update project member failed: modified user is not project member")
			return nil, ErrProjectMemberNotFound // 当作权限不足
		}

		logger.Error("update project member failed: get modified member error", zap.Error(err))
		return nil, err
	}

	// 两个成员都在项目中，对修改者进行鉴权，只有 owner 才可以设置 role
	if modifierMember.Role != model.ProjectMemberRoleOwner {
		logger.Warn("update project member failed: modifier has no permission",
			zap.String("modifier_role", modifierMember.Role),
		)
		return nil, ErrProjectForbidden // 没有权限
	}

	// 修改的角色判断 -- 进行等幂返回
	// 为空，等幂返回
	if role == "" {
		logger.Info("update project member success: no field changed",
			zap.String("current_role", projectMember.Role),
		)

		return &dto.UpdateProjectMemberResp{
			ProjectID: projectMember.ProjectID,
			UserID:    projectMember.UserID,
			Role:      projectMember.Role,
			User:      buildUserPublicProfile(projectMember.User),
		}, nil
	}

	// 如果目标角色和当前角色一致，等幂返回，避免执行无意义 SQL
	if role == projectMember.Role {
		logger.Info("update project member success: role not changed",
			zap.String("current_role", projectMember.Role),
		)

		return &dto.UpdateProjectMemberResp{
			ProjectID: projectMember.ProjectID,
			UserID:    projectMember.UserID,
			Role:      projectMember.Role,
			User:      buildUserPublicProfile(projectMember.User),
		}, nil
	}

	// owner 不能直接把自己修改为 admin/member，避免项目没有 owner
	if param.ModifierID == param.ModifiedUserID && role != model.ProjectMemberRoleOwner {
		logger.Warn("update project member failed: owner cannot demote self",
			zap.String("current_role", projectMember.Role),
			zap.String("target_role", role),
		)
		return nil, ErrProjectForbidden
	}

	param.Role = &role // 防止没有更新
	var updateErr error

	switch role {
	case model.ProjectMemberRoleOwner:
		// 转让 owner：被修改者变为 owner，原 owner 变为 member
		updateErr = s.transferOwner(ctx, param, logger)

	case model.ProjectMemberRoleAdmin:
		// 修改为 admin：需要校验 admin 人数上限
		updateErr = s.updateToAdmin(ctx, param, logger)

	case model.ProjectMemberRoleMember:
		// 修改为普通成员：不允许直接把 owner 降级为 member
		updateErr = s.updateToMember(ctx, param, projectMember, logger)

	default:
		// 正常情况下不会走到这里
		logger.Warn("update project member failed: invalid role in switch")
		return nil, ErrInvalidProjectMemberParam
	}

	if updateErr != nil {
		logger.Error("update project member failed", zap.Error(updateErr))
		return nil, updateErr
	}

	logger.Info("update project member success",
		zap.String("old_role", projectMember.Role),
		zap.String("new_role", role),
	)

	return &dto.UpdateProjectMemberResp{
		ProjectID: projectMember.ProjectID,
		UserID:    projectMember.UserID,
		Role:      role,
		User:      buildUserPublicProfile(projectMember.User),
	}, nil
}

// RemoveProjectMemberParam 移除项目成员参数
type RemoveProjectMemberParam struct {
	OperatorID    uint64
	ProjectID     uint64
	RemovedUserID uint64
}

// RemoveProjectMember 移除项目成员
func (s *ProjectMemberService) RemoveProjectMember(ctx context.Context, param *RemoveProjectMemberParam) (*dto.RemoveProjectMemberResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("remove project member failed: invalid param")
		return nil, ErrInvalidProjectMemberParam
	}
	if param.OperatorID <= 0 || param.ProjectID <= 0 || param.RemovedUserID <= 0 {
		zap.L().Warn("remove project member failed: invalid ids",
			zap.Uint64("project_id", param.ProjectID),
			zap.Uint64("operator_id", param.OperatorID),
			zap.Uint64("removed_user_id", param.RemovedUserID),
		)
		return nil, ErrInvalidProjectMemberParam
	}

	// 使用 With 复用日志字段，避免后续日志重复写 project_id、operator_id、removed_user_id
	logger := zap.L().With(
		zap.Uint64("project_id", param.ProjectID),
		zap.Uint64("operator_id", param.OperatorID),
		zap.Uint64("removed_user_id", param.RemovedUserID),
	)

	// 参数校验完成，逻辑判断
	// 项目要存在、用户要存在
	// 操作者权限： owner 可删 admin、member 不可以删 owner（自己）；admin 可删 member 不能删owner、admin
	ok, err := s.pr.ExistsByProjectID(ctx, param.ProjectID)
	if err != nil {
		logger.Error("remove project member failed: check project exists error", zap.Error(err))
		return nil, err
	}
	if !ok {
		logger.Warn("remove project member failed: project not found")
		return nil, ErrProjectNotFound
	}

	// 项目存在，下一步判断用户是否存在
	// 这里校验的是操作者是否为项目成员
	operatorRole, err := s.pmr.GetProjectMemberRoleByProjectIDAndUserID(ctx, param.ProjectID, param.OperatorID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Warn("remove project member failed: operator is not project member")
			return nil, ErrProjectMemberNotFound // 上层业务处理成权限不足
		}

		logger.Error("remove project member failed: get operator role error", zap.Error(err))
		return nil, err
	}

	// 这里校验的是被移除用户是否为项目成员
	removedUserRole, err := s.pmr.GetProjectMemberRoleByProjectIDAndUserID(ctx, param.ProjectID, param.RemovedUserID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Warn("remove project member failed: removed user is not project member")
			return nil, ErrProjectMemberNotFound // 上层业务处理成权限不足
		}

		logger.Error("remove project member failed: get removed user role error", zap.Error(err))
		return nil, err
	}
	logger = logger.With(
		zap.String("operator_role", operatorRole),
		zap.String("removed_user_role", removedUserRole),
	)
	// 权限等级大的可以删除小的，平级不能删除 0 代表等级最大
	// 这也保证了最后一定剩余一个 owner！

	// 为什么不直接通过 map 来比较，为了得到对应的错误信息

	// 不允许通过移除成员接口移除自己
	// 如果需要退出项目，单独设计 LeaveProject 接口处理
	if param.OperatorID == param.RemovedUserID {
		logger.Warn("remove project member failed: operator cannot remove self")
		return nil, ErrProjectForbidden
	}

	// 不允许移除 owner
	// owner 转让应该通过 UpdateProjectMember 修改 owner 的逻辑完成
	if removedUserRole == model.ProjectMemberRoleOwner {
		logger.Warn("remove project member failed: cannot remove owner")
		return nil, ErrProjectForbidden
	}

	// 权限等级大的可以删除小的，平级不能删除 0 代表等级最大
	// 这也保证了最后一定剩余一个 owner！
	operatorLevel, ok := model.RoleLevel[operatorRole]
	if !ok {
		logger.Error("remove project member failed: invalid operator role")
		return nil, ErrInvalidProjectMemberRole
	}
	removedUserLevel, ok := model.RoleLevel[removedUserRole]
	if !ok {
		logger.Error("remove project member failed: invalid removed user role")
		return nil, ErrInvalidProjectMemberRole
	}

	// 如果操作者的权限小于删除者
	if operatorLevel >= removedUserLevel {
		logger.Warn("remove project member failed: permission denied",
			zap.Int("operator_level", operatorLevel),
			zap.Int("removed_user_level", removedUserLevel),
		)
		return nil, ErrProjectForbidden
	}

	updatedAt := time.Now()

	// 使用事务保证：
	// 1. 移除项目成员
	// 2. 清空该成员在当前项目下负责的任务负责人
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.pmr.SoftDeleteProjectMember(ctx, tx, param.ProjectID, param.RemovedUserID); err != nil {
			return err
		}

		if err := s.tr.ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx(
			ctx,
			tx,
			param.ProjectID,
			param.RemovedUserID,
			updatedAt,
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		logger.Error("remove project member failed: transaction rollback", zap.Error(err))
		return nil, err
	}

	logger.Info("remove project member success")
	return &dto.RemoveProjectMemberResp{}, nil
}

// projectMemberSvcUserRepo 用户仓储接口
type projectMemberSvcUserRepo interface {
	GetByID(ctx context.Context, id uint64) (*model.User, error)
}

// projectMemberSvcProjectRepo 项目仓储接口
type projectMemberSvcProjectRepo interface {
	ExistsByProjectID(ctx context.Context, id uint64) (bool, error)
}

// projectMemberSvcProjectMemberRepo 项目成员仓储接口
type projectMemberSvcProjectMemberRepo interface {
	CreateWithTx(ctx context.Context, tx *gorm.DB, projectMember *model.ProjectMember) error
	SearchProjectMembers(ctx context.Context, query *repository.ProjectMemberSearchQuery) (*repository.SearchProjectMemberResult, error)
	UpdateProjectMemberRole(ctx context.Context, tx *gorm.DB, projectID, userID uint64, role string) error
	SoftDeleteProjectMember(ctx context.Context, tx *gorm.DB, projectID, userID uint64) error

	ExistsByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (bool, error)
	GetProjectMemberByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (*model.ProjectMember, error)
	CountByProjectIDAndRole(ctx context.Context, projectID uint64, role string) (int64, error)
	GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error)
	GetProjectMemberByProjectIDAndUserIDUnscoped(ctx context.Context, projectID uint64, userID uint64) (*model.ProjectMember, error)
	RestoreProjectMemberWithTx(ctx context.Context, tx *gorm.DB, param *repository.RestoreProjectMemberWithTxParam) error
}

// projectMemberSvcTaskRepo 任务仓储接口
type projectMemberSvcTaskRepo interface {
	ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx(
		ctx context.Context,
		tx *gorm.DB,
		projectID uint64,
		assigneeID uint64,
		updatedAt time.Time,
	) error
}

// transferOwner 转让项目 owner
func (s *ProjectMemberService) transferOwner(
	ctx context.Context,
	param *UpdateProjectMemberParam,
	logger *zap.Logger,
) error {
	return s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		// 被修改者变为新 owner
		if err := s.pmr.UpdateProjectMemberRole(ctx, tx, param.ProjectID, param.ModifiedUserID, model.ProjectMemberRoleOwner); err != nil {
			logger.Error("transfer owner failed: update new owner db error", zap.Error(err))
			return err
		}

		// 原 owner 降级为 member
		if param.ModifierID != param.ModifiedUserID {
			if err := s.pmr.UpdateProjectMemberRole(ctx, tx, param.ProjectID, param.ModifierID, model.ProjectMemberRoleMember); err != nil {
				logger.Error("transfer owner failed: update old owner to member db error", zap.Error(err))
				return err
			}
		}

		return nil
	})
}

// updateToAdmin 修改项目成员为 admin
func (s *ProjectMemberService) updateToAdmin(
	ctx context.Context,
	param *UpdateProjectMemberParam,
	logger *zap.Logger,
) error {
	// admin 人数不能超过限制
	count, err := s.pmr.CountByProjectIDAndRole(ctx, param.ProjectID, model.ProjectMemberRoleAdmin)
	if err != nil {
		logger.Error("update project member to admin failed: count admin error", zap.Error(err))
		return err
	}

	if count >= int64(AdminCount) {
		logger.Warn("update project member to admin failed: admin limit exceeded",
			zap.Int64("admin_count", count),
			zap.Int("admin_limit", AdminCount),
		)
		return ErrExceedsAdminMemberLimit
	}

	return s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.pmr.UpdateProjectMemberRole(ctx, tx, param.ProjectID, param.ModifiedUserID, model.ProjectMemberRoleAdmin); err != nil {
			logger.Error("update project member to admin failed: update role db error", zap.Error(err))
			return err
		}

		return nil
	})
}

// updateToMember 修改项目成员为 member
func (s *ProjectMemberService) updateToMember(
	ctx context.Context,
	param *UpdateProjectMemberParam,
	projectMember *model.ProjectMember,
	logger *zap.Logger,
) error {
	// 不允许直接把 owner 修改为 member
	// owner 转让应该走 transferOwner
	if projectMember.Role == model.ProjectMemberRoleOwner {
		logger.Warn("update project member to member failed: cannot demote owner directly")
		return ErrProjectForbidden
	}

	return s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.pmr.UpdateProjectMemberRole(ctx, tx, param.ProjectID, param.ModifiedUserID, model.ProjectMemberRoleMember); err != nil {
			logger.Error("update project member to member failed: update role db error", zap.Error(err))
			return err
		}

		return nil
	})
}
