// internal/service/project_member_soft_delete_add_test.go
package test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"smart-task-platform/internal/model"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	pmTestProjectID uint64 = 1001

	pmTestOwnerID  uint64 = 2001
	pmTestAdminID  uint64 = 2002
	pmTestMemberID uint64 = 2003
	pmTestOtherID  uint64 = 2004
)

var errPMRepoMock = errors.New("project member mock repository error")

// =========================
// Test Env：补充 taskRepo
// =========================

type pmProjectMemberServiceTestEnv struct {
	ctx context.Context
	db  *gorm.DB

	txMgr       *repository.TxManager
	userRepo    *pmMockUserRepo
	projectRepo *pmMockProjectRepo
	memberRepo  *pmMockProjectMemberRepo
	taskRepo    *pmMockTaskRepo

	svc *service.ProjectMemberService
}

func pmNewProjectMemberServiceTestEnv(t *testing.T) *pmProjectMemberServiceTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	zap.ReplaceGlobals(logger)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	ctx := context.Background()

	userRepo := pmNewMockUserRepo()
	projectRepo := pmNewMockProjectRepo()
	memberRepo := pmNewMockProjectMemberRepo()
	taskRepo := pmNewMockTaskRepo()
	txMgr := repository.NewTxManager(db)

	env := &pmProjectMemberServiceTestEnv{
		ctx:         ctx,
		db:          db,
		txMgr:       txMgr,
		userRepo:    userRepo,
		projectRepo: projectRepo,
		memberRepo:  memberRepo,
		taskRepo:    taskRepo,
		svc: service.NewProjectMemberService(
			txMgr,
			userRepo,
			projectRepo,
			memberRepo,
			taskRepo,
		),
	}

	env.pmSeedBaseData(t)

	return env
}

func (e *pmProjectMemberServiceTestEnv) pmSeedBaseData(t *testing.T) {
	t.Helper()

	e.userRepo.users[pmTestOwnerID] = &model.User{
		ID:       pmTestOwnerID,
		Username: "pm_test_owner",
		Nickname: "项目所有者",
		Avatar:   "https://example.com/owner.png",
	}
	e.userRepo.users[pmTestAdminID] = &model.User{
		ID:       pmTestAdminID,
		Username: "pm_test_admin",
		Nickname: "项目管理员",
		Avatar:   "https://example.com/admin.png",
	}
	e.userRepo.users[pmTestMemberID] = &model.User{
		ID:       pmTestMemberID,
		Username: "pm_test_member",
		Nickname: "普通成员",
		Avatar:   "https://example.com/member.png",
	}
	e.userRepo.users[pmTestOtherID] = &model.User{
		ID:       pmTestOtherID,
		Username: "pm_test_other",
		Nickname: "待添加用户",
		Avatar:   "https://example.com/other.png",
	}

	e.projectRepo.projects[pmTestProjectID] = true

	now := time.Now()

	e.memberRepo.members[pmProjectMemberKey(pmTestProjectID, pmTestOwnerID)] = &model.ProjectMember{
		ID:        1,
		ProjectID: pmTestProjectID,
		UserID:    pmTestOwnerID,
		Role:      model.ProjectMemberRoleOwner,
		JoinedAt:  now.Add(-72 * time.Hour),
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}

	e.memberRepo.members[pmProjectMemberKey(pmTestProjectID, pmTestAdminID)] = &model.ProjectMember{
		ID:        2,
		ProjectID: pmTestProjectID,
		UserID:    pmTestAdminID,
		Role:      model.ProjectMemberRoleAdmin,
		InvitedBy: pmUint64Ptr(pmTestOwnerID),
		JoinedAt:  now.Add(-48 * time.Hour),
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-48 * time.Hour),
	}

	e.memberRepo.members[pmProjectMemberKey(pmTestProjectID, pmTestMemberID)] = &model.ProjectMember{
		ID:        3,
		ProjectID: pmTestProjectID,
		UserID:    pmTestMemberID,
		Role:      model.ProjectMemberRoleMember,
		InvitedBy: pmUint64Ptr(pmTestOwnerID),
		JoinedAt:  now.Add(-24 * time.Hour),
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
	}
}

// =========================
// Soft Delete 测试中补充 taskRepo 断言
// =========================

func TestProjectMemberServiceSoftDeleteProjectMember(t *testing.T) {
	t.Run("success owner soft delete member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[soft delete member] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.softDeleteCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
		assert.Equal(t, pmTestProjectID, env.memberRepo.lastSoftDeleteProjectID)
		assert.Equal(t, pmTestMemberID, env.memberRepo.lastSoftDeleteUserID)

		// 软删除成员后，需要清空该成员在当前项目下负责的任务
		assert.True(t, env.taskRepo.clearCalled)
		assert.False(t, env.taskRepo.lastTxIsNil)
		assert.Equal(t, pmTestProjectID, env.taskRepo.lastProjectID)
		assert.Equal(t, pmTestMemberID, env.taskRepo.lastAssigneeID)
		assert.False(t, env.taskRepo.lastUpdatedAt.IsZero())

		// 普通查询应该查不到软删除成员
		_, err = env.memberRepo.GetProjectMemberByProjectIDAndUserID(
			env.ctx,
			pmTestProjectID,
			pmTestMemberID,
		)
		require.ErrorIs(t, err, repository.ErrProjectMemberNotFound)

		// Unscoped 查询应该可以查到软删除成员
		pm, err := env.memberRepo.GetProjectMemberByProjectIDAndUserIDUnscoped(
			env.ctx,
			pmTestProjectID,
			pmTestMemberID,
		)
		require.NoError(t, err)
		require.NotNil(t, pm)

		assert.Equal(t, pmTestProjectID, pm.ProjectID)
		assert.Equal(t, pmTestMemberID, pm.UserID)
		assert.True(t, pm.DeletedAt.Valid)
		assert.False(t, pm.DeletedAt.Time.IsZero())
		assert.False(t, pm.UpdatedAt.IsZero())
	})

	t.Run("failed soft delete repository error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.softDeleteErr = errPMRepoMock

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[soft delete member] repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)

		assert.True(t, env.memberRepo.softDeleteCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)

		// 软删除失败，不应该继续清空任务负责人
		assert.False(t, env.taskRepo.clearCalled)

		// 删除失败时成员仍然应该是有效成员
		pm, getErr := env.memberRepo.GetProjectMemberByProjectIDAndUserID(
			env.ctx,
			pmTestProjectID,
			pmTestMemberID,
		)
		require.NoError(t, getErr)
		require.NotNil(t, pm)
		assert.False(t, pm.DeletedAt.Valid)
	})

	t.Run("failed clear task assignee repository error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.taskRepo.clearErr = errPMRepoMock

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[soft delete member] clear task assignee repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)

		assert.True(t, env.memberRepo.softDeleteCalled)
		assert.True(t, env.taskRepo.clearCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
		assert.False(t, env.taskRepo.lastTxIsNil)
		assert.Equal(t, pmTestProjectID, env.taskRepo.lastProjectID)
		assert.Equal(t, pmTestMemberID, env.taskRepo.lastAssigneeID)
		assert.False(t, env.taskRepo.lastUpdatedAt.IsZero())
	})

	t.Run("failed removed user not project member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestOtherID,
		})

		t.Logf("[soft delete member] removed user not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.softDeleteCalled)
		assert.False(t, env.taskRepo.clearCalled)
	})

	t.Run("failed admin remove owner forbidden", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestAdminID,
			RemovedUserID: pmTestOwnerID,
		})

		t.Logf("[soft delete member] admin remove owner forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.softDeleteCalled)
		assert.False(t, env.taskRepo.clearCalled)
	})
}

// =========================
// Tests: Add Member
// =========================

func TestProjectMemberServiceAddProjectMember(t *testing.T) {
	t.Run("success create new member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestOtherID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add member] create new member resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.existsCalled)
		assert.True(t, env.memberRepo.getUnscopedCalled)
		assert.True(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.memberRepo.restoreCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)

		require.NotNil(t, env.memberRepo.lastCreatedMember)
		assert.Equal(t, pmTestProjectID, env.memberRepo.lastCreatedMember.ProjectID)
		assert.Equal(t, pmTestOtherID, env.memberRepo.lastCreatedMember.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, env.memberRepo.lastCreatedMember.Role)
		assert.False(t, env.memberRepo.lastCreatedMember.JoinedAt.IsZero())
		assert.False(t, env.memberRepo.lastCreatedMember.DeletedAt.Valid)

		pm, getErr := env.memberRepo.GetProjectMemberByProjectIDAndUserID(
			env.ctx,
			pmTestProjectID,
			pmTestOtherID,
		)
		require.NoError(t, getErr)
		require.NotNil(t, pm)

		assert.Equal(t, pmTestProjectID, pm.ProjectID)
		assert.Equal(t, pmTestOtherID, pm.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, pm.Role)
		assert.False(t, pm.DeletedAt.Valid)

		assert.Equal(t, pmTestProjectID, resp.ProjectID)
		assert.Equal(t, pmTestOtherID, resp.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, resp.Role)
		assert.False(t, resp.JoinedAt.IsZero())
	})

	t.Run("success restore soft deleted member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		deletedAt := time.Now().Add(-2 * time.Hour)
		oldJoinedAt := time.Now().Add(-48 * time.Hour)

		env.memberRepo.members[pmProjectMemberKey(pmTestProjectID, pmTestOtherID)] = &model.ProjectMember{
			ID:        999,
			ProjectID: pmTestProjectID,
			UserID:    pmTestOtherID,
			Role:      model.ProjectMemberRoleMember,
			InvitedBy: pmUint64Ptr(pmTestAdminID),
			JoinedAt:  oldJoinedAt,
			CreatedAt: oldJoinedAt,
			UpdatedAt: deletedAt,
			DeletedAt: gorm.DeletedAt{
				Time:  deletedAt,
				Valid: true,
			},
		}

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestOtherID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add member] restore soft deleted member resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.existsCalled)
		assert.True(t, env.memberRepo.getUnscopedCalled)
		assert.True(t, env.memberRepo.restoreCalled)
		assert.False(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)

		require.NotNil(t, env.memberRepo.lastRestoreParam)
		assert.Equal(t, pmTestProjectID, env.memberRepo.lastRestoreParam.ProjectID)
		assert.Equal(t, pmTestOtherID, env.memberRepo.lastRestoreParam.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, env.memberRepo.lastRestoreParam.Role)
		require.NotNil(t, env.memberRepo.lastRestoreParam.InvitedBy)
		assert.Equal(t, pmTestOwnerID, *env.memberRepo.lastRestoreParam.InvitedBy)
		assert.False(t, env.memberRepo.lastRestoreParam.JoinedAt.IsZero())

		pm, getErr := env.memberRepo.GetProjectMemberByProjectIDAndUserID(
			env.ctx,
			pmTestProjectID,
			pmTestOtherID,
		)
		require.NoError(t, getErr)
		require.NotNil(t, pm)

		assert.Equal(t, pmTestProjectID, pm.ProjectID)
		assert.Equal(t, pmTestOtherID, pm.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, pm.Role)
		assert.False(t, pm.DeletedAt.Valid)
		assert.False(t, pm.JoinedAt.IsZero())
		assert.True(t, pm.JoinedAt.After(oldJoinedAt))
		assert.False(t, pm.UpdatedAt.IsZero())

		require.NotNil(t, pm.InvitedBy)
		assert.Equal(t, pmTestOwnerID, *pm.InvitedBy)

		assert.Equal(t, pmTestProjectID, resp.ProjectID)
		assert.Equal(t, pmTestOtherID, resp.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, resp.Role)
	})

	t.Run("failed active member already exists", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestMemberID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add member] active member already exists resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberAlreadyExists)
		assert.Nil(t, resp)

		assert.True(t, env.memberRepo.existsCalled)
		assert.False(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.memberRepo.restoreCalled)
	})

	t.Run("failed restore repository error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.restoreErr = errPMRepoMock

		deletedAt := time.Now().Add(-2 * time.Hour)
		env.memberRepo.members[pmProjectMemberKey(pmTestProjectID, pmTestOtherID)] = &model.ProjectMember{
			ID:        1000,
			ProjectID: pmTestProjectID,
			UserID:    pmTestOtherID,
			Role:      model.ProjectMemberRoleMember,
			InvitedBy: pmUint64Ptr(pmTestAdminID),
			JoinedAt:  time.Now().Add(-48 * time.Hour),
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: deletedAt,
			DeletedAt: gorm.DeletedAt{
				Time:  deletedAt,
				Valid: true,
			},
		}

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestOtherID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add member] restore repository error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)

		assert.True(t, env.memberRepo.existsCalled)
		assert.True(t, env.memberRepo.getUnscopedCalled)
		assert.True(t, env.memberRepo.restoreCalled)
		assert.False(t, env.memberRepo.createWithTxCalled)

		pm, getErr := env.memberRepo.GetProjectMemberByProjectIDAndUserIDUnscoped(
			env.ctx,
			pmTestProjectID,
			pmTestOtherID,
		)
		require.NoError(t, getErr)
		require.NotNil(t, pm)

		// 恢复失败后，仍然应该保持软删除状态。
		assert.True(t, pm.DeletedAt.Valid)
	})
}

// =========================
// Helper
// =========================

func pmProjectMemberKey(projectID uint64, userID uint64) string {
	return fmt.Sprintf("%d:%d", projectID, userID)
}

func pmProjectMemberActive(pm *model.ProjectMember) bool {
	return pm != nil && !pm.DeletedAt.Valid
}

func pmCloneProjectMember(pm *model.ProjectMember) *model.ProjectMember {
	if pm == nil {
		return nil
	}

	cp := *pm
	return &cp
}

func pmUint64Ptr(v uint64) *uint64 {
	return &v
}

// =========================
// Mock User Repo
// =========================

type pmMockUserRepo struct {
	mu    sync.RWMutex
	users map[uint64]*model.User

	getByIDErr error
}

func pmNewMockUserRepo() *pmMockUserRepo {
	return &pmMockUserRepo{
		users: make(map[uint64]*model.User),
	}
}

func (m *pmMockUserRepo) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}

	user, ok := m.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}

	return pmCloneUser(user), nil
}

func pmCloneUser(user *model.User) *model.User {
	if user == nil {
		return nil
	}

	cp := *user
	return &cp
}

// =========================
// Mock Project Repo
// =========================

type pmMockProjectRepo struct {
	mu       sync.RWMutex
	projects map[uint64]bool

	existsErr error
}

func pmNewMockProjectRepo() *pmMockProjectRepo {
	return &pmMockProjectRepo{
		projects: make(map[uint64]bool),
	}
}

func (m *pmMockProjectRepo) ExistsByProjectID(ctx context.Context, id uint64) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.existsErr != nil {
		return false, m.existsErr
	}

	return m.projects[id], nil
}

// =========================
// Mock Project Member Repo
// =========================

type pmMockProjectMemberRepo struct {
	mu sync.RWMutex

	// members 保存全部成员，包括有效成员和已软删除成员。
	members map[string]*model.ProjectMember

	createWithTxCalled bool
	softDeleteCalled   bool
	restoreCalled      bool
	getUnscopedCalled  bool
	existsCalled       bool

	lastTxIsNil bool

	lastCreatedMember *model.ProjectMember

	lastSoftDeleteProjectID uint64
	lastSoftDeleteUserID    uint64

	lastRestoreParam *repository.RestoreProjectMemberWithTxParam

	createErr      error
	softDeleteErr  error
	restoreErr     error
	getErr         error
	getUnscopedErr error
	existsErr      error
	countErr       error
}

func pmNewMockProjectMemberRepo() *pmMockProjectMemberRepo {
	return &pmMockProjectMemberRepo{
		members: make(map[string]*model.ProjectMember),
	}
}

func (m *pmMockProjectMemberRepo) CreateWithTx(
	ctx context.Context,
	tx *gorm.DB,
	projectMember *model.ProjectMember,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createWithTxCalled = true
	m.lastTxIsNil = tx == nil

	if m.createErr != nil {
		return m.createErr
	}
	if projectMember == nil {
		return repository.ErrProjectMemberQueryInvalid
	}
	if projectMember.ProjectID <= 0 || projectMember.UserID <= 0 {
		return repository.ErrProjectMemberQueryInvalid
	}

	key := pmProjectMemberKey(projectMember.ProjectID, projectMember.UserID)

	// 模拟数据库唯一索引 uk_project_user：即使软删除记录存在，也不能重复创建。
	if _, ok := m.members[key]; ok {
		return service.ErrProjectMemberAlreadyExists
	}

	now := time.Now()
	cp := pmCloneProjectMember(projectMember)

	if cp.ID == 0 {
		cp.ID = uint64(len(m.members) + 1)
	}
	if cp.JoinedAt.IsZero() {
		cp.JoinedAt = now
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = now
	}

	cp.DeletedAt = gorm.DeletedAt{}
	m.members[key] = cp
	m.lastCreatedMember = pmCloneProjectMember(cp)

	return nil
}

func (m *pmMockProjectMemberRepo) SoftDeleteProjectMember(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	userID uint64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.softDeleteCalled = true
	m.lastTxIsNil = tx == nil
	m.lastSoftDeleteProjectID = projectID
	m.lastSoftDeleteUserID = userID

	if m.softDeleteErr != nil {
		return m.softDeleteErr
	}
	if projectID <= 0 || userID <= 0 {
		return repository.ErrProjectMemberQueryInvalid
	}

	pm, ok := m.members[pmProjectMemberKey(projectID, userID)]
	if !ok || !pmProjectMemberActive(pm) {
		return repository.ErrProjectMemberNotFound
	}

	now := time.Now()
	pm.DeletedAt = gorm.DeletedAt{
		Time:  now,
		Valid: true,
	}
	pm.UpdatedAt = now

	return nil
}

// =========================
// Mock Task Repo
// =========================

type pmMockTaskRepo struct {
	mu sync.RWMutex

	clearCalled bool
	lastTxIsNil bool

	lastProjectID  uint64
	lastAssigneeID uint64
	lastUpdatedAt  time.Time

	clearErr error
}

func pmNewMockTaskRepo() *pmMockTaskRepo {
	return &pmMockTaskRepo{}
}

// ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx 清空指定项目下指定负责人的任务负责人
func (m *pmMockTaskRepo) ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	assigneeID uint64,
	updatedAt time.Time,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clearCalled = true
	m.lastTxIsNil = tx == nil
	m.lastProjectID = projectID
	m.lastAssigneeID = assigneeID
	m.lastUpdatedAt = updatedAt

	if m.clearErr != nil {
		return m.clearErr
	}

	return nil
}

// RemoveProjectMember 兼容旧接口名。
// 如果业务代码已经完全替换为 SoftDeleteProjectMember，这个方法不会被调用。
func (m *pmMockProjectMemberRepo) RemoveProjectMember(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	userID uint64,
) error {
	return m.SoftDeleteProjectMember(ctx, tx, projectID, userID)
}

func (m *pmMockProjectMemberRepo) RestoreProjectMemberWithTx(
	ctx context.Context,
	tx *gorm.DB,
	param *repository.RestoreProjectMemberWithTxParam,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.restoreCalled = true
	m.lastTxIsNil = tx == nil
	m.lastRestoreParam = param

	if m.restoreErr != nil {
		return m.restoreErr
	}
	if param == nil || param.ProjectID <= 0 || param.UserID <= 0 {
		return repository.ErrProjectMemberQueryInvalid
	}

	key := pmProjectMemberKey(param.ProjectID, param.UserID)
	pm, ok := m.members[key]
	if !ok || !pm.DeletedAt.Valid {
		return repository.ErrProjectMemberNotFound
	}

	now := time.Now()

	pm.Role = param.Role
	pm.InvitedBy = param.InvitedBy
	pm.JoinedAt = param.JoinedAt
	pm.UpdatedAt = now
	pm.DeletedAt = gorm.DeletedAt{}

	return nil
}

func (m *pmMockProjectMemberRepo) ExistsByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.existsCalled = true

	if m.existsErr != nil {
		return false, m.existsErr
	}

	pm, ok := m.members[pmProjectMemberKey(projectID, userID)]
	return ok && pmProjectMemberActive(pm), nil
}

func (m *pmMockProjectMemberRepo) GetProjectMemberByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (*model.ProjectMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	pm, ok := m.members[pmProjectMemberKey(projectID, userID)]
	if !ok || !pmProjectMemberActive(pm) {
		return nil, repository.ErrProjectMemberNotFound
	}

	return pmCloneProjectMember(pm), nil
}

func (m *pmMockProjectMemberRepo) GetProjectMemberByProjectIDAndUserIDUnscoped(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (*model.ProjectMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getUnscopedCalled = true

	if m.getUnscopedErr != nil {
		return nil, m.getUnscopedErr
	}

	pm, ok := m.members[pmProjectMemberKey(projectID, userID)]
	if !ok {
		return nil, repository.ErrProjectMemberNotFound
	}

	return pmCloneProjectMember(pm), nil
}

func (m *pmMockProjectMemberRepo) GetProjectMemberRoleByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return "", m.getErr
	}

	pm, ok := m.members[pmProjectMemberKey(projectID, userID)]
	if !ok || !pmProjectMemberActive(pm) {
		return "", repository.ErrProjectMemberNotFound
	}

	return pm.Role, nil
}

func (m *pmMockProjectMemberRepo) CountByProjectIDAndRole(
	ctx context.Context,
	projectID uint64,
	role string,
) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.countErr != nil {
		return 0, m.countErr
	}

	var count int64
	for _, pm := range m.members {
		if pm == nil {
			continue
		}
		if pm.ProjectID == projectID && pm.Role == role && pmProjectMemberActive(pm) {
			count++
		}
	}

	return count, nil
}

func (m *pmMockProjectMemberRepo) SearchProjectMembers(
	ctx context.Context,
	query *repository.ProjectMemberSearchQuery,
) ([]*model.ProjectMember, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*model.ProjectMember, 0)

	for _, pm := range m.members {
		if !pmProjectMemberActive(pm) {
			continue
		}

		members = append(members, pmCloneProjectMember(pm))
	}

	return members, int64(len(members)), nil
}

func (m *pmMockProjectMemberRepo) UpdateProjectMemberRole(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	userID uint64,
	role string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pm, ok := m.members[pmProjectMemberKey(projectID, userID)]
	if !ok || !pmProjectMemberActive(pm) {
		return repository.ErrProjectMemberNotFound
	}

	pm.Role = role
	pm.UpdatedAt = time.Now()

	return nil
}
