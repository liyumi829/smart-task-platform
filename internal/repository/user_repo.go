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
	ErrUserIsEmpty           = errors.New("user cannot be empty")             // 用户对象为空
	ErrPasswordIsEmpty       = errors.New("password cannot be empty")         // 密码不能为空
	ErrUserNotFound          = errors.New("user not found")                   // 用户未找到
	ErrInvalidUserParam      = errors.New("invalid user param")               // 用户参数非法
	ErrUserUpdateDataIsEmpty = errors.New("user update data cannot be empty") // 用户更新数据为空
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
		return ErrUserIsEmpty
	}

	return getDB(ctx, r.db, tx).
		Create(user).Error
}

// GetByID 根据 ID 查询用户
func (r *userRepository) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	if id == 0 {
		return nil, ErrInvalidUserParam
	}

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
	if account == "" {
		return nil, ErrInvalidUserParam
	}
	var user model.User
	err := getDB(ctx, r.db, nil).
		Where(model.UserColumnUsername+" = ? OR "+model.UserColumnEmail+" = ?", account, account).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

// ExistsByUsername 检查用户名是否存在。
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return false, ErrInvalidUserParam
	}

	var id uint64

	err := getDB(ctx, r.db, nil).
		Model(&model.User{}).
		Select(model.UserColumnID).
		Where(model.UserColumnUsername+" = ?", username).
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

// ExistsByEmail 检查邮箱是否存在。
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	if email == "" {
		return false, ErrInvalidUserParam
	}

	var id uint64

	err := getDB(ctx, r.db, nil).
		Model(&model.User{}).
		Select(model.UserColumnID).
		Where(model.UserColumnEmail+" = ?", email).
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

// UpdateLastLoginAtWithTx 事务更新最后登录时间。
func (r *userRepository) UpdateLastLoginAtWithTx(ctx context.Context, tx *gorm.DB, userID uint64, loginAt time.Time) error {
	if userID == 0 || loginAt.IsZero() {
		return ErrInvalidUserParam
	}

	result := getDB(ctx, r.db, tx).
		Model(&model.User{}).
		Where(model.UserColumnID+" = ?", userID).
		Update(model.UserColumnLastLoginAt, loginAt)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateUserProfileWithTx 事务更新用户昵称和头像
func (r *userRepository) UpdateUserProfileWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	nickname string,
	avatar string,
) error {

	if userID == 0 {
		return ErrInvalidUserParam
	}

	updates := map[string]interface{}{
		model.UserColumnUpdatedAt: time.Now(),
	}

	if nickname != "" {
		updates[model.UserColumnNickname] = nickname
	}
	if avatar != "" {
		updates[model.UserColumnAvatar] = avatar
	}

	if len(updates) == 1 {
		return ErrUserUpdateDataIsEmpty
	}

	result := getDB(ctx, r.db, tx).
		Model(&model.User{}).
		Where(model.UserColumnID+" = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateUserPasswordWithTx 事务修改用户密码。
func (r *userRepository) UpdateUserPasswordWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	password string,
) error {
	if userID == 0 {
		return ErrInvalidUserParam
	}
	if password == "" {
		return ErrPasswordIsEmpty
	}

	result := getDB(ctx, r.db, tx).
		Model(&model.User{}).
		Where(model.UserColumnID+" = ?", userID).
		Updates(map[string]interface{}{
			model.UserColumnPassword:  password,
			model.UserColumnUpdatedAt: time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UserSearchQuery 用户搜索查询参数。
type UserSearchQuery struct {
	Keyword string
	SearchQuery
}

type SearchUserResult = SearchResult[*model.User]

// SearchUsers 搜索用户列表。
// 说明：
// 1. 支持按 username 前缀模糊搜索
// 2. 支持分页
// 3. total 按需查询，避免每次 COUNT(*) 带来的性能开销
// 4. hasMore 通过 Limit(pageSize + 1) 判断，不依赖 total
// 5. keyword 为空时直接返回空列表，避免全表扫描
func (r *userRepository) SearchUsers(ctx context.Context, query *UserSearchQuery) (*SearchUserResult, error) {
	// 参数校验：避免非法分页和空关键词导致全表扫描。
	if query == nil || query.Page <= 0 || query.PageSize <= 0 || query.Keyword == "" {
		return &SearchUserResult{
			List:    []*model.User{},
			Total:   nil,
			HasMore: false,
		}, nil
	}

	// 计算分页偏移量。
	offset := (query.Page - 1) * query.PageSize

	// 使用前缀匹配，便于数据库利用索引。
	likePattern := query.Keyword + "%"

	// 构建基础查询条件。
	baseDB := getDB(ctx, r.db, nil).
		Model(&model.User{}).
		Where(model.UserColumnStatus+" = ?", model.UserStatusActive).
		Where(model.UserColumnUsername+" LIKE ?", likePattern)

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
			return &SearchUserResult{
				List:    []*model.User{},
				Total:   totalPtr,
				HasMore: false,
			}, nil
		}
	}

	// 多查一条用于判断是否还有下一页。
	limit := query.PageSize + 1

	users := make([]*model.User, 0, limit)
	if err := baseDB.
		Select([]string{
			model.UserColumnID,
			model.UserColumnUsername,
			model.UserColumnNickname,
			model.UserColumnAvatar,
		}).
		Order(model.UserColumnUsername + " ASC").
		Order(model.UserColumnID + " DESC").
		Offset(offset).
		Limit(limit).
		Find(&users).Error; err != nil {
		return nil, err
	}

	// 通过多查的一条数据判断 hasMore，不依赖 total。
	hasMore := len(users) > query.PageSize
	if hasMore {
		users = users[:query.PageSize]
	}

	return &SearchUserResult{
		List:    users,
		Total:   totalPtr,
		HasMore: hasMore,
	}, nil
}

// ExistsByUserID 根据用户ID判断用户是否存在
func (r *userRepository) ExistsByUserID(ctx context.Context, userID uint64) (bool, error) {
	var id int64
	err := getDB(ctx, r.db, nil).
		Model(&model.User{}).
		Select(model.UserColumnID).
		Where(model.UserColumnID+" = ?", userID).
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
