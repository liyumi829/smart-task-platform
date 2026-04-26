// Package service
// 业务层，实现用户模块的服务
package service

import (
	"context"
	"errors"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/password"
	"smart-task-platform/internal/pkg/validator"
	"smart-task-platform/internal/repository"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// UserService 认证服务
type UserService struct {
	txMgr    *repository.TxManager // 事务管理器
	userRepo UserUserRepository    // 用户仓库接口
}

// NewAuthService 创建用户服务
func NewUserService(
	txMgr *repository.TxManager,
	userRepo UserUserRepository,
) *UserService {
	return &UserService{
		txMgr:    txMgr,
		userRepo: userRepo,
	}
}

// UpdateUserProfile 更新个人资料
func (s *UserService) UpdateUserProfile(ctx context.Context, userID uint64, nickname, avatar string) (*dto.UpdateProfileResp, error) {
	// 先保证昵称和头像没有前后空格
	nickname = strings.TrimSpace(nickname)
	avatar = strings.TrimSpace(avatar)
	// 参数检验 格式是否合法。如果传入空，我们就认为用户不会更改该参数
	if nickname != "" && !validator.IsValidNickname(nickname) {
		return nil, ErrInvalidNicknameFormat
	}
	if avatar != "" && !validator.IsValidAvatarURL(avatar) {
		return nil, ErrInvalidAvatarURLFormat
	}

	// 先搜索获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			zap.L().Warn("user not found when updating profile",
				zap.Uint64("user_id", userID),
			)
			return nil, ErrUserNotFound
		}
		zap.L().Error("fail to get user by id when updating profile",
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		return nil, err
	}

	// 查看用户状态，如果用户被禁用则返回错误
	if user.Status != model.UserStatusActive {
		zap.L().Warn("update attempt for disabled user",
			zap.Uint64("user_id", user.ID))
		return nil, ErrUserDisabled // 用户被禁用，返回用户被禁用错误
	}

	// 先检验参数是否为空
	if nickname == "" && avatar == "" {
		zap.L().Info("skip update user profile because nickname and avatar are both empty",
			zap.Uint64("user_id", userID),
		)

		// 如果两个参数都为空，那么就不用修改，直接返回即可
		return &dto.UpdateProfileResp{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		}, nil
	}
	// 获取新昵称，避免空字符串注入
	newNickname := user.Nickname
	newAvatar := user.Avatar
	if nickname != "" {
		newNickname = nickname
	}
	if avatar != "" {
		newAvatar = avatar
	}

	// 使用事务更新用户个人资料
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		if err := s.userRepo.UpdateUserProfileWithTx(
			ctx,
			tx,
			userID,
			newNickname,
			newAvatar,
		); err != nil {
			zap.L().Error("fail to update user profile with tx",
				zap.Uint64("user_id", userID),
				zap.String("nickname", nickname),
				zap.String("avatar", avatar),
				zap.Error(err),
			)
			return err
		}
		return nil
	})
	if err != nil {
		zap.L().Error("fail to update user profile",
			zap.Uint64("user_id", userID),
			zap.String("nickname", newNickname),
			zap.String("avatar", newAvatar),
			zap.Error(err))
		return nil, err
	}

	zap.L().Info("user profile updated successfully",
		zap.Uint64("user_id", userID),
	)

	return &dto.UpdateProfileResp{
		ID:       user.ID,
		Username: user.Username,
		Nickname: newNickname,
		Avatar:   newAvatar,
	}, nil
}

// UpdateUserPassword 修改密码
func (s *UserService) UpdateUserPassword(ctx context.Context, userID uint64, oldPassword, newPassword string) (*dto.UpdateUserPasswordResp, error) {
	// 参数检验 检验密码格式是否正确，检验旧密码/新密码是否和原来密码相同，
	// 检验密码格式
	if !validator.IsValidPassword(oldPassword) || !validator.IsValidPassword(newPassword) {
		// 密码格式不正确
		zap.L().Warn("invalid password format when updating password",
			zap.Uint64("user_id", userID),
		)
		return nil, ErrInvalidPasswordFormat
	}

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID) // 获取到用户信息
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			zap.L().Warn("user not found when updating password",
				zap.Uint64("user_id", userID),
			)
			return nil, ErrUserNotFound
		}
		zap.L().Error("fail to get user by id when updating password",
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		return nil, err
	}

	// 查看用户状态，如果用户被禁用则返回错误
	if user.Status != model.UserStatusActive {
		zap.L().Warn("update password attempt for disabled user",
			zap.Uint64("user_id", user.ID))
		return nil, ErrUserDisabled // 用户被禁用，返回用户被禁用错误
	}

	// 检验旧密码是否匹配
	if ok := password.CheckPasswordHash(oldPassword, user.PasswordHash); !ok {
		// 和旧密码不相同
		zap.L().Warn("old password mismatch when updating password",
			zap.Uint64("user_id", userID),
		)
		return nil, ErrOldPasswordMismatch
	}

	// 检查旧密码和新密码是否相同
	// 刚刚进行了旧密码匹配，所以旧密码的明文就是用户密码，直接进行比较
	if oldPassword == newPassword {
		zap.L().Warn("new password is same as old password",
			zap.Uint64("user_id", userID),
		)
		return nil, ErrNewPasswordSameAsOld
	}

	// 参数检验通过，更新密码
	newPasswordHash, err := password.HashPassword(newPassword)
	if err != nil {
		zap.L().Error("fail to hash new password",
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		return nil, err
	}

	err = s.txMgr.Transaction(ctx,
		func(tx *gorm.DB) error {
			if err := s.userRepo.UpdateUserPasswordWithTx(ctx, tx, userID, newPasswordHash); err != nil {
				zap.L().Error("fail to update user password with tx",
					zap.Uint64("user_id", userID),
					zap.Error(err),
				)
				return err
			}
			return nil
		})

	if err != nil {
		zap.L().Error("fail to update user password",
			zap.Uint64("user_id", userID),
			zap.Error(err),
		)
		return nil, err
	}

	zap.L().Info("user password updated successfully",
		zap.Uint64("user_id", userID))

	return &dto.UpdateUserPasswordResp{}, nil
}

// GetUserPublicInfo 获取用户公开信息
func (s *UserService) GetUserPublicInfo(ctx context.Context, targetUserID uint64) (*dto.UserPublicProfileResp, error) {
	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, targetUserID) // 获取到用户信息
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			zap.L().Warn("user not found when getting public user info",
				zap.Uint64("target_user_id", targetUserID),
			)
			return nil, ErrUserNotFound
		}
		zap.L().Error("fail to get user by id when getting public user info",
			zap.Uint64("target_user_id", targetUserID),
			zap.Error(err),
		)
		return nil, err
	}

	zap.L().Info("get user public info successfully",
		zap.Uint64("target_user_id", targetUserID),
	)

	return &dto.UserPublicProfileResp{
		UserPublicProfile: dto.UserPublicProfile{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		},
	}, nil
}

// ListUsers 用户搜索列表（分页）
func (s *UserService) ListUsers(ctx context.Context, page, pageSize int, key string) (*dto.UserSearchListResp, error) {
	// 参数检查。
	keyword := strings.TrimSpace(key) // 清理关键字空格
	if keyword == "" {
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = dto.MinPageSize
		}
		if pageSize > dto.MaxPageSize {
			pageSize = dto.MaxPageSize
		}

		zap.L().Info("skip search user list because keyword is empty",
			zap.Int("page", page),
			zap.Int("page_size", pageSize),
		)

		return &dto.UserSearchListResp{
			List:     []*dto.UserSearchItem{},
			Total:    0,
			Page:     page,
			PageSize: 0,
		}, nil // 不做查询，防止全量查询
	}

	// 分页参数兜底。
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = dto.MinPageSize
	}
	if pageSize > dto.MaxPageSize {
		pageSize = dto.MaxPageSize
	}

	// 参数校验完成
	// 开始搜索用户：
	users, total, err := s.userRepo.SearchUsers(ctx, &repository.UserSearchQuery{
		Keyword:  keyword,
		Page:     page,
		PageSize: pageSize,
	})
	// 差错处理
	if err != nil {
		// 搜索失败
		zap.L().Error("fail to search user list",
			zap.String("keyword", keyword),
			zap.Int("page", page),
			zap.Int("page_size", pageSize),
			zap.Error(err),
		)
		return nil, err
	}

	userItems := make([]*dto.UserSearchItem, 0, len(users))
	for _, user := range users {
		userItems = append(userItems,
			&dto.UserSearchItem{
				ID:       user.ID,
				Username: user.Username,
				Nickname: user.Nickname,
				Avatar:   user.Avatar,
			})
	}

	zap.L().Info("search user list successfully",
		zap.String("keyword", keyword),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
		zap.Int64("total", total),
	)

	return &dto.UserSearchListResp{
		List:     userItems,
		Total:    int(total),
		Page:     page,
		PageSize: len(userItems),
	}, nil
}

// UserUserRepository 用户服务使用的仓储操作
type UserUserRepository interface {
	GetByID(ctx context.Context, id uint64) (*model.User, error)                                                   // 获取用户
	UpdateUserProfileWithTx(ctx context.Context, tx *gorm.DB, userID uint64, nickname string, avatar string) error // 更新个人资料
	UpdateUserPasswordWithTx(ctx context.Context, tx *gorm.DB, userID uint64, password string) error               // 更新密码
	SearchUsers(ctx context.Context, query *repository.UserSearchQuery) ([]*model.User, int64, error)
}
