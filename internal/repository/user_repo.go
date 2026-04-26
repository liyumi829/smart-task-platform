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
	ErrUserIsEmpty     = errors.New("user cannot be empty")     // 用户对象为空
	ErrPasswordIsEmpty = errors.New("password cannot be empty") // 密码不能为空
	ErrUserNotFound    = errors.New("user not found")           // 用户未找到错误消息
)

// userRepository 用户仓储实现
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储
func NewUserRepository(db *gorm.DB) *userRepository {
	return &userRepository{db: db}
}

// Create 创建用户
func (r *userRepository) Create(ctx context.Context, tx *gorm.DB, user *model.User) error {
	if user == nil {
		return errors.New("user is nil")
	}

	return getDB(ctx, r.db, tx).
		Create(user).Error
}

// GetByID 根据 ID 查询用户
func (r *userRepository) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	var user model.User
	err := getDB(ctx, r.db, nil).
		Where(model.UserColumnID+" = ?", id).
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
	err := getDB(ctx, r.db, nil).
		Where(
			r.db.Where(model.UserColumnUsername, account).
				Or(model.UserColumnEmail, account),
		).
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
	err := getDB(ctx, r.db, nil).
		Model(&model.User{}).
		Where(model.UserColumnUsername+" = ?", username).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否存在
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := getDB(ctx, r.db, nil).
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
	return getDB(ctx, r.db, tx).
		Model(&model.User{}).
		Where(model.UserColumnID+" = ?", userID).
		Update(model.UserColumnLastLoginAt, loginAt).Error
}

// UpdateUserProfileWithTx 事务更新用户昵称和头像
func (r *userRepository) UpdateUserProfileWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	nickname string,
	avatar string,
) error {

	updates := map[string]interface{}{
		model.UserColumnUpdatedAt: time.Now(),
	}

	if nickname != "" {
		updates[model.UserColumnNickname] = nickname
	}
	if avatar != "" {
		updates[model.UserColumnAvatar] = avatar
	}

	return getDB(ctx, r.db, tx).
		Model(&model.User{}).
		Where(model.UserColumnID+" = ?", userID).
		Updates(updates).Error
}

// UpdateUserPasswordWithTx 事务修改用户的密码
func (r *userRepository) UpdateUserPasswordWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	password string,
) error {
	if password == "" {
		return ErrPasswordIsEmpty // 返回密码为空的错误
	}

	return getDB(ctx, r.db, tx).
		Model(&model.User{}).
		Where(model.UserColumnID+" = ?", userID).
		Updates(map[string]interface{}{
			model.UserColumnPassword:  password,
			model.UserColumnUpdatedAt: time.Now(),
		}).Error
}

// UserSearchQuery 用户搜索查询参数
type UserSearchQuery struct {
	Page     int    // 页码
	PageSize int    // 页码大小
	Keyword  string // 搜索关键词
}

// SearchUsers 搜索用户列表
// 说明：
// 1. 支持按 username 模糊搜索
// 2. 支持分页
// 3. 返回当前条件下总数 total
// 4. keyword 为空时直接返回空列表，避免全表扫描
func (r *userRepository) SearchUsers(ctx context.Context, query *UserSearchQuery) ([]*model.User, int64, error) {
	var (
		users []*model.User
		total int64
	)

	// 获取参数
	page := query.Page
	pageSize := query.PageSize

	offset := (page - 1) * pageSize    // 偏移量
	likePattern := query.Keyword + "%" // 模糊匹配关键词 key%

	db := getDB(ctx, r.db, nil).
		Model(&model.User{}).
		Where(model.UserColumnStatus, model.UserStatusActive).
		Where(model.UserColumnUsername+" LIKE ?", likePattern)

	// 获取查询结果的查询总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 如果没有数据，直接返回，避免无意义查询
	if total == 0 {
		return []*model.User{}, 0, nil
	}

	// 查询当前页数据
	err := db.Select(
		model.UserColumnID,
		model.UserColumnUsername,
		model.UserColumnNickname,
		model.UserColumnAvatar,
	).
		Order(model.UserColumnUsername + " ASC"). // 以用户名进行排序升序排序
		Offset(offset).                           // 偏移量
		Limit(pageSize).                          // limit限制
		Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
