// 说明：
//  1. 不连接真实 MySQL，不连接真实 Redis。
//  2. Redis 使用 miniredis，测试结束自动销毁，不污染真实 Redis。
//  3. UserRepository 使用内存 mock，不污染真实数据库。
//  4. service.AuthService.Login 依赖 repository.TxManager，因此测试使用 sqlite 内存数据库构造 TxManager。
//  5. 测试覆盖：注册、登录、Me、刷新 Token、退出登录、并发登录、异常分支。
//  6. 测试中包含日志，运行 go test -v 可查看详细过程。

package test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	jwtpkg "smart-task-platform/internal/pkg/jwt"
	"smart-task-platform/internal/pkg/password"
	redispkg "smart-task-platform/internal/pkg/redis"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service"
)

const (
	constTokenType = "Bearer"
)

const (
	authTestJWTSecret      = "auth-service-unit-test-secret"
	authTestJWTIssuer      = "auth-service-unit-test-issuer"
	authTestAccessTTL      = 15 * time.Minute
	authTestRefreshTTL     = 7 * 24 * time.Hour
	authTestPlainPassword  = "Password123456"
	authTestUsername       = "zhangsan"
	authTestEmail          = "zhangsan@example.com"
	authTestNickname       = "张三"
	authTestDisabledUser   = "disableduser"
	authTestDisabledEmail  = "disabled@example.com"
	authTestConcurrentSize = 10
)

// mockAuthUserRepository 是 UserRepository 的内存 mock 实现。
// 用于替代真实数据库，避免测试污染真实用户表。
type mockAuthUserRepository struct {
	mu     sync.RWMutex
	nextID uint64

	usersByID       map[uint64]*model.User
	usersByUsername map[string]*model.User
	usersByEmail    map[string]*model.User

	// 可注入错误，用于覆盖异常分支。
	createErr          error
	getByIDErr         error
	getByAccountErr    error
	existsUsernameErr  error
	existsEmailErr     error
	updateLastLoginErr error

	updateLastLoginCall int
}

// newMockAuthUserRepository 创建内存用户仓储。
func newMockAuthUserRepository() *mockAuthUserRepository {
	return &mockAuthUserRepository{
		nextID:          1,
		usersByID:       make(map[uint64]*model.User),
		usersByUsername: make(map[string]*model.User),
		usersByEmail:    make(map[string]*model.User),
	}
}

// cloneAuthTestUser 复制用户对象，避免外部修改 mock 内部状态。
func cloneAuthTestUser(user *model.User) *model.User {
	if user == nil {
		return nil
	}

	cp := *user
	return &cp
}

// normalizeAuthTestEmail 统一邮箱格式。
func normalizeAuthTestEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// seedUser 预置测试用户。
func (m *mockAuthUserRepository) seedUser(
	t *testing.T,
	username string,
	email string,
	nickname string,
	plainPassword string,
	status string,
) *model.User {
	t.Helper()

	hashedPassword, err := password.HashPassword(plainPassword)
	require.NoError(t, err)

	user := &model.User{
		Username:     strings.TrimSpace(username),
		Email:        normalizeAuthTestEmail(email),
		PasswordHash: hashedPassword,
		Nickname:     strings.TrimSpace(nickname),
		Status:       status,
	}

	require.NoError(t, m.Create(context.Background(), &gorm.DB{}, user))

	t.Logf(
		"[mock seed user] id=%d username=%s email=%s status=%v",
		user.ID,
		user.Username,
		user.Email,
		user.Status,
	)

	return cloneAuthTestUser(user)
}

// Create 创建用户。
func (m *mockAuthUserRepository) Create(ctx context.Context, tx *gorm.DB, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}

	if user == nil {
		return errors.New("user is nil")
	}

	username := strings.TrimSpace(user.Username)
	email := normalizeAuthTestEmail(user.Email)

	if username == "" || email == "" {
		return errors.New("username or email is empty")
	}

	if _, ok := m.usersByUsername[username]; ok {
		return service.ErrUsernameExists
	}

	if _, ok := m.usersByEmail[email]; ok {
		return service.ErrEmailExists
	}

	cp := *user
	cp.Username = username
	cp.Email = email

	if cp.ID == 0 {
		cp.ID = m.nextID
		m.nextID++
	}

	now := time.Now()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = now
	}

	m.usersByID[cp.ID] = &cp
	m.usersByUsername[cp.Username] = &cp
	m.usersByEmail[cp.Email] = &cp

	user.ID = cp.ID
	user.Username = cp.Username
	user.Email = cp.Email
	user.PasswordHash = cp.PasswordHash
	user.Nickname = cp.Nickname
	user.Status = cp.Status
	user.Avatar = cp.Avatar
	user.CreatedAt = cp.CreatedAt
	user.UpdatedAt = cp.UpdatedAt

	return nil
}

// GetByID 根据 ID 查询用户。
func (m *mockAuthUserRepository) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}

	user, ok := m.usersByID[id]
	if !ok {
		return nil, service.ErrUserNotFound
	}

	return cloneAuthTestUser(user), nil
}

// GetByAccount 根据用户名或邮箱查询用户。
func (m *mockAuthUserRepository) GetByAccount(ctx context.Context, account string) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getByAccountErr != nil {
		return nil, m.getByAccountErr
	}

	account = strings.TrimSpace(account)
	email := normalizeAuthTestEmail(account)

	if user, ok := m.usersByUsername[account]; ok {
		return cloneAuthTestUser(user), nil
	}

	if user, ok := m.usersByEmail[email]; ok {
		return cloneAuthTestUser(user), nil
	}

	return nil, service.ErrUserNotFound
}

// ExistsByUsername 检查用户名是否存在。
func (m *mockAuthUserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.existsUsernameErr != nil {
		return false, m.existsUsernameErr
	}

	username = strings.TrimSpace(username)
	_, ok := m.usersByUsername[username]
	return ok, nil
}

// ExistsByEmail 检查邮箱是否存在。
func (m *mockAuthUserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.existsEmailErr != nil {
		return false, m.existsEmailErr
	}

	email = normalizeAuthTestEmail(email)
	_, ok := m.usersByEmail[email]
	return ok, nil
}

// UpdateLastLoginAtWithTx 更新最后登录时间。
// 注意：这里不会写真实数据库，只更新 mock 内存状态。
func (m *mockAuthUserRepository) UpdateLastLoginAtWithTx(
	ctx context.Context,
	tx *gorm.DB,
	userID uint64,
	loginAt time.Time,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.updateLastLoginCall++

	if m.updateLastLoginErr != nil {
		return m.updateLastLoginErr
	}

	user, ok := m.usersByID[userID]
	if !ok {
		return service.ErrUserNotFound
	}

	user.LastLoginAt = &loginAt
	user.UpdatedAt = loginAt

	return nil
}

// authServiceMockTestEnv 是 AuthService 的隔离测试环境。
type authServiceMockTestEnv struct {
	ctx context.Context

	mr  *miniredis.Miniredis
	rdb *goredis.Client

	db        *gorm.DB
	txMgr     *repository.TxManager
	userRepo  *mockAuthUserRepository
	authStore *redispkg.RedisAuthStore
	jwtMgr    *jwtpkg.Manager
	svc       *service.AuthService
}

// newAuthServiceMockTestEnv 创建隔离测试环境。
func newAuthServiceMockTestEnv(t *testing.T) *authServiceMockTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	oldLogger := zap.L()
	zap.ReplaceGlobals(logger)

	ctx := context.Background()

	// 启动 miniredis，避免污染真实 Redis。
	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
		DB:   0,
	})
	require.NoError(t, rdb.Ping(ctx).Err())

	// sqlite 内存数据库，仅用于构造 TxManager，不污染真实数据库。
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	txMgr := repository.NewTxManager(db)
	require.NotNil(t, txMgr)

	userRepo := newMockAuthUserRepository()

	authStore := redispkg.NewRedisAuthStore(rdb)

	jwtMgr := jwtpkg.NewManager(
		authTestJWTSecret,
		authTestJWTIssuer,
		authTestAccessTTL,
		authTestRefreshTTL,
	)

	svc := service.NewAuthService(
		txMgr,
		userRepo,
		authStore,
		jwtMgr,
	)

	t.Cleanup(func() {
		zap.ReplaceGlobals(oldLogger)

		_ = logger.Sync()
		_ = rdb.Close()
		mr.Close()

		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	t.Logf("[newAuthServiceMockTestEnv] miniredis_addr=%s", mr.Addr())

	return &authServiceMockTestEnv{
		ctx:       ctx,
		mr:        mr,
		rdb:       rdb,
		db:        db,
		txMgr:     txMgr,
		userRepo:  userRepo,
		authStore: authStore,
		jwtMgr:    jwtMgr,
		svc:       svc,
	}
}

// redisKeysForAuthTest 打印当前 Redis 所有 key。
func redisKeysForAuthTest(t *testing.T, env *authServiceMockTestEnv) []string {
	t.Helper()

	keys, err := env.rdb.Keys(env.ctx, "*").Result()
	require.NoError(t, err)

	sort.Strings(keys)
	t.Logf("[redis keys] keys=%v", keys)

	return keys
}

// redisHashForAuthTest 打印指定 Redis hash。
func redisHashForAuthTest(t *testing.T, env *authServiceMockTestEnv, key string) map[string]string {
	t.Helper()

	value, err := env.rdb.HGetAll(env.ctx, key).Result()
	require.NoError(t, err)

	t.Logf("[redis hash] key=%s value=%v", key, value)

	return value
}

// redisTTLPositiveForAuthTest 断言 key TTL 为正。
func redisTTLPositiveForAuthTest(t *testing.T, env *authServiceMockTestEnv, key string) {
	t.Helper()

	ttl, err := env.rdb.TTL(env.ctx, key).Result()
	require.NoError(t, err)

	t.Logf("[redis ttl] key=%s ttl=%s", key, ttl)

	assert.Greater(t, ttl, time.Duration(0))
}

// findRedisKeyByPatterns 根据多个 pattern 查找 Redis key。
func findRedisKeyByPatterns(t *testing.T, env *authServiceMockTestEnv, patterns ...string) string {
	t.Helper()

	for _, pattern := range patterns {
		keys, err := env.rdb.Keys(env.ctx, pattern).Result()
		require.NoError(t, err)

		sort.Strings(keys)

		if len(keys) > 0 {
			t.Logf("[findRedisKeyByPatterns] pattern=%s keys=%v", pattern, keys)
			return keys[0]
		}
	}

	keys := redisKeysForAuthTest(t, env)
	require.Failf(t, "redis key not found", "patterns=%v all_keys=%v", patterns, keys)

	return ""
}

// countRedisKeysByPattern 统计指定 pattern 的 Redis key 数量。
func countRedisKeysByPattern(t *testing.T, env *authServiceMockTestEnv, pattern string) int {
	t.Helper()

	keys, err := env.rdb.Keys(env.ctx, pattern).Result()
	require.NoError(t, err)

	sort.Strings(keys)
	t.Logf("[countRedisKeysByPattern] pattern=%s keys=%v", pattern, keys)

	return len(keys)
}

// findSessionKeyForAuthTest 查找用户 session key。
func findSessionKeyForAuthTest(t *testing.T, env *authServiceMockTestEnv, userID uint64) string {
	t.Helper()

	return findRedisKeyByPatterns(
		t,
		env,
		fmt.Sprintf("*session*%d*", userID),
		fmt.Sprintf("*auth*session*%d*", userID),
		fmt.Sprintf("*user*%d*session*", userID),
	)
}

// findRefreshKeyForAuthTest 查找 refresh key。
func findRefreshKeyForAuthTest(t *testing.T, env *authServiceMockTestEnv, refreshJTI string) string {
	t.Helper()

	return findRedisKeyByPatterns(
		t,
		env,
		fmt.Sprintf("*refresh*%s*", refreshJTI),
		fmt.Sprintf("*jti*%s*", refreshJTI),
	)
}

// findBlacklistKeyForAuthTest 查找 access 黑名单 key。
func findBlacklistKeyForAuthTest(t *testing.T, env *authServiceMockTestEnv, accessJTI string) string {
	t.Helper()

	return findRedisKeyByPatterns(
		t,
		env,
		fmt.Sprintf("*blacklist*%s*", accessJTI),
		fmt.Sprintf("*access*%s*", accessJTI),
	)
}

// refreshKeyShouldNotExistForAuthTest 断言 refresh key 不存在。
func refreshKeyShouldNotExistForAuthTest(t *testing.T, env *authServiceMockTestEnv, refreshJTI string) {
	t.Helper()

	patterns := []string{
		fmt.Sprintf("*refresh*%s*", refreshJTI),
		fmt.Sprintf("*jti*%s*", refreshJTI),
	}

	for _, pattern := range patterns {
		keys, err := env.rdb.Keys(env.ctx, pattern).Result()
		require.NoError(t, err)

		sort.Strings(keys)
		t.Logf("[refreshKeyShouldNotExistForAuthTest] pattern=%s keys=%v", pattern, keys)

		assert.Empty(t, keys)
	}
}

// TestAuthService_Register_Success 测试注册成功。
func TestAuthService_Register_Success(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	req := &dto.RegisterReq{
		Username: "  lisi123  ",
		Email:    "  LISI123@Example.COM  ",
		Password: authTestPlainPassword,
		Nickname: "  李四  ",
	}

	resp, err := env.svc.Register(env.ctx, req)

	t.Logf("[Register Success] req=%+v resp=%+v err=%v", req, resp, err)

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotZero(t, resp.ID)
	assert.Equal(t, "lisi123", resp.Username)
	assert.Equal(t, "lisi123@example.com", resp.Email)
	assert.Equal(t, "李四", resp.Nickname)

	user, err := env.userRepo.GetByAccount(env.ctx, "lisi123")
	require.NoError(t, err)
	require.NotNil(t, user)

	assert.Equal(t, resp.ID, user.ID)
	assert.Equal(t, resp.Username, user.Username)
	assert.Equal(t, resp.Email, user.Email)
	assert.Equal(t, model.UserStatusActive, user.Status)
	assert.NotEmpty(t, user.PasswordHash)
	assert.NotEqual(t, authTestPlainPassword, user.PasswordHash)
	assert.True(t, password.CheckPasswordHash(authTestPlainPassword, user.PasswordHash))
}

// TestAuthService_Register_InvalidCases 测试注册参数非法场景。
func TestAuthService_Register_InvalidCases(t *testing.T) {
	tests := []struct {
		name    string
		req     *dto.RegisterReq
		wantErr error
	}{
		{
			name: "用户名格式非法",
			req: &dto.RegisterReq{
				Username: "ab",
				Email:    "valid@example.com",
				Password: authTestPlainPassword,
				Nickname: "昵称",
			},
			wantErr: service.ErrInvalidUsernameFormat,
		},
		{
			name: "邮箱格式非法",
			req: &dto.RegisterReq{
				Username: "validuser",
				Email:    "invalid-email",
				Password: authTestPlainPassword,
				Nickname: "昵称",
			},
			wantErr: service.ErrInvalidEmailFormat,
		},
		{
			name: "密码格式非法",
			req: &dto.RegisterReq{
				Username: "validuser",
				Email:    "valid@example.com",
				Password: "123",
				Nickname: "昵称",
			},
			wantErr: service.ErrInvalidPasswordFormat,
		},
		{
			name: "昵称格式非法",
			req: &dto.RegisterReq{
				Username: "validuser",
				Email:    "valid@example.com",
				Password: authTestPlainPassword,
				Nickname: strings.Repeat("很", 100),
			},
			wantErr: service.ErrInvalidNicknameFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newAuthServiceMockTestEnv(t)

			resp, err := env.svc.Register(env.ctx, tt.req)

			t.Logf("[Register InvalidCases] name=%s resp=%+v err=%v", tt.name, resp, err)

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestAuthService_Register_DuplicateCases 测试重复注册。
func TestAuthService_Register_DuplicateCases(t *testing.T) {
	tests := []struct {
		name    string
		req     *dto.RegisterReq
		wantErr error
	}{
		{
			name: "用户名已存在",
			req: &dto.RegisterReq{
				Username: authTestUsername,
				Email:    "new@example.com",
				Password: authTestPlainPassword,
				Nickname: "new",
			},
			wantErr: service.ErrUsernameExists,
		},
		{
			name: "邮箱已存在",
			req: &dto.RegisterReq{
				Username: "newuser",
				Email:    strings.ToUpper(authTestEmail),
				Password: authTestPlainPassword,
				Nickname: "new",
			},
			wantErr: service.ErrEmailExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newAuthServiceMockTestEnv(t)
			env.userRepo.seedUser(
				t,
				authTestUsername,
				authTestEmail,
				authTestNickname,
				authTestPlainPassword,
				model.UserStatusActive,
			)

			resp, err := env.svc.Register(env.ctx, tt.req)

			t.Logf("[Register DuplicateCases] name=%s resp=%+v err=%v", tt.name, resp, err)

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestAuthService_Login_Success 测试用户名登录成功。
func TestAuthService_Login_Success(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	user := env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	resp, err := env.svc.Login(env.ctx, &dto.LoginReq{
		Account:  "  " + authTestUsername + "  ",
		Password: authTestPlainPassword,
	})

	t.Logf("[Login Success] resp=%+v err=%v", resp, err)

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, constTokenType, resp.TokenType)
	assert.Equal(t, int64(authTestAccessTTL.Seconds()), resp.ExpiresIn)
	assert.Equal(t, user.ID, resp.User.ID)
	assert.Equal(t, user.Username, resp.User.Username)
	assert.Equal(t, user.Nickname, resp.User.Nickname)

	assert.Equal(t, 1, env.userRepo.updateLastLoginCall)

	accessClaims, err := env.jwtMgr.ParseToken(resp.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, accessClaims)

	refreshClaims, err := env.jwtMgr.ParseToken(resp.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, refreshClaims)

	t.Logf("[Login AccessClaims] claims=%+v", accessClaims)
	t.Logf("[Login RefreshClaims] claims=%+v", refreshClaims)

	assert.Equal(t, user.ID, accessClaims.UserID)
	assert.Equal(t, user.Username, accessClaims.Username)
	assert.Equal(t, "access", accessClaims.TokenType)
	assert.NotEmpty(t, accessClaims.SessionID)
	assert.NotEmpty(t, accessClaims.ID)

	assert.Equal(t, user.ID, refreshClaims.UserID)
	assert.Equal(t, user.Username, refreshClaims.Username)
	assert.Equal(t, "refresh", refreshClaims.TokenType)
	assert.Equal(t, accessClaims.SessionID, refreshClaims.SessionID)
	assert.NotEmpty(t, refreshClaims.ID)
	assert.NotEqual(t, accessClaims.ID, refreshClaims.ID)

	sessionKey := findSessionKeyForAuthTest(t, env, user.ID)
	sessionValue := redisHashForAuthTest(t, env, sessionKey)
	require.NotEmpty(t, sessionValue)
	redisTTLPositiveForAuthTest(t, env, sessionKey)

	refreshKey := findRefreshKeyForAuthTest(t, env, refreshClaims.ID)
	refreshValue := redisHashForAuthTest(t, env, refreshKey)
	require.NotEmpty(t, refreshValue)
	redisTTLPositiveForAuthTest(t, env, refreshKey)

	redisKeysForAuthTest(t, env)
}

// TestAuthService_Login_ByUpperEmail_Success 测试邮箱大小写登录成功。
func TestAuthService_Login_ByUpperEmail_Success(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	user := env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	resp, err := env.svc.Login(env.ctx, &dto.LoginReq{
		Account:  strings.ToUpper(authTestEmail),
		Password: authTestPlainPassword,
	})

	t.Logf("[Login ByUpperEmail] resp=%+v err=%v", resp, err)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, user.ID, resp.User.ID)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
}

// ////////////////////////////////////////////////////////////////////////////////
// TestAuthService_Login_FailedCases 测试登录失败场景。
func TestAuthService_Login_FailedCases(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(env *authServiceMockTestEnv)
		req     *dto.LoginReq
		wantErr error
	}{
		{
			name: "账户格式非法",
			req: &dto.LoginReq{
				Account:  "@@@",
				Password: authTestPlainPassword,
			},
			wantErr: service.ErrInvalidAccountFormat,
		},
		{
			name: "用户不存在",
			req: &dto.LoginReq{
				Account:  "nouser",
				Password: authTestPlainPassword,
			},
			wantErr: service.ErrUserNotFound,
		},
		{
			name: "密码错误",
			prepare: func(env *authServiceMockTestEnv) {
				env.userRepo.seedUser(
					t,
					authTestUsername,
					authTestEmail,
					authTestNickname,
					authTestPlainPassword,
					model.UserStatusActive,
				)
			},
			req: &dto.LoginReq{
				Account:  authTestUsername,
				Password: "WrongPassword123",
			},
			wantErr: service.ErrPasswordMismatch,
		},
		{
			name: "用户被禁用",
			prepare: func(env *authServiceMockTestEnv) {
				env.userRepo.seedUser(
					t,
					authTestDisabledUser,
					authTestDisabledEmail,
					authTestNickname,
					authTestPlainPassword,
					model.UserStatusDisabled,
				)
			},
			req: &dto.LoginReq{
				Account:  authTestDisabledUser,
				Password: authTestPlainPassword,
			},
			wantErr: service.ErrUserDisabled,
		},
		{
			name: "更新最后登录时间失败",
			prepare: func(env *authServiceMockTestEnv) {
				env.userRepo.seedUser(
					t,
					authTestUsername,
					authTestEmail,
					authTestNickname,
					authTestPlainPassword,
					model.UserStatusActive,
				)
				env.userRepo.updateLastLoginErr = service.ErrInternal
			},
			req: &dto.LoginReq{
				Account:  authTestUsername,
				Password: authTestPlainPassword,
			},
			wantErr: service.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newAuthServiceMockTestEnv(t)
			if tt.prepare != nil {
				tt.prepare(env)
			}

			resp, err := env.svc.Login(env.ctx, tt.req)

			t.Logf("[Login FailedCases] name=%s resp=%+v err=%v", tt.name, resp, err)

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.ErrorIs(t, err, tt.wantErr)

			// 登录失败不应该写入 Redis session / refresh。
			keys := redisKeysForAuthTest(t, env)
			assert.Empty(t, keys)
		})
	}
}

// TestAuthService_Me_Success 测试获取当前用户成功。
func TestAuthService_Me_Success(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	user := env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	resp, err := env.svc.Me(env.ctx, user.ID)

	t.Logf("[Me Success] resp=%+v err=%v", resp, err)

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, user.ID, resp.ID)
	assert.Equal(t, user.Username, resp.Username)
	assert.Equal(t, user.Email, resp.Email)
	assert.Equal(t, user.Nickname, resp.Nickname)
}

// TestAuthService_Me_FailedCases 测试获取当前用户失败场景。
func TestAuthService_Me_FailedCases(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(env *authServiceMockTestEnv) uint64
		userID  uint64
		wantErr error
	}{
		{
			name:    "用户不存在",
			userID:  999,
			wantErr: service.ErrUserNotFound,
		},
		{
			name: "用户被禁用",
			prepare: func(env *authServiceMockTestEnv) uint64 {
				user := env.userRepo.seedUser(
					t,
					authTestDisabledUser,
					authTestDisabledEmail,
					authTestNickname,
					authTestPlainPassword,
					model.UserStatusDisabled,
				)
				return user.ID
			},
			wantErr: service.ErrUserDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newAuthServiceMockTestEnv(t)

			userID := tt.userID
			if tt.prepare != nil {
				userID = tt.prepare(env)
			}

			resp, err := env.svc.Me(env.ctx, userID)

			t.Logf("[Me FailedCases] name=%s resp=%+v err=%v", tt.name, resp, err)

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestAuthService_RefreshToken_Success 测试刷新 Token 成功。
func TestAuthService_RefreshToken_Success(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	user := env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	loginResp, err := env.svc.Login(env.ctx, &dto.LoginReq{
		Account:  authTestUsername,
		Password: authTestPlainPassword,
	})
	require.NoError(t, err)
	require.NotNil(t, loginResp)

	oldRefreshClaims, err := env.jwtMgr.ParseToken(loginResp.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, oldRefreshClaims)

	resp, err := env.svc.RefreshToken(env.ctx, &dto.RefreshTokenReq{
		RefreshToken: loginResp.RefreshToken,
	})

	t.Logf("[RefreshToken Success] resp=%+v err=%v", resp, err)

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, constTokenType, resp.TokenType)
	assert.Equal(t, int64(authTestAccessTTL.Seconds()), resp.ExpiresIn)

	newAccessClaims, err := env.jwtMgr.ParseToken(resp.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, newAccessClaims)

	newRefreshClaims, err := env.jwtMgr.ParseToken(resp.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, newRefreshClaims)

	t.Logf("[RefreshToken OldRefreshClaims] claims=%+v", oldRefreshClaims)
	t.Logf("[RefreshToken NewAccessClaims] claims=%+v", newAccessClaims)
	t.Logf("[RefreshToken NewRefreshClaims] claims=%+v", newRefreshClaims)

	assert.Equal(t, user.ID, newAccessClaims.UserID)
	assert.Equal(t, user.ID, newRefreshClaims.UserID)

	assert.Equal(t, "access", newAccessClaims.TokenType)
	assert.Equal(t, "refresh", newRefreshClaims.TokenType)

	assert.Equal(t, oldRefreshClaims.SessionID, newAccessClaims.SessionID)
	assert.Equal(t, oldRefreshClaims.SessionID, newRefreshClaims.SessionID)

	assert.NotEqual(t, oldRefreshClaims.ID, newRefreshClaims.ID)
	assert.NotEqual(t, newAccessClaims.ID, newRefreshClaims.ID)

	// 旧 refresh 应被删除。
	refreshKeyShouldNotExistForAuthTest(t, env, oldRefreshClaims.ID)

	// 新 refresh 应存在。
	newRefreshKey := findRefreshKeyForAuthTest(t, env, newRefreshClaims.ID)
	newRefreshValue := redisHashForAuthTest(t, env, newRefreshKey)
	require.NotEmpty(t, newRefreshValue)
	redisTTLPositiveForAuthTest(t, env, newRefreshKey)

	// 单设备会话下，最终只应该有一个 refresh。
	assert.Equal(t, 1, countRedisKeysByPattern(t, env, "*refresh*"))

	redisKeysForAuthTest(t, env)
}

// TestAuthService_RefreshToken_FailedCases 测试刷新 Token 失败场景。
func TestAuthService_RefreshToken_FailedCases(t *testing.T) {
	t.Run("空 refresh token", func(t *testing.T) {
		env := newAuthServiceMockTestEnv(t)

		resp, err := env.svc.RefreshToken(env.ctx, &dto.RefreshTokenReq{
			RefreshToken: "   ",
		})

		t.Logf("[RefreshToken Empty] resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})

	t.Run("非法 refresh token", func(t *testing.T) {
		env := newAuthServiceMockTestEnv(t)

		resp, err := env.svc.RefreshToken(env.ctx, &dto.RefreshTokenReq{
			RefreshToken: "invalid.token.value",
		})

		t.Logf("[RefreshToken Invalid] resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})

	t.Run("使用 access token 调 refresh 接口", func(t *testing.T) {
		env := newAuthServiceMockTestEnv(t)

		env.userRepo.seedUser(
			t,
			authTestUsername,
			authTestEmail,
			authTestNickname,
			authTestPlainPassword,
			model.UserStatusActive,
		)

		loginResp, err := env.svc.Login(env.ctx, &dto.LoginReq{
			Account:  authTestUsername,
			Password: authTestPlainPassword,
		})
		require.NoError(t, err)
		require.NotNil(t, loginResp)

		resp, err := env.svc.RefreshToken(env.ctx, &dto.RefreshTokenReq{
			RefreshToken: loginResp.AccessToken,
		})

		t.Logf("[RefreshToken UseAccessToken] resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidToken)
	})

	t.Run("旧 refresh token 重复使用失败", func(t *testing.T) {
		env := newAuthServiceMockTestEnv(t)

		env.userRepo.seedUser(
			t,
			authTestUsername,
			authTestEmail,
			authTestNickname,
			authTestPlainPassword,
			model.UserStatusActive,
		)

		loginResp, err := env.svc.Login(env.ctx, &dto.LoginReq{
			Account:  authTestUsername,
			Password: authTestPlainPassword,
		})
		require.NoError(t, err)
		require.NotNil(t, loginResp)

		firstResp, err := env.svc.RefreshToken(env.ctx, &dto.RefreshTokenReq{
			RefreshToken: loginResp.RefreshToken,
		})
		require.NoError(t, err)
		require.NotNil(t, firstResp)

		secondResp, err := env.svc.RefreshToken(env.ctx, &dto.RefreshTokenReq{
			RefreshToken: loginResp.RefreshToken,
		})

		t.Logf("[RefreshToken ReuseOld] second_resp=%+v err=%v", secondResp, err)

		require.Error(t, err)
		assert.Nil(t, secondResp)
	})
}

// TestAuthService_Logout_Success 测试退出登录成功。
func TestAuthService_Logout_Success(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	user := env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	loginResp, err := env.svc.Login(env.ctx, &dto.LoginReq{
		Account:  authTestUsername,
		Password: authTestPlainPassword,
	})
	require.NoError(t, err)
	require.NotNil(t, loginResp)

	accessClaims, err := env.jwtMgr.ParseToken(loginResp.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, accessClaims)

	refreshClaims, err := env.jwtMgr.ParseToken(loginResp.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, refreshClaims)

	resp, err := env.svc.Logout(env.ctx, accessClaims)

	t.Logf("[Logout Success] resp=%+v err=%v", resp, err)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Logout)

	// session 应被删除。
	sessionKeys, err := env.rdb.Keys(env.ctx, fmt.Sprintf("*session*%d*", user.ID)).Result()
	require.NoError(t, err)
	sort.Strings(sessionKeys)
	t.Logf("[Logout session keys] keys=%v", sessionKeys)
	assert.Empty(t, sessionKeys)

	// refresh 应被删除。
	refreshKeyShouldNotExistForAuthTest(t, env, refreshClaims.ID)

	// access token jti 应被加入黑名单。
	blacklistKey := findBlacklistKeyForAuthTest(t, env, accessClaims.ID)
	exists, err := env.rdb.Exists(env.ctx, blacklistKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
	redisTTLPositiveForAuthTest(t, env, blacklistKey)

	redisKeysForAuthTest(t, env)
}

// TestAuthService_Logout_Idempotent 测试重复退出。
// 如果此测试失败，说明当前 LogoutSession 对重复退出不是幂等的；
// 建议在 service.AuthService.Logout 或 RedisAuthStore.LogoutSession 中对会话不存在做幂等处理。
func TestAuthService_Logout_Idempotent(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	loginResp, err := env.svc.Login(env.ctx, &dto.LoginReq{
		Account:  authTestUsername,
		Password: authTestPlainPassword,
	})
	require.NoError(t, err)
	require.NotNil(t, loginResp)

	claims, err := env.jwtMgr.ParseToken(loginResp.AccessToken)
	require.NoError(t, err)
	require.NotNil(t, claims)

	firstResp, firstErr := env.svc.Logout(env.ctx, claims)
	t.Logf("[Logout Idempotent First] resp=%+v err=%v", firstResp, firstErr)

	require.NoError(t, firstErr)
	require.NotNil(t, firstResp)
	assert.True(t, firstResp.Logout)

	secondResp, secondErr := env.svc.Logout(env.ctx, claims)
	t.Logf("[Logout Idempotent Second] resp=%+v err=%v", secondResp, secondErr)

	require.NoError(t, secondErr)
	require.NotNil(t, secondResp)
	assert.True(t, secondResp.Logout)

	redisKeysForAuthTest(t, env)
}

// TestAuthService_ConcurrentLogin 测试同一用户并发登录最终一致性。
func TestAuthService_ConcurrentLogin(t *testing.T) {
	env := newAuthServiceMockTestEnv(t)

	user := env.userRepo.seedUser(
		t,
		authTestUsername,
		authTestEmail,
		authTestNickname,
		authTestPlainPassword,
		model.UserStatusActive,
	)

	type loginResult struct {
		index int
		resp  *dto.LoginResp
		err   error
	}

	var wg sync.WaitGroup
	resultCh := make(chan loginResult, authTestConcurrentSize)

	for i := 0; i < authTestConcurrentSize; i++ {
		i := i

		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, err := env.svc.Login(env.ctx, &dto.LoginReq{
				Account:  authTestUsername,
				Password: authTestPlainPassword,
			})

			t.Logf("[ConcurrentLogin] i=%d resp_nil=%v err=%v", i, resp == nil, err)

			resultCh <- loginResult{
				index: i,
				resp:  resp,
				err:   err,
			}
		}()
	}

	wg.Wait()
	close(resultCh)

	successCount := 0

	for result := range resultCh {
		if result.err != nil {
			t.Logf("[ConcurrentLogin Error] index=%d err=%v", result.index, result.err)
			continue
		}

		successCount++

		require.NotNil(t, result.resp)
		assert.NotEmpty(t, result.resp.AccessToken)
		assert.NotEmpty(t, result.resp.RefreshToken)
		assert.Equal(t, constTokenType, result.resp.TokenType)
	}

	t.Logf(
		"[ConcurrentLogin Summary] success_count=%d total=%d",
		successCount,
		authTestConcurrentSize,
	)

	// service.AuthService.Login 内部使用 RetryRedisTx，通常应该全部成功；
	// 但最终一致性测试只强制要求至少一个成功。
	require.GreaterOrEqual(t, successCount, 1)

	sessionKey := findSessionKeyForAuthTest(t, env, user.ID)
	sessionValue := redisHashForAuthTest(t, env, sessionKey)
	require.NotEmpty(t, sessionValue)
	redisTTLPositiveForAuthTest(t, env, sessionKey)

	// 单设备登录最终只应保留一个 refresh。
	assert.Equal(t, 1, countRedisKeysByPattern(t, env, "*refresh*"))

	redisKeysForAuthTest(t, env)
}
