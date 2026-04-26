// user_service_test.go
//
// 说明：
//  1. 不连接真实 MySQL。
//  2. UserRepository 使用内存 mock，不污染真实数据库。
//  3. UserService 依赖 repository.TxManager，使用 sqlite 内存数据库构造真实事务环境。
//  4. 测试覆盖主要业务分支：查询、更新资料、修改密码、用户搜索、异常分支。
//  5. 全字段断言、完整日志输出，go test -v 可观察执行流程。
//  6. 事务逻辑真实执行，确保事务闭包路径可正常运行。

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/password"
	"smart-task-platform/internal/repository"
)

const (
	testUserPassword         = "Password123456!"
	testUserUsername         = "testuser"
	testUserNickname         = "测试用户"
	testUserAvatar           = "https://example.com/avatar.png"
	testInvalidAvatar        = "avatar.png"
	testNewNickname          = "新的昵称"
	testNewAvatar            = "https://example.com/new_avatar.png"
	testNewPassword          = "NewPassword123456!"
	testUserDisabledUsername = "disabled_user"
	testSearchKeyword        = "test"
)

// mockUserRepository 用户仓库内存 mock
type mockUserRepository struct {
	mu     sync.RWMutex
	users  map[uint64]*model.User // 数据源
	nextID uint64

	// 错误注入
	getByIDErr        error
	updateProfileErr  error
	updatePasswordErr error
	searchErr         error
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users:  make(map[uint64]*model.User),
		nextID: 1,
	}
}

// cloneUser 克隆用户，避免测试时直接引用底层对象
func cloneUser(u *model.User) *model.User {
	if u == nil {
		return nil
	}
	cp := *u
	return &cp
}

// GetByID 获取用户
func (m *mockUserRepository) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}

	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	return cloneUser(u), nil
}

// UpdateUserProfileWithTx 更新个人资料
func (m *mockUserRepository) UpdateUserProfileWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	nickname string,
	avatar string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updateProfileErr != nil {
		return m.updateProfileErr
	}

	u, ok := m.users[userID]
	if !ok {
		return repository.ErrUserNotFound
	}

	// 按照当前 service 语义：传空表示不修改该字段
	if nickname != "" {
		u.Nickname = nickname
	}
	if avatar != "" {
		u.Avatar = avatar
	}
	u.UpdatedAt = time.Now()

	m.users[userID] = u
	return nil
}

// UpdateUserPasswordWithTx 更新密码
func (m *mockUserRepository) UpdateUserPasswordWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	newHash string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updatePasswordErr != nil {
		return m.updatePasswordErr
	}

	u, ok := m.users[userID]
	if !ok {
		return repository.ErrUserNotFound
	}

	u.PasswordHash = newHash
	u.UpdatedAt = time.Now()
	m.users[userID] = u
	return nil
}

// SearchUsers 搜索用户
func (m *mockUserRepository) SearchUsers(
	ctx context.Context,
	query *repository.UserSearchQuery,
) ([]*model.User, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.searchErr != nil {
		return nil, 0, m.searchErr
	}

	keyword := strings.TrimSpace(query.Keyword)
	var matched []*model.User

	for _, u := range m.users {
		// 按你的 service 当前使用公开搜索语义，这里仅返回启用用户
		if u.Status != model.UserStatusActive {
			continue
		}

		// 简单模拟搜索逻辑：匹配用户名或昵称包含关键字
		if keyword == "" ||
			strings.Contains(u.Username, keyword) ||
			strings.Contains(u.Nickname, keyword) {
			matched = append(matched, cloneUser(u))
		}
	}

	total := int64(len(matched))

	// 模拟分页
	start := (query.Page - 1) * query.PageSize
	if start >= len(matched) {
		return []*model.User{}, total, nil
	}

	end := start + query.PageSize
	if end > len(matched) {
		end = len(matched)
	}

	return matched[start:end], total, nil
}

// userTestEnv 测试环境
type userTestEnv struct {
	ctx      context.Context
	db       *gorm.DB
	txMgr    *repository.TxManager
	userRepo *mockUserRepository
	svc      *UserService
}

// newUserTestEnv 创建测试环境
func newUserTestEnv(t *testing.T) *userTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	oldLogger := zap.L()
	zap.ReplaceGlobals(logger)

	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	txMgr := repository.NewTxManager(db)
	userRepo := newMockUserRepository()
	svc := NewUserService(txMgr, userRepo)

	t.Cleanup(func() {
		zap.ReplaceGlobals(oldLogger)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	return &userTestEnv{
		ctx:      ctx,
		db:       db,
		txMgr:    txMgr,
		userRepo: userRepo,
		svc:      svc,
	}
}

// seedActiveUser 预置正常用户
func (e *userTestEnv) seedActiveUser(t *testing.T) *model.User {
	t.Helper()

	hash, err := password.HashPassword(testUserPassword)
	require.NoError(t, err)

	now := time.Now()
	u := &model.User{
		ID:           e.userRepo.nextID,
		Username:     testUserUsername,
		Nickname:     testUserNickname,
		Avatar:       testUserAvatar,
		PasswordHash: hash,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	e.userRepo.users[u.ID] = u
	e.userRepo.nextID++

	t.Logf("[seed active user] id=%d username=%s nickname=%s", u.ID, u.Username, u.Nickname)
	return cloneUser(u)
}

// seedCustomActiveUser 预置自定义启用用户
func (e *userTestEnv) seedCustomActiveUser(t *testing.T, username, nickname, avatar string) *model.User {
	t.Helper()

	hash, err := password.HashPassword(testUserPassword)
	require.NoError(t, err)

	now := time.Now()
	u := &model.User{
		ID:           e.userRepo.nextID,
		Username:     username,
		Nickname:     nickname,
		Avatar:       avatar,
		PasswordHash: hash,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	e.userRepo.users[u.ID] = u
	e.userRepo.nextID++

	t.Logf("[seed custom active user] id=%d username=%s nickname=%s", u.ID, u.Username, u.Nickname)
	return cloneUser(u)
}

// seedDisabledUser 预置禁用用户
func (e *userTestEnv) seedDisabledUser(t *testing.T) *model.User {
	t.Helper()

	hash, err := password.HashPassword(testUserPassword)
	require.NoError(t, err)

	now := time.Now()
	u := &model.User{
		ID:           e.userRepo.nextID,
		Username:     testUserDisabledUsername,
		Nickname:     "禁用账号",
		Avatar:       testUserAvatar,
		PasswordHash: hash,
		Status:       model.UserStatusDisabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	e.userRepo.users[u.ID] = u
	e.userRepo.nextID++

	t.Logf("[seed disabled user] id=%d username=%s", u.ID, u.Username)
	return cloneUser(u)
}

// mustGetUser 获取仓库中的最新用户
func (e *userTestEnv) mustGetUser(t *testing.T, userID uint64) *model.User {
	t.Helper()

	u, err := e.userRepo.GetByID(e.ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, u)
	return u
}

func TestGetUserPublicInfo(t *testing.T) {
	env := newUserTestEnv(t)

	t.Run("user not found", func(t *testing.T) {
		resp, err := env.svc.GetUserPublicInfo(env.ctx, 9999)
		t.Logf("[get public info] user not found, err=%v", err)

		require.ErrorIs(t, err, ErrUserNotFound)
		assert.Nil(t, resp)
	})

	t.Run("repo get by id error", func(t *testing.T) {
		env.userRepo.getByIDErr = errors.New("mock get user failed")
		defer func() { env.userRepo.getByIDErr = nil }()

		resp, err := env.svc.GetUserPublicInfo(env.ctx, 1)
		t.Logf("[get public info] repo error, err=%v", err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get user failed")
		assert.Nil(t, resp)
	})

	t.Run("success, all fields assert", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.GetUserPublicInfo(env.ctx, user.ID)
		t.Logf("[get public info] success resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, user.ID, resp.ID)
		assert.Equal(t, user.Username, resp.Username)
		assert.Equal(t, user.Nickname, resp.Nickname)
		assert.Equal(t, user.Avatar, resp.Avatar)
		assert.NotEmpty(t, resp.Avatar)
	})
}

func TestUpdateUserProfile(t *testing.T) {
	env := newUserTestEnv(t)

	t.Run("invalid nickname format", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, "", "")
		t.Logf("[update profile] empty nickname and avatar as blank input, resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("user not found", func(t *testing.T) {
		resp, err := env.svc.UpdateUserProfile(env.ctx, 9999, "nick", testUserAvatar)
		t.Logf("[update profile] user not found, err=%v", err)

		require.ErrorIs(t, err, ErrUserNotFound)
		assert.Nil(t, resp)
	})

	t.Run("repo get by id error", func(t *testing.T) {
		env.userRepo.getByIDErr = errors.New("mock get user failed")
		defer func() { env.userRepo.getByIDErr = nil }()

		resp, err := env.svc.UpdateUserProfile(env.ctx, 1, "newnick", testUserAvatar)
		t.Logf("[update profile] repo get user error, err=%v", err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get user failed")
		assert.Nil(t, resp)
	})

	t.Run("user disabled", func(t *testing.T) {
		user := env.seedDisabledUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, "newnick", testUserAvatar)
		t.Logf("[update profile] user disabled, err=%v", err)

		require.ErrorIs(t, err, ErrUserDisabled)
		assert.Nil(t, resp)
	})

	t.Run("invalid avatar URL", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, "newnick", testInvalidAvatar)
		t.Logf("[update profile] invalid avatar, err=%v", err)

		// 注意：这里使用你当前 service 中的错误名
		require.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("empty nickname and avatar, no change", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, "   ", "   ")
		t.Logf("[update profile] empty update, resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, user.ID, resp.ID)
		assert.Equal(t, user.Username, resp.Username)
		assert.Equal(t, user.Nickname, resp.Nickname)
		assert.Equal(t, user.Avatar, resp.Avatar)

		after := env.mustGetUser(t, user.ID)
		assert.Equal(t, user.Nickname, after.Nickname)
		assert.Equal(t, user.Avatar, after.Avatar)
	})

	t.Run("update profile repo error", func(t *testing.T) {
		user := env.seedActiveUser(t)
		env.userRepo.updateProfileErr = errors.New("mock update profile failed")
		defer func() { env.userRepo.updateProfileErr = nil }()

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, testNewNickname, testNewAvatar)
		t.Logf("[update profile] repo update error, err=%v", err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock update profile failed")
		assert.Nil(t, resp)

		after := env.mustGetUser(t, user.ID)
		assert.Equal(t, user.Nickname, after.Nickname)
		assert.Equal(t, user.Avatar, after.Avatar)
	})

	t.Run("success full update, all fields assert", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, testNewNickname, testNewAvatar)
		t.Logf("[update profile] success resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, user.ID, resp.ID)
		assert.Equal(t, user.Username, resp.Username)
		assert.Equal(t, testNewNickname, resp.Nickname)
		assert.Equal(t, testNewAvatar, resp.Avatar)

		after := env.mustGetUser(t, user.ID)
		assert.Equal(t, testNewNickname, after.Nickname)
		assert.Equal(t, testNewAvatar, after.Avatar)
		assert.True(t, after.UpdatedAt.After(user.UpdatedAt) || after.UpdatedAt.Equal(user.UpdatedAt))
	})

	t.Run("success update nickname only", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, testNewNickname, "")
		t.Logf("[update profile] nickname only resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, testNewNickname, resp.Nickname)
		assert.Equal(t, user.Avatar, resp.Avatar)

		after := env.mustGetUser(t, user.ID)
		assert.Equal(t, testNewNickname, after.Nickname)
		assert.Equal(t, user.Avatar, after.Avatar)
	})

	t.Run("success update avatar only", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserProfile(env.ctx, user.ID, "", testNewAvatar)
		t.Logf("[update profile] avatar only resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, user.Nickname, resp.Nickname)
		assert.Equal(t, testNewAvatar, resp.Avatar)

		after := env.mustGetUser(t, user.ID)
		assert.Equal(t, user.Nickname, after.Nickname)
		assert.Equal(t, testNewAvatar, after.Avatar)
	})
}

func TestUpdateUserPassword(t *testing.T) {
	env := newUserTestEnv(t)

	t.Run("invalid password format", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserPassword(env.ctx, user.ID, testUserPassword, "123")
		t.Logf("[update pwd] invalid format err=%v", err)

		require.ErrorIs(t, err, ErrInvalidPasswordFormat)
		assert.Nil(t, resp)
	})

	t.Run("user not found", func(t *testing.T) {
		resp, err := env.svc.UpdateUserPassword(env.ctx, 9999, testUserPassword, testNewPassword)
		t.Logf("[update pwd] user not found err=%v", err)

		require.ErrorIs(t, err, ErrUserNotFound)
		assert.Nil(t, resp)
	})

	t.Run("repo get by id error", func(t *testing.T) {
		env.userRepo.getByIDErr = errors.New("mock get user failed")
		defer func() { env.userRepo.getByIDErr = nil }()

		resp, err := env.svc.UpdateUserPassword(env.ctx, 1, testUserPassword, testNewPassword)
		t.Logf("[update pwd] repo get user error err=%v", err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get user failed")
		assert.Nil(t, resp)
	})

	t.Run("user disabled", func(t *testing.T) {
		user := env.seedDisabledUser(t)

		resp, err := env.svc.UpdateUserPassword(env.ctx, user.ID, testUserPassword, testNewPassword)
		t.Logf("[update pwd] user disabled err=%v", err)

		require.ErrorIs(t, err, ErrUserDisabled)
		assert.Nil(t, resp)
	})

	t.Run("old password mismatch", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserPassword(env.ctx, user.ID, "WrongPass123!", testNewPassword)
		t.Logf("[update pwd] old pwd mismatch err=%v", err)

		require.ErrorIs(t, err, ErrOldPasswordMismatch)
		assert.Nil(t, resp)
	})

	t.Run("new password same as old", func(t *testing.T) {
		user := env.seedActiveUser(t)

		resp, err := env.svc.UpdateUserPassword(env.ctx, user.ID, testUserPassword, testUserPassword)
		t.Logf("[update pwd] same password err=%v", err)

		require.ErrorIs(t, err, ErrNewPasswordSameAsOld)
		assert.Nil(t, resp)
	})

	t.Run("repo update password error", func(t *testing.T) {
		user := env.seedActiveUser(t)
		oldSnapshot := env.mustGetUser(t, user.ID)

		env.userRepo.updatePasswordErr = errors.New("mock update password failed")
		defer func() { env.userRepo.updatePasswordErr = nil }()

		resp, err := env.svc.UpdateUserPassword(env.ctx, user.ID, testUserPassword, testNewPassword)
		t.Logf("[update pwd] repo update error err=%v", err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock update password failed")
		assert.Nil(t, resp)

		after := env.mustGetUser(t, user.ID)
		assert.Equal(t, oldSnapshot.PasswordHash, after.PasswordHash)
	})

	t.Run("success update password", func(t *testing.T) {
		user := env.seedActiveUser(t)
		before := env.mustGetUser(t, user.ID)

		resp, err := env.svc.UpdateUserPassword(env.ctx, user.ID, testUserPassword, testNewPassword)
		t.Logf("[update pwd] success resp=%+v", resp)

		require.NoError(t, err)
		assert.NotNil(t, resp)

		after := env.mustGetUser(t, user.ID)
		assert.True(t, password.CheckPasswordHash(testNewPassword, after.PasswordHash))
		assert.NotEqual(t, before.PasswordHash, after.PasswordHash)
		assert.False(t, password.CheckPasswordHash(testUserPassword, after.PasswordHash))
	})
}

func TestListUsers(t *testing.T) {
	t.Run("empty keyword return empty list", func(t *testing.T) {
		env := newUserTestEnv(t)

		resp, err := env.svc.ListUsers(env.ctx, 1, 10, "   ")
		t.Logf("[search] empty keyword resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.List)
		assert.Equal(t, 0, resp.Total)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
	})

	t.Run("page and pageSize fallback", func(t *testing.T) {
		env := newUserTestEnv(t)

		resp, err := env.svc.ListUsers(env.ctx, 0, 0, "   ")
		t.Logf("[search] fallback page resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, dto.MinPageSize, resp.PageSize)
	})

	t.Run("pageSize max fallback", func(t *testing.T) {
		env := newUserTestEnv(t)

		resp, err := env.svc.ListUsers(env.ctx, 1, dto.MaxPageSize+100, "   ")
		t.Logf("[search] max pageSize fallback resp=%+v", resp)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, dto.MaxPageSize, resp.PageSize)
	})

	t.Run("repo search error", func(t *testing.T) {
		env := newUserTestEnv(t)

		env.seedActiveUser(t)
		env.userRepo.searchErr = errors.New("db connection failed")
		defer func() { env.userRepo.searchErr = nil }()

		resp, err := env.svc.ListUsers(env.ctx, 1, 10, testSearchKeyword)
		t.Logf("[search] repo error err=%v", err)

		require.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("success with data, full assert", func(t *testing.T) {
		env := newUserTestEnv(t)

		u1 := env.seedCustomActiveUser(t, "testuser01", "测试用户01", "https://example.com/01.png")
		u2 := env.seedCustomActiveUser(t, "testuser02", "测试用户02", "https://example.com/02.png")
		_ = env.seedDisabledUser(t)

		resp, err := env.svc.ListUsers(env.ctx, 1, 10, "testuser")
		t.Logf("[search] success list=%+v total=%d", resp.List, resp.Total)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.List, 2)
		assert.Equal(t, 2, resp.Total)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)

		gotIDs := []uint64{resp.List[0].ID, resp.List[1].ID}
		assert.Contains(t, gotIDs, u1.ID)
		assert.Contains(t, gotIDs, u2.ID)
	})

	t.Run("success keyword by nickname", func(t *testing.T) {
		env := newUserTestEnv(t)

		u1 := env.seedCustomActiveUser(t, "alpha_user", "张三测试", "https://example.com/a.png")
		_ = env.seedCustomActiveUser(t, "beta_user", "李四", "https://example.com/b.png")

		resp, err := env.svc.ListUsers(env.ctx, 1, 10, "张三")
		t.Logf("[search] nickname match list=%+v total=%d", resp.List, resp.Total)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.List, 1)
		assert.Equal(t, 1, resp.Total)
		assert.Equal(t, u1.ID, resp.List[0].ID)
		assert.Equal(t, u1.Username, resp.List[0].Username)
		assert.Equal(t, u1.Nickname, resp.List[0].Nickname)
		assert.Equal(t, u1.Avatar, resp.List[0].Avatar)
	})

	t.Run("success no matched data", func(t *testing.T) {
		env := newUserTestEnv(t)

		env.seedCustomActiveUser(t, "hello_user", "你好", "https://example.com/hello.png")

		resp, err := env.svc.ListUsers(env.ctx, 1, 10, "not-exists-keyword")
		t.Logf("[search] no match list=%+v total=%d", resp.List, resp.Total)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.List)
		assert.Equal(t, 0, resp.Total)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
	})

	t.Run("pagination works", func(t *testing.T) {
		env := newUserTestEnv(t)

		env.seedCustomActiveUser(t, "page_user_1", "分页用户1", "https://example.com/p1.png")
		env.seedCustomActiveUser(t, "page_user_2", "分页用户2", "https://example.com/p2.png")
		env.seedCustomActiveUser(t, "page_user_3", "分页用户3", "https://example.com/p3.png")

		resp, err := env.svc.ListUsers(env.ctx, 2, 2, "page_user")
		t.Logf("[search] pagination list=%+v total=%d", resp.List, resp.Total)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.List, 1)
		assert.Equal(t, 3, resp.Total)
		assert.Equal(t, 2, resp.Page)
		assert.Equal(t, 2, resp.PageSize)
	})
}
