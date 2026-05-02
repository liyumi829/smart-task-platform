// internal/service/util.go
// Package service 服务用到的共用工具函数
package service

import (
	"context"
	"errors"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	"strings"
	"time"

	"go.uber.org/zap"
)

// fixPageParams 分页参数自动矫正（兜底处理）
func fixPageParams(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = dto.MinPageSize
	}
	if pageSize > dto.MaxPageSize {
		pageSize = dto.MaxPageSize
	}
	return page, pageSize
}

// parseOptionalISOTime 解析可选 ISO 时间字符串
func parseOptionalISOTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	t, err := utils.ISO2Time(value)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

// isValidMemberRole 校验项目成员角色是否合法
func isValidMemberRole(role string) bool {
	switch role {
	case model.ProjectMemberRoleOwner,
		model.ProjectMemberRoleAdmin,
		model.ProjectMemberRoleMember:
		return true
	default:
		return false
	}
}

// isValidProjectStatus 校验项目状态是否为合法值
func isValidProjectStatus(status string) bool {
	switch status {
	case model.ProjectStatusActive, model.ProjectStatusArchived:
		return true
	default:
		return false
	}
}

// isValidTaskPriority 校验任务优先级是否为合法值
func isValidTaskPriority(priority string) bool {
	switch priority {
	case model.TaskPriorityHigh,
		model.TaskPriorityUrgent,
		model.TaskPriorityMedium,
		model.TaskPriorityLow:
		return true
	default:
		return false
	}
}

// isValidTaskStatus 校验任务状态是否为合法值
func isValidTaskStatus(status string) bool {
	switch status {
	case model.TaskStatusTodo,
		model.TaskStatusInProgress,
		model.TaskStatusDone,
		model.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// isValidTaskSortBy 检验排序规则是否是合法值
func isValidTaskSortBy(sortBy string) bool {
	switch sortBy {
	case dto.SortByPriority,
		dto.SortByStatus,
		dto.SortByTitle,
		dto.SortByDueDate,
		dto.SortByCreateTime:
		return true

	default:
		return false
	}
}

// isValidTaskSortOrder 检查排序顺序是否合法值
func IsValidTaskSortOrder(sortOrder string) bool {
	switch sortOrder {
	case
		dto.UpperAsc,
		dto.UpperDesc,
		dto.LowerAsc,
		dto.LowerDesc:
		return true
	default:
		return false
	}
}

// buildUserPublicProfile 构造用户公开信息 DTO
func buildUserPublicProfile(user *model.User) *dto.UserPublicProfile {
	// 未预加载 Owner 或 Owner 数据异常时，返回 nil
	// 空值保护
	if user == nil {
		return nil
	}

	return &dto.UserPublicProfile{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}
}

// buildTaskBaseFields 构造任务基础信息 DTO
func buildTaskBaseFields(task *model.Task) *dto.TaskBaseFields {
	// 空值保护
	if task == nil {
		return nil
	}

	return &dto.TaskBaseFields{
		ID:         task.ID,
		ProjectID:  task.ProjectID,
		Title:      task.Title,
		Status:     task.Status,
		Priority:   task.Priority,
		AssigneeID: task.AssigneeID,
		DueDate:    task.DueDate,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
	}
}

// buildProjectPublicProfile 构造任务基础信息 DTO
func buildProjectPublicProfile(project *model.Project) *dto.ProjectPublicProfile {
	// 空值保护
	if project == nil {
		return nil
	}

	return &dto.ProjectPublicProfile{
		ID:   project.ID,
		Name: project.Name,
	}
}

// roleLevelInvoker 项目成员角色等级查询接口
type roleLevelInvoker interface {
	GetProjectMemberByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (*model.ProjectMember, error)
}

// getProjectMemberRoleLevel 获取项目成员角色和角色权限等级
//
// model.RoleLevel 约定：
//   - owner  -> 0
//   - admin  -> 1
//   - member -> 2
//
// 数字越小，权限越高。
func getProjectMemberRoleLevel(
	ctx context.Context,
	invoker roleLevelInvoker,
	projectID uint64,
	userID uint64,
	logger *zap.Logger,
) (string, int, error) {
	// 查询用户在项目中的成员信息
	member, err := invoker.GetProjectMemberByProjectIDAndUserID(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Warn("project member role level check failed: project member not found",
				zap.Uint64("project_id", projectID),
				zap.Uint64("user_id", userID),
			)
			return "", 0, ErrProjectMemberNotFound
		}

		logger.Error("project member role level check failed: get project member error",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		return "", 0, err
	}

	// 根据角色获取权限等级
	level, ok := model.RoleLevel[member.Role]
	if !ok {
		logger.Error("project member role level check failed: invalid member role",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("member_role", member.Role),
		)
		return "", 0, ErrInvalidProjectMemberRole
	}

	return member.Role, level, nil
}

// projectRoleInvoker 项目成员角色查询接口
type projectRoleInvoker interface {
	GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error)
}

// hasProjectManagePermission 判断用户在项目中是否具有管理权限
//
// 会判断 UserID 是否存在
//
// 会判断 ProjectID 是否存在
//   - 不存在都当作是无权限
func hasProjectManagePermission(
	ctx context.Context,
	invoker projectRoleInvoker,
	projectID uint64,
	userID uint64,
	logger *zap.Logger,
) (bool, error) {
	// 查询用户在项目中的角色
	role, err := invoker.GetProjectMemberRoleByProjectIDAndUserID(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrProjectMemberNotFound) {
			logger.Warn("project manage permission check failed: project member not found",
				zap.Uint64("project_id", projectID),
				zap.Uint64("user_id", userID),
			)
			return false, ErrProjectMemberNotFound
		}

		logger.Error("project manage permission check failed: get project member role error",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		return false, err
	}

	// 判断角色是否具有项目管理权限
	if role == model.ProjectMemberRoleAdmin || role == model.ProjectMemberRoleOwner {
		logger.Debug("project manage permission check success: permission granted",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("member_role", role),
		)
		return true, nil
	}

	logger.Warn("project manage permission check failed: permission denied",
		zap.Uint64("project_id", projectID),
		zap.Uint64("user_id", userID),
		zap.String("member_role", role),
	)

	return false, nil
}
