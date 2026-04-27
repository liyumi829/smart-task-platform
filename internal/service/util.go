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
	"unicode/utf8"

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

// buildUserPublicProfile 构造用户公开信息 DTO
func buildUserPublicProfile(user *model.User) *dto.UserPublicProfile {
	// 未预加载 Owner 或 Owner 数据异常时，返回 nil
	// 空值保护
	if user.ID <= 0 {
		return nil
	}

	return &dto.UserPublicProfile{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}
}

// isValidDescription 控制描述在 200 字以内
func isValidDescription(s string) bool {
	return utf8.RuneCountInString(s) <= 10
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
	pmr roleLevelInvoker,
	projectID uint64,
	userID uint64,
	logger *zap.Logger,
) (string, int, error) {
	// 查询用户在项目中的成员信息
	member, err := pmr.GetProjectMemberByProjectIDAndUserID(ctx, projectID, userID)
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

// roleLevelInvoker 项目成员角色等级查询接口
type roleLevelInvoker interface {
	GetProjectMemberByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (*model.ProjectMember, error)
}
