// internal/service/test/project_service_test.go
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"smart-task-platform/internal/model"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service"
)

const (
	psTestOwnerUsername  = "ps_test_owner"
	psTestOwnerNickname  = "项目拥有者"
	psTestOwnerAvatar    = "https://example.com/project-owner.png"
	psTestAdminUsername  = "ps_test_admin"
	psTestAdminNickname  = "项目管理员"
	psTestAdminAvatar    = "https://example.com/project-admin.png"
	psTestMemberUsername = "ps_test_member"
	psTestMemberNickname = "普通成员"
	psTestMemberAvatar   = "https://example.com/project-member.png"

	psTestProjectName        = "测试项目"
	psTestProjectDescription = "测试项目描述"
	psTestProjectNewName     = "更新后的测试项目"
	psTestProjectNewDesc     = "更新后的测试项目描述"

	psTestStartTime    = "2026-01-01T10:00:00+08:00"
	psTestEndTime      = "2026-01-31T18:00:00+08:00"
	psTestNewStartTime = "2026-02-01T10:00:00+08:00"
	psTestNewEndTime   = "2026-02-28T18:00:00+08:00"
)

// psMockUserRepo 项目服务测试专用用户仓储 mock
type psMockUserRepo struct {
	mu     sync.RWMutex
	users  map[uint64]*model.User
	nextID uint64

	getByIDErr error
}

func psNewMockUserRepo() *psMockUserRepo {
	return &psMockUserRepo{
		users:  make(map[uint64]*model.User),
		nextID: 1,
	}
}

// psCloneUser 克隆用户，避免测试直接修改 mock 底层数据。
func psCloneUser(u *model.User) *model.User {
	if u == nil {
		return nil
	}

	cp := *u
	return &cp
}

func (m *psMockUserRepo) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}

	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}

	return psCloneUser(u), nil
}

// psMockProjectRepo 项目服务测试专用项目仓储 mock。
type psMockProjectRepo struct {
	mu       sync.RWMutex
	projects map[uint64]*model.Project
	nextID   uint64
	userRepo *psMockUserRepo

	createWithTxCalled  bool
	updateWithTxCalled  bool
	archiveWithTxCalled bool
	lastTxIsNil         bool

	createErr  error
	searchErr  error
	detailErr  error
	updateErr  error
	archiveErr error
}

func psNewMockProjectRepo(userRepo *psMockUserRepo) *psMockProjectRepo {
	return &psMockProjectRepo{
		projects: make(map[uint64]*model.Project),
		nextID:   1,
		userRepo: userRepo,
	}
}

// psCloneProject 克隆项目，避免测试直接修改 mock 底层数据。
func psCloneProject(p *model.Project) *model.Project {
	if p == nil {
		return nil
	}

	cp := *p
	return &cp
}

// psFillOwner 模拟 repository 层 Preload Owner。
func (m *psMockProjectRepo) psFillOwner(p *model.Project) {
	if p == nil || m.userRepo == nil {
		return
	}

	u, err := m.userRepo.GetByID(context.Background(), p.OwnerID)
	if err != nil || u == nil {
		return
	}

	p.Owner = u
}

func (m *psMockProjectRepo) CreateWithTx(ctx context.Context, tx *gorm.DB, project *model.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createWithTxCalled = true
	m.lastTxIsNil = tx == nil

	if m.createErr != nil {
		return m.createErr
	}
	if project == nil {
		return errors.New("mock project is nil")
	}

	now := time.Now()
	project.ID = m.nextID
	project.CreatedAt = now
	project.UpdatedAt = now

	m.projects[project.ID] = psCloneProject(project)
	m.nextID++

	return nil
}

func (m *psMockProjectRepo) SearchProjects(
	ctx context.Context,
	query *repository.ProjectSearchQuery,
) ([]*model.Project, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.searchErr != nil {
		return nil, 0, m.searchErr
	}
	if query == nil {
		return nil, 0, errors.New("mock project search query is nil")
	}

	visibleProjectIDs := make(map[uint64]struct{}, len(query.ProjectIDs))
	for _, id := range query.ProjectIDs {
		visibleProjectIDs[id] = struct{}{}
	}

	keyword := strings.TrimSpace(query.Keyword)
	status := strings.TrimSpace(query.Status)

	matched := make([]*model.Project, 0)
	for _, p := range m.projects {
		if _, ok := visibleProjectIDs[p.ID]; !ok {
			continue
		}
		if status != "" && p.Status != status {
			continue
		}
		if keyword != "" && !strings.Contains(p.Name, keyword) {
			continue
		}

		cp := psCloneProject(p)
		m.psFillOwner(cp)
		matched = append(matched, cp)
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].Name < matched[j].Name
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := int64(len(matched))
	page := query.Page
	pageSize := query.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	start := (page - 1) * pageSize
	if start >= len(matched) {
		return []*model.Project{}, total, nil
	}

	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}

	return matched[start:end], total, nil
}

func (m *psMockProjectRepo) GetDetailByID(ctx context.Context, id uint64) (*model.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.detailErr != nil {
		return nil, m.detailErr
	}

	p, ok := m.projects[id]
	if !ok {
		return nil, repository.ErrProjectNotFound
	}

	cp := psCloneProject(p)
	m.psFillOwner(cp)

	return cp, nil
}

func (m *psMockProjectRepo) UpdateProjectInformationWithTx(
	ctx context.Context,
	tx *gorm.DB,
	id uint64,
	data *repository.UpdateProjectData,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.updateWithTxCalled = true
	m.lastTxIsNil = tx == nil

	if m.updateErr != nil {
		return m.updateErr
	}
	if data == nil {
		return errors.New("mock update project data is nil")
	}

	p, ok := m.projects[id]
	if !ok {
		return repository.ErrProjectNotFound
	}

	if data.Name != "" {
		p.Name = data.Name
	}
	if data.Description != "" {
		p.Description = data.Description
	}
	if data.Status != "" {
		p.Status = data.Status
	}
	if data.StartDate != nil {
		p.StartDate = data.StartDate
	}
	if data.EndDate != nil {
		p.EndDate = data.EndDate
	}

	p.UpdatedAt = time.Now()
	m.projects[id] = p

	return nil
}

func (m *psMockProjectRepo) ArchiveProjectWithTx(ctx context.Context, tx *gorm.DB, id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.archiveWithTxCalled = true
	m.lastTxIsNil = tx == nil

	if m.archiveErr != nil {
		return m.archiveErr
	}

	p, ok := m.projects[id]
	if !ok {
		return repository.ErrProjectNotFound
	}

	p.Status = model.ProjectStatusArchived
	p.UpdatedAt = time.Now()
	m.projects[id] = p

	return nil
}

// psMockProjectMemberRepo 项目服务测试专用项目成员仓储 mock。
type psMockProjectMemberRepo struct {
	mu      sync.RWMutex
	members map[string]*model.ProjectMember

	createWithTxCalled bool
	lastTxIsNil        bool

	createErr error
	listErr   error
	existsErr error
	getErr    error
}

func psNewMockProjectMemberRepo() *psMockProjectMemberRepo {
	return &psMockProjectMemberRepo{
		members: make(map[string]*model.ProjectMember),
	}
}

func psProjectMemberKey(projectID, userID uint64) string {
	return fmt.Sprintf("%d:%d", projectID, userID)
}

// psCloneProjectMember 克隆项目成员，避免测试直接修改 mock 底层数据。
func psCloneProjectMember(pm *model.ProjectMember) *model.ProjectMember {
	if pm == nil {
		return nil
	}

	cp := *pm
	return &cp
}

func (m *psMockProjectMemberRepo) CreateWithTx(
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
		return errors.New("mock project member is nil")
	}

	cp := psCloneProjectMember(projectMember)
	m.members[psProjectMemberKey(cp.ProjectID, cp.UserID)] = cp

	return nil
}

func (m *psMockProjectMemberRepo) ListProjectIDsByUserID(ctx context.Context, userID uint64) ([]uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.listErr != nil {
		return nil, m.listErr
	}

	projectIDs := make([]uint64, 0)
	for _, pm := range m.members {
		if pm.UserID == userID {
			projectIDs = append(projectIDs, pm.ProjectID)
		}
	}

	sort.Slice(projectIDs, func(i, j int) bool {
		return projectIDs[i] < projectIDs[j]
	})

	return projectIDs, nil
}

func (m *psMockProjectMemberRepo) ExistsByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.existsErr != nil {
		return false, m.existsErr
	}

	_, ok := m.members[psProjectMemberKey(projectID, userID)]
	return ok, nil
}

func (m *psMockProjectMemberRepo) GetProjectMemberByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (*model.ProjectMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	pm, ok := m.members[psProjectMemberKey(projectID, userID)]
	if !ok {
		return nil, repository.ErrProjectMemberNotFound
	}

	return psCloneProjectMember(pm), nil
}

// psProjectServiceTestEnv 项目服务测试环境。
type psProjectServiceTestEnv struct {
	ctx         context.Context
	db          *gorm.DB
	txMgr       *repository.TxManager
	userRepo    *psMockUserRepo
	projectRepo *psMockProjectRepo
	memberRepo  *psMockProjectMemberRepo
	svc         *service.ProjectService
}

// psNewProjectServiceTestEnv 创建独立测试环境，避免测试之间数据污染。
func psNewProjectServiceTestEnv(t *testing.T) *psProjectServiceTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	oldLogger := zap.L()
	zap.ReplaceGlobals(logger)

	ctx := context.Background()

	// 使用 sqlite 内存数据库构造真实事务环境，不连接真实 MySQL。
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	require.NoError(t, err)

	txMgr := repository.NewTxManager(db)
	userRepo := psNewMockUserRepo()
	projectRepo := psNewMockProjectRepo(userRepo)
	memberRepo := psNewMockProjectMemberRepo()

	svc := service.NewProjectService(txMgr, userRepo, projectRepo, memberRepo)

	t.Cleanup(func() {
		zap.ReplaceGlobals(oldLogger)

		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return &psProjectServiceTestEnv{
		ctx:         ctx,
		db:          db,
		txMgr:       txMgr,
		userRepo:    userRepo,
		projectRepo: projectRepo,
		memberRepo:  memberRepo,
		svc:         svc,
	}
}

// psSeedUser 预置用户。
func (e *psProjectServiceTestEnv) psSeedUser(t *testing.T, username, nickname, avatar string) *model.User {
	t.Helper()

	e.userRepo.mu.Lock()
	defer e.userRepo.mu.Unlock()

	now := time.Now()
	u := &model.User{
		ID:        e.userRepo.nextID,
		Username:  username,
		Nickname:  nickname,
		Avatar:    avatar,
		Status:    model.UserStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	e.userRepo.users[u.ID] = u
	e.userRepo.nextID++

	t.Logf("[seed user] id=%d username=%s nickname=%s avatar=%s", u.ID, u.Username, u.Nickname, u.Avatar)

	return psCloneUser(u)
}

// psSeedProject 预置项目。
func (e *psProjectServiceTestEnv) psSeedProject(
	t *testing.T,
	owner *model.User,
	name string,
	description string,
	status string,
) *model.Project {
	t.Helper()

	e.projectRepo.mu.Lock()
	defer e.projectRepo.mu.Unlock()

	now := time.Now()
	startDate := time.Date(2026, 1, 1, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	endDate := time.Date(2026, 1, 31, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))

	p := &model.Project{
		ID:          e.projectRepo.nextID,
		Name:        name,
		Description: description,
		Status:      status,
		StartDate:   &startDate,
		EndDate:     &endDate,
		OwnerID:     owner.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	e.projectRepo.projects[p.ID] = p
	e.projectRepo.nextID++

	t.Logf("[seed project] id=%d name=%s status=%s owner_id=%d", p.ID, p.Name, p.Status, p.OwnerID)

	return psCloneProject(p)
}

// psSeedProjectMember 预置项目成员。
func (e *psProjectServiceTestEnv) psSeedProjectMember(
	t *testing.T,
	projectID uint64,
	userID uint64,
	role string,
) *model.ProjectMember {
	t.Helper()

	e.memberRepo.mu.Lock()
	defer e.memberRepo.mu.Unlock()

	pm := &model.ProjectMember{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
		JoinedAt:  time.Now(),
	}

	e.memberRepo.members[psProjectMemberKey(projectID, userID)] = pm

	t.Logf("[seed project member] project_id=%d user_id=%d role=%s", projectID, userID, role)

	return psCloneProjectMember(pm)
}

// psSeedFullProject 预置 owner/admin/member/project 完整项目环境。
func (e *psProjectServiceTestEnv) psSeedFullProject(
	t *testing.T,
) (*model.User, *model.User, *model.User, *model.Project) {
	t.Helper()

	owner := e.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)
	admin := e.psSeedUser(t, psTestAdminUsername, psTestAdminNickname, psTestAdminAvatar)
	member := e.psSeedUser(t, psTestMemberUsername, psTestMemberNickname, psTestMemberAvatar)

	project := e.psSeedProject(t, owner, psTestProjectName, psTestProjectDescription, model.ProjectStatusActive)

	e.psSeedProjectMember(t, project.ID, owner.ID, model.ProjectMemberRoleOwner)
	e.psSeedProjectMember(t, project.ID, admin.ID, model.ProjectMemberRoleAdmin)
	e.psSeedProjectMember(t, project.ID, member.ID, model.ProjectMemberRoleMember)

	return owner, admin, member, project
}

// psMustGetProject 获取 mock 仓库中的最新项目。
func (e *psProjectServiceTestEnv) psMustGetProject(t *testing.T, projectID uint64) *model.Project {
	t.Helper()

	p, err := e.projectRepo.GetDetailByID(e.ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, p)

	return p
}

func TestProjectServiceCreateProject(t *testing.T) {
	t.Run("invalid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)

		resp, err := env.svc.CreateProject(env.ctx, nil)
		t.Logf("[create project] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectParam)
		assert.Nil(t, resp)
	})

	t.Run("invalid project name", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID: owner.ID,
			Name:   "   ",
		})
		t.Logf("[create project] invalid name resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrEmptyProjectName)
		assert.Nil(t, resp)
	})

	t.Run("invalid start time", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:    owner.ID,
			Name:      psTestProjectName,
			StartTime: "invalid-time",
		})
		t.Logf("[create project] invalid start time resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTime)
		assert.Nil(t, resp)
	})

	t.Run("invalid end time", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:  owner.ID,
			Name:    psTestProjectName,
			EndTime: "invalid-time",
		})
		t.Logf("[create project] invalid end time resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTime)
		assert.Nil(t, resp)
	})

	t.Run("invalid time range", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:    owner.ID,
			Name:      psTestProjectName,
			StartTime: psTestEndTime,
			EndTime:   psTestStartTime,
		})
		t.Logf("[create project] invalid time range resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTimeRange)
		assert.Nil(t, resp)
	})

	t.Run("user not found", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:      9999,
			Name:        psTestProjectName,
			Description: psTestProjectDescription,
		})
		t.Logf("[create project] user not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrUserNotFound)
		assert.Nil(t, resp)
	})

	t.Run("user repo error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		env.userRepo.getByIDErr = errors.New("mock get user failed")

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:      1,
			Name:        psTestProjectName,
			Description: psTestProjectDescription,
		})
		t.Logf("[create project] user repo error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get user failed")
		assert.Nil(t, resp)
	})

	t.Run("project create tx error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)
		env.projectRepo.createErr = errors.New("mock create project failed")

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:      owner.ID,
			Name:        psTestProjectName,
			Description: psTestProjectDescription,
			StartTime:   psTestStartTime,
			EndTime:     psTestEndTime,
		})
		t.Logf("[create project] project create tx error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock create project failed")
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.createWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
	})

	t.Run("project member create tx error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)
		env.memberRepo.createErr = errors.New("mock create project member failed")

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:      owner.ID,
			Name:        psTestProjectName,
			Description: psTestProjectDescription,
			StartTime:   psTestStartTime,
			EndTime:     psTestEndTime,
		})
		t.Logf("[create project] member create tx error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock create project member failed")
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.createWithTxCalled)
		assert.True(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
		assert.False(t, env.memberRepo.lastTxIsNil)
	})

	t.Run("success all fields assert", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.CreateProject(env.ctx, &service.CreateProjectParams{
			UserID:      owner.ID,
			Name:        "  " + psTestProjectName + "  ",
			Description: "  " + psTestProjectDescription + "  ",
			StartTime:   psTestStartTime,
			EndTime:     psTestEndTime,
		})
		t.Logf("[create project] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.NotZero(t, resp.ID)
		assert.Equal(t, psTestProjectName, resp.Name)
		assert.Equal(t, psTestProjectDescription, resp.Description)
		assert.Equal(t, model.ProjectStatusActive, resp.Status)
		require.NotNil(t, resp.StartDate)
		require.NotNil(t, resp.EndDate)
		assert.False(t, resp.EndDate.Before(*resp.StartDate))
		assert.False(t, resp.CreatedAt.IsZero())
		assert.Equal(t, owner.ID, resp.OwnerID)

		require.NotNil(t, resp.Owner)
		assert.Equal(t, owner.ID, resp.Owner.ID)
		assert.Equal(t, owner.Username, resp.Owner.Username)
		assert.Equal(t, owner.Nickname, resp.Owner.Nickname)
		assert.Equal(t, owner.Avatar, resp.Owner.Avatar)

		assert.True(t, env.projectRepo.createWithTxCalled)
		assert.True(t, env.memberRepo.createWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
		assert.False(t, env.memberRepo.lastTxIsNil)

		pm, err := env.memberRepo.GetProjectMemberByProjectIDAndUserID(env.ctx, resp.ID, owner.ID)
		require.NoError(t, err)
		assert.Equal(t, resp.ID, pm.ProjectID)
		assert.Equal(t, owner.ID, pm.UserID)
		assert.Equal(t, model.ProjectMemberRoleOwner, pm.Role)
		assert.False(t, pm.JoinedAt.IsZero())
	})
}

func TestProjectServiceListProjects(t *testing.T) {
	t.Run("invalid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)

		resp, err := env.svc.ListProjects(env.ctx, nil)
		t.Logf("[list projects] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectParam)
		assert.Nil(t, resp)
	})

	t.Run("invalid status", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		user := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.ListProjects(env.ctx, &service.ListProjectsParam{
			UserID: user.ID,
			Status: "bad-status",
		})
		t.Logf("[list projects] invalid status resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectStatus)
		assert.Nil(t, resp)
	})

	t.Run("list visible project ids error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		user := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)
		env.memberRepo.listErr = errors.New("mock list project ids failed")

		resp, err := env.svc.ListProjects(env.ctx, &service.ListProjectsParam{
			UserID:   user.ID,
			Page:     1,
			PageSize: 10,
		})
		t.Logf("[list projects] list ids error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock list project ids failed")
		assert.Nil(t, resp)
	})

	t.Run("no visible projects", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		user := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)

		resp, err := env.svc.ListProjects(env.ctx, &service.ListProjectsParam{
			UserID:   user.ID,
			Page:     1,
			PageSize: 10,
		})
		t.Logf("[list projects] no visible projects resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.List)
		assert.Equal(t, 0, resp.Total)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
	})

	t.Run("search projects error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.projectRepo.searchErr = errors.New("mock search projects failed")

		resp, err := env.svc.ListProjects(env.ctx, &service.ListProjectsParam{
			UserID:   owner.ID,
			Page:     1,
			PageSize: 10,
			Keyword:  project.Name,
		})
		t.Logf("[list projects] search error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock search projects failed")
		assert.Nil(t, resp)
	})

	t.Run("success filter by keyword and status all fields assert", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.ListProjects(env.ctx, &service.ListProjectsParam{
			UserID:   owner.ID,
			Page:     1,
			PageSize: 10,
			Status:   model.ProjectStatusActive,
			Keyword:  "测试",
		})
		t.Logf("[list projects] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 1, resp.Total)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
		require.Len(t, resp.List, 1)

		item := resp.List[0]
		assert.Equal(t, project.ID, item.ID)
		assert.Equal(t, project.Name, item.Name)
		assert.Equal(t, project.Status, item.Status)
		assert.Equal(t, project.OwnerID, item.OwnerID)
		require.NotNil(t, item.StartDate)
		require.NotNil(t, item.EndDate)

		require.NotNil(t, item.Owner)
		assert.Equal(t, owner.ID, item.Owner.ID)
		assert.Equal(t, owner.Username, item.Owner.Username)
		assert.Equal(t, owner.Nickname, item.Owner.Nickname)
		assert.Equal(t, owner.Avatar, item.Owner.Avatar)
	})

	t.Run("success pagination empty result but total exists", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, _ := env.psSeedFullProject(t)

		resp, err := env.svc.ListProjects(env.ctx, &service.ListProjectsParam{
			UserID:   owner.ID,
			Page:     2,
			PageSize: 10,
			Status:   model.ProjectStatusActive,
		})
		t.Logf("[list projects] page overflow resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.List)
		assert.Equal(t, 1, resp.Total)
		assert.Equal(t, 2, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
	})
}

func TestProjectServiceGetProjectDetail(t *testing.T) {
	t.Run("invalid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)

		resp, err := env.svc.GetProjectDetail(env.ctx, nil)
		t.Logf("[get project detail] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectParam)
		assert.Nil(t, resp)
	})

	t.Run("exists check error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.memberRepo.existsErr = errors.New("mock exists project member failed")

		resp, err := env.svc.GetProjectDetail(env.ctx, &service.GetProjectDetailParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
		})
		t.Logf("[get project detail] exists error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock exists project member failed")
		assert.Nil(t, resp)
	})

	t.Run("project member not found", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		outsider := env.psSeedUser(t, "ps_test_outsider", "外部用户", "https://example.com/outside.png")
		_, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.GetProjectDetail(env.ctx, &service.GetProjectDetailParam{
			UserID:    outsider.ID,
			ProjectID: project.ID,
		})
		t.Logf("[get project detail] project member not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
	})

	t.Run("project not found after member exists", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner := env.psSeedUser(t, psTestOwnerUsername, psTestOwnerNickname, psTestOwnerAvatar)
		env.psSeedProjectMember(t, 9999, owner.ID, model.ProjectMemberRoleOwner)

		resp, err := env.svc.GetProjectDetail(env.ctx, &service.GetProjectDetailParam{
			UserID:    owner.ID,
			ProjectID: 9999,
		})
		t.Logf("[get project detail] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
	})

	t.Run("detail repo error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.projectRepo.detailErr = errors.New("mock get project detail failed")

		resp, err := env.svc.GetProjectDetail(env.ctx, &service.GetProjectDetailParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
		})
		t.Logf("[get project detail] detail repo error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get project detail failed")
		assert.Nil(t, resp)
	})

	t.Run("success all fields assert", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.GetProjectDetail(env.ctx, &service.GetProjectDetailParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
		})
		t.Logf("[get project detail] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, project.ID, resp.ID)
		assert.Equal(t, project.Name, resp.Name)
		assert.Equal(t, project.Description, resp.Description)
		assert.Equal(t, project.Status, resp.Status)
		assert.Equal(t, project.OwnerID, resp.OwnerID)
		assert.False(t, resp.CreatedAt.IsZero())
		assert.False(t, resp.UpdatedAt.IsZero())
		require.NotNil(t, resp.StartDate)
		require.NotNil(t, resp.EndDate)

		require.NotNil(t, resp.Owner)
		assert.Equal(t, owner.ID, resp.Owner.ID)
		assert.Equal(t, owner.Username, resp.Owner.Username)
		assert.Equal(t, owner.Nickname, resp.Owner.Nickname)
		assert.Equal(t, owner.Avatar, resp.Owner.Avatar)
	})
}

func TestProjectServiceUpdateProject(t *testing.T) {
	t.Run("invalid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)

		resp, err := env.svc.UpdateProject(env.ctx, nil)
		t.Logf("[update project] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectParam)
		assert.Nil(t, resp)
	})

	t.Run("valid project name handled as valid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
			Name:      "   ", // 允许通过，后台不改数据
		})
		t.Logf("[update project] valid project name resp=%+v err=%v", resp, err)
		assert.Equal(t, project.Name, resp.Name)
	})

	t.Run("invalid project name handled as invalid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
			Name:      "   @",
		})
		t.Logf("[update project] invalid project name resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectName)
		assert.Nil(t, resp)
	})

	t.Run("invalid status", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
			Name:      psTestProjectNewName,
			Status:    "bad-status",
		})
		t.Logf("[update project] invalid status resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectStatus)
		assert.Nil(t, resp)
	})

	t.Run("invalid start time", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
			Name:      psTestProjectNewName,
			StartTime: "invalid-time",
		})
		t.Logf("[update project] invalid start time resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTime)
		assert.Nil(t, resp)
	})

	t.Run("invalid end time", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
			Name:      psTestProjectNewName,
			EndTime:   "invalid-time",
		})
		t.Logf("[update project] invalid end time resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTime)
		assert.Nil(t, resp)
	})

	t.Run("invalid time range", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:    owner.ID,
			ProjectID: project.ID,
			Name:      psTestProjectNewName,
			StartTime: psTestNewEndTime,
			EndTime:   psTestNewStartTime,
		})
		t.Logf("[update project] invalid time range resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTimeRange)
		assert.Nil(t, resp)
	})

	t.Run("project member not found", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		outsider := env.psSeedUser(t, "ps_test_outsider", "外部用户", "https://example.com/outside.png")
		_, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      outsider.ID,
			ProjectID:   project.ID,
			Name:        psTestProjectNewName,
			Description: psTestProjectNewDesc,
		})
		t.Logf("[update project] member not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
	})

	t.Run("project member repo error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.memberRepo.getErr = errors.New("mock get project member failed")

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      owner.ID,
			ProjectID:   project.ID,
			Name:        psTestProjectNewName,
			Description: psTestProjectNewDesc,
		})
		t.Logf("[update project] member repo error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get project member failed")
		assert.Nil(t, resp)
	})

	t.Run("forbidden normal member", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		_, _, member, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      member.ID,
			ProjectID:   project.ID,
			Name:        psTestProjectNewName,
			Description: psTestProjectNewDesc,
		})
		t.Logf("[update project] forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
	})

	t.Run("update tx project not found", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		env.projectRepo.mu.Lock()
		delete(env.projectRepo.projects, project.ID)
		env.projectRepo.mu.Unlock()

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      owner.ID,
			ProjectID:   project.ID,
			Name:        psTestProjectNewName,
			Description: psTestProjectNewDesc,
			Status:      model.ProjectStatusActive,
		})
		t.Logf("[update project] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.updateWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
	})

	t.Run("update tx repo error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.projectRepo.updateErr = errors.New("mock update project failed")

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      owner.ID,
			ProjectID:   project.ID,
			Name:        psTestProjectNewName,
			Description: psTestProjectNewDesc,
			Status:      model.ProjectStatusActive,
		})
		t.Logf("[update project] update repo error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock update project failed")
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.updateWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
	})

	t.Run("get detail after update error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.projectRepo.detailErr = errors.New("mock get updated project failed")

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      owner.ID,
			ProjectID:   project.ID,
			Name:        psTestProjectNewName,
			Description: psTestProjectNewDesc,
			Status:      model.ProjectStatusActive,
		})
		t.Logf("[update project] get detail after update error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get updated project failed")
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.updateWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
	})

	t.Run("success by admin all fields assert", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, admin, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.UpdateProject(env.ctx, &service.UpdateProjectParam{
			UserID:      admin.ID,
			ProjectID:   project.ID,
			Name:        "  " + psTestProjectNewName + "  ",
			Description: "  " + psTestProjectNewDesc + "  ",
			Status:      model.ProjectStatusArchived,
			StartTime:   psTestNewStartTime,
			EndTime:     psTestNewEndTime,
		})
		t.Logf("[update project] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, project.ID, resp.ID)
		assert.Equal(t, psTestProjectNewName, resp.Name)
		assert.Equal(t, psTestProjectNewDesc, resp.Description)
		assert.Equal(t, model.ProjectStatusArchived, resp.Status)
		assert.Equal(t, owner.ID, resp.OwnerID)
		require.NotNil(t, resp.StartDate)
		require.NotNil(t, resp.EndDate)
		assert.False(t, resp.EndDate.Before(*resp.StartDate))
		assert.False(t, resp.UpdatedAt.IsZero())

		require.NotNil(t, resp.Owner)
		assert.Equal(t, owner.ID, resp.Owner.ID)
		assert.Equal(t, owner.Username, resp.Owner.Username)
		assert.Equal(t, owner.Nickname, resp.Owner.Nickname)
		assert.Equal(t, owner.Avatar, resp.Owner.Avatar)

		assert.True(t, env.projectRepo.updateWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)

		updated := env.psMustGetProject(t, project.ID)
		assert.Equal(t, psTestProjectNewName, updated.Name)
		assert.Equal(t, psTestProjectNewDesc, updated.Description)
		assert.Equal(t, model.ProjectStatusArchived, updated.Status)
		assert.Equal(t, owner.ID, updated.OwnerID)
	})
}

func TestProjectServiceArchiveProject(t *testing.T) {
	t.Run("invalid param", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)

		resp, err := env.svc.ArchiveProject(env.ctx, 0, 0)
		t.Logf("[archive project] invalid param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidProjectParam)
		assert.Nil(t, resp)
	})

	t.Run("project member not found", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		outsider := env.psSeedUser(t, "ps_test_outsider", "外部用户", "https://example.com/outside.png")
		_, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.ArchiveProject(env.ctx, outsider.ID, project.ID)
		t.Logf("[archive project] member not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectMemberNotFound)
		assert.Nil(t, resp)
	})

	t.Run("project member repo error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.memberRepo.getErr = errors.New("mock get project member failed")

		resp, err := env.svc.ArchiveProject(env.ctx, owner.ID, project.ID)
		t.Logf("[archive project] member repo error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock get project member failed")
		assert.Nil(t, resp)
	})

	t.Run("forbidden normal member", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		_, _, member, project := env.psSeedFullProject(t)

		resp, err := env.svc.ArchiveProject(env.ctx, member.ID, project.ID)
		t.Logf("[archive project] forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectForbidden)
		assert.Nil(t, resp)
	})

	t.Run("archive tx project not found", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		env.projectRepo.mu.Lock()
		delete(env.projectRepo.projects, project.ID)
		env.projectRepo.mu.Unlock()

		resp, err := env.svc.ArchiveProject(env.ctx, owner.ID, project.ID)
		t.Logf("[archive project] project not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrProjectNotFound)
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.archiveWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
	})

	t.Run("archive tx repo error", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)
		env.projectRepo.archiveErr = errors.New("mock archive project failed")

		resp, err := env.svc.ArchiveProject(env.ctx, owner.ID, project.ID)
		t.Logf("[archive project] archive repo error resp=%+v err=%v", resp, err)

		require.Error(t, err)
		assert.EqualError(t, err, "mock archive project failed")
		assert.Nil(t, resp)
		assert.True(t, env.projectRepo.archiveWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)
	})

	t.Run("success by owner all fields assert", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		owner, _, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.ArchiveProject(env.ctx, owner.ID, project.ID)
		t.Logf("[archive project] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, project.ID, resp.ID)
		assert.Equal(t, model.ProjectStatusArchived, resp.Status)

		assert.True(t, env.projectRepo.archiveWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)

		archived := env.psMustGetProject(t, project.ID)
		assert.Equal(t, model.ProjectStatusArchived, archived.Status)
	})

	t.Run("success by admin all fields assert", func(t *testing.T) {
		env := psNewProjectServiceTestEnv(t)
		_, admin, _, project := env.psSeedFullProject(t)

		resp, err := env.svc.ArchiveProject(env.ctx, admin.ID, project.ID)
		t.Logf("[archive project] success by admin resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, project.ID, resp.ID)
		assert.Equal(t, model.ProjectStatusArchived, resp.Status)

		assert.True(t, env.projectRepo.archiveWithTxCalled)
		assert.False(t, env.projectRepo.lastTxIsNil)

		archived := env.psMustGetProject(t, project.ID)
		assert.Equal(t, model.ProjectStatusArchived, archived.Status)
	})
}
