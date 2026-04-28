// internal/service/test/project_member_service_test.go
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

	"smart-task-platform/internal/model"
	"smart-task-platform/internal/repository"
	service "smart-task-platform/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	pmTestProjectID = uint64(10001)

	pmTestOwnerID  = uint64(1)
	pmTestAdminID  = uint64(2)
	pmTestMemberID = uint64(3)
	pmTestUserID   = uint64(4)
	pmTestOtherID  = uint64(5)
)

var errPMRepoMock = errors.New("mock repository error")

// pmProjectMemberServiceTestEnv 项目成员 Service 测试环境。
type pmProjectMemberServiceTestEnv struct {
	ctx context.Context

	db          *gorm.DB
	txMgr       *repository.TxManager
	userRepo    *pmMockUserRepo
	projectRepo *pmMockProjectRepo
	memberRepo  *pmMockProjectMemberRepo
	svc         *service.ProjectMemberService
}

// pmNewProjectMemberServiceTestEnv 创建独立测试环境，避免测试之间数据污染。
func pmNewProjectMemberServiceTestEnv(t *testing.T) *pmProjectMemberServiceTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	oldLogger := zap.L()
	zap.ReplaceGlobals(logger)

	// 使用 sqlite 内存数据库构造真实事务环境，不连接真实 MySQL。
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	require.NoError(t, err)

	ctx := context.Background()
	txMgr := repository.NewTxManager(db)

	userRepo := pmNewMockUserRepo()
	projectRepo := pmNewMockProjectRepo()
	memberRepo := pmNewMockProjectMemberRepo()

	svc := service.NewProjectMemberService(
		txMgr,
		userRepo,
		projectRepo,
		memberRepo,
	)

	env := &pmProjectMemberServiceTestEnv{
		ctx:         ctx,
		db:          db,
		txMgr:       txMgr,
		userRepo:    userRepo,
		projectRepo: projectRepo,
		memberRepo:  memberRepo,
		svc:         svc,
	}

	env.pmSeedDefaultData(t)

	t.Cleanup(func() {
		zap.ReplaceGlobals(oldLogger)

		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return env
}

// pmSeedDefaultData 初始化默认用户、项目、项目成员数据。
func (e *pmProjectMemberServiceTestEnv) pmSeedDefaultData(t *testing.T) {
	t.Helper()

	e.projectRepo.projects[pmTestProjectID] = true

	e.userRepo.users[pmTestOwnerID] = pmBuildTestUser(pmTestOwnerID, "owner", "项目拥有者")
	e.userRepo.users[pmTestAdminID] = pmBuildTestUser(pmTestAdminID, "admin", "项目管理员")
	e.userRepo.users[pmTestMemberID] = pmBuildTestUser(pmTestMemberID, "member", "普通成员")
	e.userRepo.users[pmTestUserID] = pmBuildTestUser(pmTestUserID, "new_user", "新用户")
	e.userRepo.users[pmTestOtherID] = pmBuildTestUser(pmTestOtherID, "other_user", "其他用户")

	e.memberRepo.members[pmMemberKey(pmTestProjectID, pmTestOwnerID)] = pmBuildTestProjectMember(
		pmTestProjectID,
		pmTestOwnerID,
		model.ProjectMemberRoleOwner,
		e.userRepo.users[pmTestOwnerID],
	)

	e.memberRepo.members[pmMemberKey(pmTestProjectID, pmTestAdminID)] = pmBuildTestProjectMember(
		pmTestProjectID,
		pmTestAdminID,
		model.ProjectMemberRoleAdmin,
		e.userRepo.users[pmTestAdminID],
	)

	e.memberRepo.members[pmMemberKey(pmTestProjectID, pmTestMemberID)] = pmBuildTestProjectMember(
		pmTestProjectID,
		pmTestMemberID,
		model.ProjectMemberRoleMember,
		e.userRepo.users[pmTestMemberID],
	)
}

// pmBuildTestUser 构造测试用户。
func pmBuildTestUser(id uint64, username, nickname string) *model.User {
	return &model.User{
		ID:       id,
		Username: username,
		Email:    fmt.Sprintf("%s@example.com", username),
		Nickname: nickname,
		Avatar:   fmt.Sprintf("https://example.com/avatar/%d.png", id),
	}
}

// pmBuildTestProjectMember 构造测试项目成员。
func pmBuildTestProjectMember(projectID, userID uint64, role string, user *model.User) *model.ProjectMember {
	pm := &model.ProjectMember{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
		JoinedAt:  time.Now(),
	}

	if user != nil {
		pm.User = *user
	}

	return pm
}

// pmMemberKey 构造项目成员 map key。
func pmMemberKey(projectID, userID uint64) string {
	return fmt.Sprintf("%d:%d", projectID, userID)
}

// pmStringPtr 构造 string 指针。
func pmStringPtr(v string) *string {
	return &v
}

// pmCloneUser 克隆用户，避免测试直接修改 mock 底层数据。
func pmCloneUser(u *model.User) *model.User {
	if u == nil {
		return nil
	}

	cp := *u
	return &cp
}

// pmCloneProjectMember 克隆项目成员，避免测试直接修改 mock 底层数据。
func pmCloneProjectMember(pm *model.ProjectMember) *model.ProjectMember {
	if pm == nil {
		return nil
	}

	cp := *pm
	return &cp
}

// =========================
// AddProjectMember
// =========================

func TestProjectMemberServiceAddProjectMember(t *testing.T) {
	t.Run("success owner add member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		t.Logf("[add project member] start project_id=%d invitor_id=%d invited_user_id=%d role=%s",
			pmTestProjectID,
			pmTestOwnerID,
			pmTestUserID,
			model.ProjectMemberRoleMember,
		)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, pmTestProjectID, resp.ProjectID)
		assert.Equal(t, pmTestUserID, resp.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, resp.Role)
		require.NotNil(t, resp.User)
		assert.Equal(t, pmTestUserID, resp.User.ID)

		assert.True(t, env.memberRepo.existsCalled)
		assert.True(t, env.memberRepo.getCalled)
		assert.True(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)

		pm, err := env.memberRepo.GetProjectMemberByProjectIDAndUserID(env.ctx, pmTestProjectID, pmTestUserID)
		require.NoError(t, err)
		assert.Equal(t, pmTestProjectID, pm.ProjectID)
		assert.Equal(t, pmTestUserID, pm.UserID)
		assert.Equal(t, model.ProjectMemberRoleMember, pm.Role)
		assert.False(t, pm.JoinedAt.IsZero())
	})

	t.Run("success owner add admin", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleAdmin,
		})

		t.Logf("[add project member] owner add admin resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, model.ProjectMemberRoleAdmin, resp.Role)
		assert.True(t, env.memberRepo.countCalled)
		assert.True(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, nil)

		t.Logf("[add project member] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed invalid param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     0,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed invalid role", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          "super_admin",
		})

		t.Logf("[add project member] invalid role resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberRole)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed project not found", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     99999,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed project repo error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.projectRepo.existsByProjectIDErr = errPMRepoMock

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] project repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed invited user not found", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: 99999,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] invited user not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrUserNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed user repo error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.userRepo.getByIDErr = errPMRepoMock

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] user repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed invitor not project member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOtherID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] invitor not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed admin add admin forbidden", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestAdminID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleAdmin,
		})

		t.Logf("[add project member] admin add admin forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed member add member forbidden", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestMemberID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] member add member forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed project member already exists", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestMemberID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] already exists resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberAlreadyExists)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed admin count exceeded", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.adminCountOverride = int64(service.AdminCount)

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleAdmin,
		})

		t.Logf("[add project member] admin count exceeded resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrExceedsAdminMemberLimit)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.createWithTxCalled)
	})

	t.Run("failed create with tx repo error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.createErr = errPMRepoMock

		resp, err := env.svc.AddProjectMember(env.ctx, &service.AddProjectMemberParam{
			ProjectID:     pmTestProjectID,
			InvitorID:     pmTestOwnerID,
			InvitedUserID: pmTestUserID,
			Role:          model.ProjectMemberRoleMember,
		})

		t.Logf("[add project member] create repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
	})
}

// =========================
// ListProjectMembers
// =========================

func TestProjectMemberServiceListProjectMembers(t *testing.T) {
	t.Run("success list all members", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list project members] list all resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.EqualValues(t, 3, resp.Total)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
		assert.Len(t, resp.List, 3)
		assert.True(t, env.memberRepo.searchCalled)

		for _, item := range resp.List {
			require.NotNil(t, item)
			assert.Equal(t, pmTestProjectID, item.ProjectID)
			assert.NotZero(t, item.UserID)
			assert.NotEmpty(t, item.Role)
			require.NotNil(t, item.User)
			assert.NotEmpty(t, item.User.Username)
		}
	})

	t.Run("success filter by role", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
			Role:      model.ProjectMemberRoleAdmin,
		})

		t.Logf("[list project members] filter role resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.EqualValues(t, 1, resp.Total)
		require.Len(t, resp.List, 1)
		assert.Equal(t, pmTestAdminID, resp.List[0].UserID)
		assert.Equal(t, model.ProjectMemberRoleAdmin, resp.List[0].Role)
	})

	t.Run("success filter by keyword username", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
			Keyword:   "admin",
		})

		t.Logf("[list project members] filter keyword resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.EqualValues(t, 1, resp.Total)
		require.Len(t, resp.List, 1)
		assert.Equal(t, pmTestAdminID, resp.List[0].UserID)
	})

	t.Run("success pagination", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: pmTestProjectID,
			Page:      2,
			PageSize:  2,
		})

		t.Logf("[list project members] pagination resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.EqualValues(t, 3, resp.Total)
		assert.Equal(t, 2, resp.Page)
		assert.Equal(t, 2, resp.PageSize)
		assert.Len(t, resp.List, 1)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, nil)

		t.Logf("[list project members] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.searchCalled)
	})

	t.Run("failed invalid param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    0,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list project members] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.searchCalled)
	})

	t.Run("failed invalid role", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
			Role:      "bad_role",
		})

		t.Logf("[list project members] invalid role resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberRole)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.searchCalled)
	})

	t.Run("failed project not found", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: 99999,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list project members] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.searchCalled)
	})

	t.Run("failed current user not project member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOtherID,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list project members] current user not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.searchCalled)
	})

	t.Run("failed search repo error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.searchErr = errPMRepoMock

		resp, err := env.svc.ListProjectMembers(env.ctx, &service.ListProjectMembersParam{
			UserID:    pmTestOwnerID,
			ProjectID: pmTestProjectID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list project members] search repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.memberRepo.searchCalled)
	})
}

// =========================
// UpdateProjectMember
// =========================

func TestProjectMemberServiceUpdateProjectMember(t *testing.T) {
	t.Run("success owner update member to admin", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, pmTestProjectID, resp.ProjectID)
		assert.Equal(t, pmTestMemberID, resp.UserID)
		assert.Equal(t, model.ProjectMemberRoleAdmin, resp.Role)
		assert.True(t, env.memberRepo.countCalled)
		assert.True(t, env.memberRepo.updateRoleCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)

		pm, err := env.memberRepo.GetProjectMemberByProjectIDAndUserID(env.ctx, pmTestProjectID, pmTestMemberID)
		require.NoError(t, err)
		assert.Equal(t, model.ProjectMemberRoleAdmin, pm.Role)
	})

	t.Run("success nil role idempotent", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           nil,
		})

		t.Logf("[update project member] nil role resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, model.ProjectMemberRoleMember, resp.Role)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("success same role idempotent", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleMember),
		})

		t.Logf("[update project member] same role resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, model.ProjectMemberRoleMember, resp.Role)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, nil)

		t.Logf("[update project member] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed invalid param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      0,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed invalid role", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr("bad_role"),
		})

		t.Logf("[update project member] invalid role resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberRole)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed project not found", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      99999,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed modifier not owner", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestAdminID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] modifier not owner resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed owner cannot demote self", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestOwnerID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] owner demote self resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed modified user not project member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestOtherID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] modified user not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed admin member limit exceeded", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.adminCountOverride = int64(service.AdminCount)

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] admin limit exceeded resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrExceedsAdminMemberLimit)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.updateRoleCalled)
	})

	t.Run("failed update role repo error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.updateRoleErr = errPMRepoMock

		resp, err := env.svc.UpdateProjectMember(env.ctx, &service.UpdateProjectMemberParam{
			ProjectID:      pmTestProjectID,
			ModifierID:     pmTestOwnerID,
			ModifiedUserID: pmTestMemberID,
			Role:           pmStringPtr(model.ProjectMemberRoleAdmin),
		})

		t.Logf("[update project member] update role repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.memberRepo.updateRoleCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
	})
}

// =========================
// RemoveProjectMember
// =========================

func TestProjectMemberServiceRemoveProjectMember(t *testing.T) {
	t.Run("success owner remove member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[remove project member] owner remove member resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.removeCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)

		_, err = env.memberRepo.GetProjectMemberByProjectIDAndUserID(env.ctx, pmTestProjectID, pmTestMemberID)
		require.ErrorIs(t, err, repository.ErrProjectMemberNotFound)
	})

	t.Run("success admin remove member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestAdminID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[remove project member] admin remove member resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.removeCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, nil)

		t.Logf("[remove project member] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed invalid param", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     0,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[remove project member] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectMemberParam)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed project not found", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     99999,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[remove project member] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed operator not project member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOtherID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[remove project member] operator not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed removed user not project member", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestOtherID,
		})

		t.Logf("[remove project member] removed user not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed admin remove owner forbidden", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestAdminID,
			RemovedUserID: pmTestOwnerID,
		})

		t.Logf("[remove project member] admin remove owner forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed member remove admin forbidden", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestMemberID,
			RemovedUserID: pmTestAdminID,
		})

		t.Logf("[remove project member] member remove admin forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.memberRepo.removeCalled)
	})

	t.Run("failed remove repo error", func(t *testing.T) {
		env := pmNewProjectMemberServiceTestEnv(t)
		env.memberRepo.removeErr = errPMRepoMock

		resp, err := env.svc.RemoveProjectMember(env.ctx, &service.RemoveProjectMemberParam{
			ProjectID:     pmTestProjectID,
			OperatorID:    pmTestOwnerID,
			RemovedUserID: pmTestMemberID,
		})

		t.Logf("[remove project member] remove repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errPMRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.memberRepo.removeCalled)
		assert.False(t, env.memberRepo.lastTxIsNil)
	})
}

// =========================
// Mock User Repository
// =========================

type pmMockUserRepo struct {
	mu sync.RWMutex

	users map[uint64]*model.User

	getByIDErr error
}

func pmNewMockUserRepo() *pmMockUserRepo {
	return &pmMockUserRepo{
		users: make(map[uint64]*model.User),
	}
}

// GetByID 根据用户 ID 获取用户。
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

// =========================
// Mock Project Repository
// =========================

type pmMockProjectRepo struct {
	mu sync.RWMutex

	projects map[uint64]bool

	existsByProjectIDErr error
}

func pmNewMockProjectRepo() *pmMockProjectRepo {
	return &pmMockProjectRepo{
		projects: make(map[uint64]bool),
	}
}

// ExistsByProjectID 判断项目是否存在。
func (m *pmMockProjectRepo) ExistsByProjectID(ctx context.Context, projectID uint64) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.existsByProjectIDErr != nil {
		return false, m.existsByProjectIDErr
	}

	return m.projects[projectID], nil
}

// =========================
// Mock ProjectMember Repository
// =========================

type pmMockProjectMemberRepo struct {
	mu sync.RWMutex

	members map[string]*model.ProjectMember

	createWithTxCalled bool
	searchCalled       bool
	updateRoleCalled   bool
	removeCalled       bool
	existsCalled       bool
	getCalled          bool
	countCalled        bool

	lastTxIsNil bool

	createErr     error
	searchErr     error
	getErr        error
	existsErr     error
	updateRoleErr error
	removeErr     error
	countErr      error

	adminCountOverride int64
}

func pmNewMockProjectMemberRepo() *pmMockProjectMemberRepo {
	return &pmMockProjectMemberRepo{
		members:            make(map[string]*model.ProjectMember),
		adminCountOverride: -1,
	}
}

// ExistsByProjectIDAndUserID 判断用户是否是项目成员。
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

	_, ok := m.members[pmMemberKey(projectID, userID)]
	return ok, nil
}

// GetProjectMemberByProjectIDAndUserID 获取项目成员。
func (m *pmMockProjectMemberRepo) GetProjectMemberByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (*model.ProjectMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getCalled = true

	if m.getErr != nil {
		return nil, m.getErr
	}

	pm, ok := m.members[pmMemberKey(projectID, userID)]
	if !ok {
		return nil, repository.ErrProjectMemberNotFound
	}

	return pmCloneProjectMember(pm), nil
}

// 👇 缺失的方法：必须加上！
func (m *pmMockProjectMemberRepo) GetProjectMemberRoleByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 调用标记（可选，用于测试断言）
	m.getCalled = true

	// 从 mock 数据中获取成员
	key := pmMemberKey(projectID, userID)
	pm, ok := m.members[key]
	if !ok {
		return "", repository.ErrProjectMemberNotFound
	}

	// 返回角色
	return pm.Role, nil
}

// CreateWithTx 创建项目成员，方法名和返回类型必须匹配 service.projectMemberSvcProjectMemberRepo。
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
		return service.ErrInvalidProjectMemberParam
	}

	if projectMember.JoinedAt.IsZero() {
		projectMember.JoinedAt = time.Now()
	}

	cp := *projectMember
	m.members[pmMemberKey(cp.ProjectID, cp.UserID)] = &cp

	return nil
}

// SearchProjectMembers 查询项目成员列表。
func (m *pmMockProjectMemberRepo) SearchProjectMembers(
	ctx context.Context,
	query *repository.ProjectMemberSearchQuery,
) ([]*model.ProjectMember, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchCalled = true

	if m.searchErr != nil {
		return nil, 0, m.searchErr
	}

	if query == nil {
		return nil, 0, service.ErrInvalidProjectMemberParam
	}

	role := strings.TrimSpace(query.Role)
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))

	list := make([]*model.ProjectMember, 0)

	for _, pm := range m.members {
		if pm == nil {
			continue
		}

		if pm.ProjectID != query.ProjectID {
			continue
		}

		if role != "" && pm.Role != role {
			continue
		}

		if keyword != "" {
			username := strings.ToLower(pm.User.Username)
			nickname := strings.ToLower(pm.User.Nickname)
			email := strings.ToLower(pm.User.Email)

			if !strings.Contains(username, keyword) &&
				!strings.Contains(nickname, keyword) &&
				!strings.Contains(email, keyword) {
				continue
			}
		}

		list = append(list, pmCloneProjectMember(pm))
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].UserID < list[j].UserID
	})

	total := int64(len(list))

	page := query.Page
	pageSize := query.PageSize

	if page <= 0 {
		page = 1
	}

	if pageSize <= 0 {
		pageSize = 10
	}

	start := (page - 1) * pageSize
	if start >= len(list) {
		return []*model.ProjectMember{}, total, nil
	}

	end := start + pageSize
	if end > len(list) {
		end = len(list)
	}

	return list[start:end], total, nil
}

// UpdateProjectMemberRole 更新项目成员角色。
func (m *pmMockProjectMemberRepo) UpdateProjectMemberRole(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	userID uint64,
	role string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.updateRoleCalled = true
	m.lastTxIsNil = tx == nil

	if m.updateRoleErr != nil {
		return m.updateRoleErr
	}

	pm, ok := m.members[pmMemberKey(projectID, userID)]
	if !ok {
		return repository.ErrProjectMemberNotFound
	}

	pm.Role = role
	return nil
}

// RemoveProjectMember 移除项目成员。
func (m *pmMockProjectMemberRepo) RemoveProjectMember(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	userID uint64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.removeCalled = true
	m.lastTxIsNil = tx == nil

	if m.removeErr != nil {
		return m.removeErr
	}

	key := pmMemberKey(projectID, userID)
	if _, ok := m.members[key]; !ok {
		return repository.ErrProjectMemberNotFound
	}

	delete(m.members, key)

	return nil
}

// CountByProjectIDAndRole 按角色统计项目成员数量，返回值必须是 int64。
func (m *pmMockProjectMemberRepo) CountByProjectIDAndRole(
	ctx context.Context,
	projectID uint64,
	role string,
) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.countCalled = true

	if m.countErr != nil {
		return 0, m.countErr
	}

	if role != model.ProjectMemberRoleAdmin {
		return 0, nil
	}

	if m.adminCountOverride >= 0 {
		return m.adminCountOverride, nil
	}

	var total int64
	for _, pm := range m.members {
		if pm == nil {
			continue
		}

		if pm.ProjectID == projectID && pm.Role == model.ProjectMemberRoleAdmin {
			total++
		}
	}

	return total, nil
}
