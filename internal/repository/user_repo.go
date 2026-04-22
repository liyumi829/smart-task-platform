// Package repository 实现了用户相关的数据访问层，提供了对用户数据的增删改查操作。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"smart-task-platform/internal/model"
)

var (
	ErrUserNotFound = errors.New("user not found") // 用户未找到错误消息
)

// UserRepository 用户仓储接口
type UserRepository interface {
	// 创建用户
	Create(ctx context.Context, user *model.User) error

	// 根据 ID 查询用户
	GetByID(ctx context.Context, id uint64) (*model.User, error)

	// 根据 Account 查询用户（用户名或邮箱）
	GetByAccount(ctx context.Context, account string) (*model.User, error)

	// 检查用户名是否存在
	ExistsByUsername(ctx context.Context, username string) (bool, error)

	// 检查邮箱是否存在
	ExistsByEmail(ctx context.Context, email string) (bool, error)

	// 更新最后登录时间 在用户登录退出口调用
	UpdateLastLoginAtWithTx(ctx context.Context, tx *gorm.DB, userID uint64, loginAt time.Time) error
}

// userRepository 用户仓储实现
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// Create 创建用户
func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	if user == nil {
		return errors.New("user is nil")
	}

	return r.db.WithContext(ctx).Create(user).Error
}

// GetByID 根据 ID 查询用户
func (r *userRepository) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 没有找到对应的用户，返回自定义的 ErrUserNotFound 错误
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

// GetByAccount 根据用户名或邮箱查询用户
func (r *userRepository) GetByAccount(ctx context.Context, account string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("username = ? OR email = ?", account, account).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

// ExistsByUsername 检查用户名是否存在
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("username = ?", username).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否存在
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("email = ?", email).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// UpdateLastLoginAtWithTx 事务更新最后登录时间
func (r *userRepository) UpdateLastLoginAtWithTx(ctx context.Context, tx *gorm.DB, userID uint64, loginAt time.Time) error {
	return tx.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Update("last_login_at", loginAt).Error
}
