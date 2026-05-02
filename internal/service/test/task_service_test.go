package test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"smart-task-platform/internal/dto"
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
	taskTestProjectID      = uint64(10001)
	taskTestOtherProjectID = uint64(10002)

	taskTestOwnerID    = uint64(1)
	taskTestAdminID    = uint64(2)
	taskTestMemberID   = uint64(3)
	taskTestAssigneeID = uint64(4)
	taskTestOtherID    = uint64(5)

	taskTestTaskID      = uint64(101)
	taskTestOtherTaskID = uint64(102)
)

var errTaskRepoMock = errors.New("mock repository error")

// taskServiceTestEnv TaskService mock 测试环境。
type taskServiceTestEnv struct {
	ctx context.Context

	db          *gorm.DB
	txMgr       *repository.TxManager
	userRepo    *taskMockUserRepo
	projectRepo *taskMockProjectRepo
	memberRepo  *taskMockProjectMemberRepo
	taskRepo    *taskMockTaskRepo
	svc         *service.TaskService
}

// taskNewServiceTestEnv 创建 TaskService 独立测试环境。
func taskNewServiceTestEnv(t *testing.T) *taskServiceTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	oldLogger := zap.L()
	zap.ReplaceGlobals(logger)

	// 使用 sqlite 内存数据库，只用于构造事务，不连接真实数据库。
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	require.NoError(t, err)

	ctx := context.Background()
	txMgr := repository.NewTxManager(db)

	userRepo := taskNewMockUserRepo()
	projectRepo := taskNewMockProjectRepo()
	memberRepo := taskNewMockProjectMemberRepo()
	taskRepo := taskNewMockTaskRepo()

	svc := service.NewTaskService(
		txMgr,
		userRepo,
		projectRepo,
		memberRepo,
		taskRepo,
	)

	env := &taskServiceTestEnv{
		ctx:         ctx,
		db:          db,
		txMgr:       txMgr,
		userRepo:    userRepo,
		projectRepo: projectRepo,
		memberRepo:  memberRepo,
		taskRepo:    taskRepo,
		svc:         svc,
	}

	env.taskSeedDefaultData(t)

	t.Cleanup(func() {
		zap.ReplaceGlobals(oldLogger)

		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	return env
}

// taskSeedDefaultData 初始化默认用户、项目、成员、任务数据。
func (e *taskServiceTestEnv) taskSeedDefaultData(t *testing.T) {
	t.Helper()

	e.projectRepo.projects[taskTestProjectID] = true
	e.projectRepo.projects[taskTestOtherProjectID] = true

	e.userRepo.users[taskTestOwnerID] = taskBuildTestUser(taskTestOwnerID, "owner", "项目拥有者")
	e.userRepo.users[taskTestAdminID] = taskBuildTestUser(taskTestAdminID, "admin", "项目管理员")
	e.userRepo.users[taskTestMemberID] = taskBuildTestUser(taskTestMemberID, "member", "普通成员")
	e.userRepo.users[taskTestAssigneeID] = taskBuildTestUser(taskTestAssigneeID, "assignee", "任务负责人")
	e.userRepo.users[taskTestOtherID] = taskBuildTestUser(taskTestOtherID, "other", "其他用户")

	e.memberRepo.members[taskMemberKey(taskTestProjectID, taskTestOwnerID)] = model.ProjectMemberRoleOwner
	e.memberRepo.members[taskMemberKey(taskTestProjectID, taskTestAdminID)] = model.ProjectMemberRoleAdmin
	e.memberRepo.members[taskMemberKey(taskTestProjectID, taskTestMemberID)] = model.ProjectMemberRoleMember
	e.memberRepo.members[taskMemberKey(taskTestProjectID, taskTestAssigneeID)] = model.ProjectMemberRoleMember

	e.memberRepo.members[taskMemberKey(taskTestOtherProjectID, taskTestOwnerID)] = model.ProjectMemberRoleOwner
	e.memberRepo.members[taskMemberKey(taskTestOtherProjectID, taskTestAssigneeID)] = model.ProjectMemberRoleMember

	e.taskRepo.tasks[taskTestTaskID] = taskBuildTestTask(
		taskTestTaskID,
		taskTestProjectID,
		taskTestMemberID,
		taskUint64Ptr(taskTestAssigneeID),
		"todo",
		"high",
		"任务-101",
	)

	e.taskRepo.tasks[taskTestOtherTaskID] = taskBuildTestTask(
		taskTestOtherTaskID,
		taskTestOtherProjectID,
		taskTestOwnerID,
		taskUint64Ptr(taskTestAssigneeID),
		"in_progress",
		"medium",
		"任务-102",
	)
}

func TestTaskService_CreateTask(t *testing.T) {
	t.Run("success_create_without_assignee", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID:   taskTestMemberID,
			ProjectID:   taskTestProjectID,
			Title:       "新任务",
			Description: "任务描述",
			Priority:    "high",
			AssigneeID:  0,
			DueDate:     "",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastCreatedTask)

		assert.Equal(t, taskTestProjectID, env.taskRepo.lastCreatedTask.ProjectID)
		assert.Equal(t, taskTestMemberID, env.taskRepo.lastCreatedTask.CreatorID)
		assert.Equal(t, "新任务", env.taskRepo.lastCreatedTask.Title)
		assert.Equal(t, "high", env.taskRepo.lastCreatedTask.Priority)
		assert.Nil(t, env.taskRepo.lastCreatedTask.AssigneeID)
	})

	t.Run("success_create_with_assignee", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID:   taskTestMemberID,
			ProjectID:   taskTestProjectID,
			Title:       "新任务",
			Description: "任务描述",
			Priority:    "medium",
			AssigneeID:  taskTestAssigneeID,
			DueDate:     "",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastCreatedTask)
		require.NotNil(t, env.taskRepo.lastCreatedTask.AssigneeID)

		assert.Equal(t, taskTestAssigneeID, *env.taskRepo.lastCreatedTask.AssigneeID)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_creator_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: 0,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_project_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: 0,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_title", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrEmptyTaskTitle)
	})

	t.Run("invalid_priority", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "bad_priority",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskPriority)
	})

	t.Run("project_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.projectRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("project_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.projectRepo.projects, taskTestProjectID)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrProjectNotFound)
	})

	t.Run("creator_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.userRepo.getErr = errTaskRepoMock

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("creator_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.userRepo.users, taskTestMemberID)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrUserNotFound)
	})

	t.Run("creator_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestMemberID))

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrProjectForbidden)
	})

	t.Run("assignee_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.userRepo.users, taskTestAssigneeID)

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID:  taskTestMemberID,
			ProjectID:  taskTestProjectID,
			Title:      "新任务",
			Priority:   "high",
			AssigneeID: taskTestAssigneeID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrUserNotFound)
	})

	t.Run("assignee_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestAssigneeID))

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID:  taskTestMemberID,
			ProjectID:  taskTestProjectID,
			Title:      "新任务",
			Priority:   "high",
			AssigneeID: taskTestAssigneeID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrAssigneeNotProjectMember)
	})

	t.Run("create_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.createErr = errTaskRepoMock

		resp, err := env.svc.CreateTask(env.ctx, &service.CreateTaskParam{
			CreatorID: taskTestMemberID,
			ProjectID: taskTestProjectID,
			Title:     "新任务",
			Priority:  "high",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

func TestTaskService_RemoveTask(t *testing.T) {
	t.Run("success_creator_remove_task", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.taskRepo.deletedTaskIDs[taskTestTaskID])
		assert.Equal(t, taskTestTaskID, env.taskRepo.lastSoftDeleteTaskID)
	})

	t.Run("success_owner_remove_task", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestOwnerID,
			TaskID: taskTestTaskID,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.taskRepo.deletedTaskIDs[taskTestTaskID])
		assert.Equal(t, taskTestTaskID, env.taskRepo.lastSoftDeleteTaskID)
	})

	t.Run("success_admin_remove_task", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestAdminID,
			TaskID: taskTestTaskID,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.taskRepo.deletedTaskIDs[taskTestTaskID])
		assert.Equal(t, taskTestTaskID, env.taskRepo.lastSoftDeleteTaskID)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: 0,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_task_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestOwnerID,
			TaskID: 0,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("get_task_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.getTaskErr = errTaskRepoMock

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestOwnerID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("task_not_found_when_get_task", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestOwnerID,
			TaskID: 999999,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)
	})

	t.Run("permission_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.memberRepo.getRoleErr = errTaskRepoMock

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestAdminID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("forbidden_when_user_is_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestOtherID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
		assert.False(t, env.taskRepo.deletedTaskIDs[taskTestTaskID])
	})

	t.Run("forbidden_when_normal_member_is_not_creator", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
		assert.False(t, env.taskRepo.deletedTaskIDs[taskTestTaskID])
	})

	t.Run("soft_delete_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.softDeleteErr = errTaskRepoMock

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("task_not_found_when_soft_delete", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.softDeleteErr = repository.ErrTaskNotFound

		resp, err := env.svc.RemoveTask(env.ctx, &service.RemoveTaskParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)
	})
}

func TestTaskService_ListUserTasks(t *testing.T) {
	t.Run("success_project_id_zero_query_all_assigned_tasks", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: 0,
			Page:      1,
			PageSize:  10,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastSearchQuery)

		assert.Equal(t, 2, resp.Total)
		assert.Len(t, resp.List, 2)
		assert.Equal(t, uint64(0), env.taskRepo.lastSearchQuery.ProjectID)
		require.NotNil(t, env.taskRepo.lastSearchQuery.AssigneeID)
		assert.Equal(t, taskTestAssigneeID, *env.taskRepo.lastSearchQuery.AssigneeID)
	})

	t.Run("success_filter_by_project_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: taskTestProjectID,
			Page:      1,
			PageSize:  10,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastSearchQuery)

		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.List, 1)
		assert.Equal(t, taskTestProjectID, env.taskRepo.lastSearchQuery.ProjectID)
		require.NotNil(t, env.taskRepo.lastSearchQuery.AssigneeID)
		assert.Equal(t, taskTestAssigneeID, *env.taskRepo.lastSearchQuery.AssigneeID)
	})

	t.Run("success_filter_status_priority_keyword", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: 0,
			Page:      1,
			PageSize:  10,
			Status:    "todo",
			Priority:  "high",
			Keyword:   "任务-101",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.List, 1)
	})

	t.Run("success_empty_result", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: 0,
			Page:      1,
			PageSize:  10,
			Keyword:   "不存在的任务",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 0, resp.Total)
		assert.Len(t, resp.List, 0)
	})

	t.Run("success_page_fallback", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:   taskTestAssigneeID,
			Page:     0,
			PageSize: 0,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.GreaterOrEqual(t, resp.Page, 1)
		assert.Greater(t, resp.PageSize, 0)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID: 0,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_status", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID: taskTestAssigneeID,
			Status: "bad_status",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskStatus)
	})

	t.Run("invalid_priority", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:   taskTestAssigneeID,
			Priority: "bad_priority",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskPriority)
	})

	t.Run("invalid_sort_by", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID: taskTestAssigneeID,
			SortBy: "bad_sort_by",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskSortBy)
	})

	t.Run("invalid_sort_order", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			SortOrder: "bad_sort_order",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskSortOrder)
	})

	t.Run("user_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.userRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID: taskTestAssigneeID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("user_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.userRepo.users, taskTestAssigneeID)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID: taskTestAssigneeID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrUserNotFound)
	})

	t.Run("project_exists_repo_error_when_project_id_not_zero", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.projectRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("project_not_found_when_project_id_not_zero", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.projectRepo.projects, taskTestProjectID)

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrProjectNotFound)
	})

	t.Run("project_member_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.memberRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("project_forbidden_when_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestAssigneeID))

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID:    taskTestAssigneeID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrProjectForbidden)
	})

	t.Run("search_tasks_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.searchErr = errTaskRepoMock

		resp, err := env.svc.ListUserTasks(env.ctx, &service.ListUserTasksParam{
			UserID: taskTestAssigneeID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

func TestTaskService_ListProjectTasks(t *testing.T) {
	t.Run("success_list_all_project_tasks", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			Page:      1,
			PageSize:  10,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastSearchQuery)

		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.List, 1)
		assert.Equal(t, taskTestProjectID, env.taskRepo.lastSearchQuery.ProjectID)
		assert.Nil(t, env.taskRepo.lastSearchQuery.AssigneeID)
	})

	t.Run("success_filter_assignee", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:     taskTestMemberID,
			ProjectID:  taskTestProjectID,
			Page:       1,
			PageSize:   10,
			AssigneeID: taskUint64Ptr(taskTestAssigneeID),
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastSearchQuery)

		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.List, 1)
		require.NotNil(t, env.taskRepo.lastSearchQuery.AssigneeID)
		assert.Equal(t, taskTestAssigneeID, *env.taskRepo.lastSearchQuery.AssigneeID)
	})

	t.Run("success_filter_unassigned_tasks", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		env.taskRepo.tasks[103] = taskBuildTestTask(
			103,
			taskTestProjectID,
			taskTestMemberID,
			nil,
			"todo",
			"low",
			"未分配任务",
		)
		// 种子创建了一个/已分配的

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:     taskTestMemberID,
			ProjectID:  taskTestProjectID,
			Page:       1,
			PageSize:   10,
			AssigneeID: taskUint64Ptr(0), // 查询没有分配的任务
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, env.taskRepo.lastSearchQuery)

		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.List, 1)
		require.NotNil(t, env.taskRepo.lastSearchQuery.AssigneeID)
		assert.Equal(t, uint64(0), *env.taskRepo.lastSearchQuery.AssigneeID)
	})

	t.Run("success_filter_status_priority_keyword", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			Page:      1,
			PageSize:  10,
			Status:    "todo",
			Priority:  "high",
			Keyword:   "任务-101",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.List, 1)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    0,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_project_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: 0,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_status", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			Status:    "bad_status",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskStatus)
	})

	t.Run("invalid_priority", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			Priority:  "bad_priority",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskPriority)
	})

	t.Run("invalid_sort_by", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			SortBy:    "bad_sort_by",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskSortBy)
	})

	t.Run("invalid_sort_order", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			SortOrder: "bad_sort_order",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskSortOrder)
	})

	t.Run("project_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.projectRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("project_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.projectRepo.projects, taskTestProjectID)

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrProjectNotFound)
	})

	t.Run("member_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.memberRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("user_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestMemberID))

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrProjectForbidden)
	})

	t.Run("assignee_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestAssigneeID))

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:     taskTestMemberID,
			ProjectID:  taskTestProjectID,
			AssigneeID: taskUint64Ptr(taskTestAssigneeID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrAssigneeNotProjectMember)
	})

	t.Run("search_tasks_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.searchErr = errTaskRepoMock

		resp, err := env.svc.ListProjectTasks(env.ctx, &service.ListProjectTasksParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

func TestTaskService_GetTaskDetail(t *testing.T) {
	t.Run("success_project_member_get_detail", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, taskTestTaskID, resp.ID)
		assert.Equal(t, taskTestProjectID, resp.ProjectID)
		assert.Equal(t, taskTestMemberID, resp.CreatorID)
	})

	t.Run("success_assignee_get_detail_even_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestAssigneeID))

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, taskTestTaskID, resp.ID)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.GetTaskDetail(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: 0,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_task_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestMemberID,
			TaskID: 0,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("get_task_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.getTaskErr = errTaskRepoMock

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("task_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestMemberID,
			TaskID: 999999,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)
	})

	t.Run("member_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.memberRepo.existsErr = errTaskRepoMock

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("forbidden_when_not_member_and_not_assignee", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.GetTaskDetail(env.ctx, &service.GetTaskDetailParam{
			UserID: taskTestOtherID,
			TaskID: taskTestTaskID,
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
	})
}

func TestTaskService_UpdateTaskBaseInfo(t *testing.T) {
	t.Run("success_update_title_priority_description_due_date", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID:      taskTestMemberID,
			TaskID:      taskTestTaskID,
			Title:       "更新后的标题",
			Priority:    "medium",
			Description: taskStringPtr("更新后的描述"),
			DueDate:     taskStringPtr("2024-06-01T12:00:00Z"),
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		assert.Equal(t, "更新后的标题", task.Title)
		assert.Equal(t, "medium", task.Priority)
		require.NotNil(t, task.Description)
		assert.Equal(t, "更新后的描述", *task.Description)
		require.NotNil(t, task.DueDate)
		assert.Equal(t, taskTestTaskID, env.taskRepo.lastUpdateTaskID)
	})

	t.Run("success_clear_description_and_due_date", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		desc := "原描述"
		dueDate := time.Now().Add(24 * time.Hour)
		env.taskRepo.tasks[taskTestTaskID].Description = &desc
		env.taskRepo.tasks[taskTestTaskID].DueDate = &dueDate

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID:      taskTestMemberID,
			TaskID:      taskTestTaskID,
			Description: taskStringPtr(""),
			DueDate:     taskStringPtr(""),
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		assert.Nil(t, task.Description)
		assert.Nil(t, task.DueDate)
	})

	t.Run("success_no_change_direct_return", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, uint64(0), env.taskRepo.lastUpdateTaskID)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: 0,
			TaskID: taskTestTaskID,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_task_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestMemberID,
			TaskID: 0,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_priority", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID:   taskTestMemberID,
			TaskID:   taskTestTaskID,
			Priority: "bad_priority",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskPriority)
	})

	t.Run("get_task_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.getTaskErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("task_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestMemberID,
			TaskID: 999999,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)
	})

	t.Run("permission_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.memberRepo.getRoleErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestAdminID,
			TaskID: taskTestTaskID,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("forbidden_normal_member_not_creator", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
	})

	t.Run("update_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.updateErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskBaseInfo(env.ctx, &service.UpdateTaskBaseInfoParam{
			UserID: taskTestMemberID,
			TaskID: taskTestTaskID,
			Title:  "标题",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

func TestTaskService_UpdateTaskStatus(t *testing.T) {
	t.Run("success_update_status_to_in_progress", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Status: "in_progress",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		assert.Equal(t, "in_progress", task.Status)
		assert.Nil(t, task.CompletedAt)
		assert.Equal(t, taskTestTaskID, env.taskRepo.lastUpdateTaskID)
	})

	t.Run("success_update_status_to_done_set_completed_at", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Status: "done",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		assert.Equal(t, "done", task.Status)
		assert.NotNil(t, task.CompletedAt)
	})

	t.Run("success_update_done_to_todo_clear_completed_at", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		now := time.Now()
		env.taskRepo.tasks[taskTestTaskID].Status = "done"
		env.taskRepo.tasks[taskTestTaskID].CompletedAt = &now

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Status: "todo",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		assert.Equal(t, "todo", task.Status)
		assert.Nil(t, task.CompletedAt)
	})

	t.Run("success_empty_status_direct_return", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Status: "",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, uint64(0), env.taskRepo.lastUpdateTaskID)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: 0,
			TaskID: taskTestTaskID,
			Status: "done",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_task_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: 0,
			Status: "done",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_status", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Status: "bad_status",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskStatus)
	})

	t.Run("forbidden_user_not_assignee_or_manager_or_creator", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestOtherID,
			TaskID: taskTestTaskID,
			Status: "done",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
	})

	t.Run("update_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.updateErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskStatus(env.ctx, &service.UpdateTaskStatusParam{
			UserID: taskTestAssigneeID,
			TaskID: taskTestTaskID,
			Status: "done",
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

func TestTaskService_UpdateTaskAssignee(t *testing.T) {
	t.Run("success_update_assignee", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		require.NotNil(t, task.AssigneeID)
		assert.Equal(t, taskTestMemberID, *task.AssigneeID)
		assert.Equal(t, taskTestTaskID, env.taskRepo.lastUpdateTaskID)
	})

	t.Run("success_clear_assignee", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: nil,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		task := env.taskRepo.tasks[taskTestTaskID]
		assert.Nil(t, task.AssigneeID)
		assert.Nil(t, task.Assignee)
	})

	t.Run("success_same_assignee_direct_return", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestAssigneeID),
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, uint64(0), env.taskRepo.lastUpdateTaskID)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     0,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_task_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     0,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_assignee_id_zero", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(0),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("assignee_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.userRepo.users, taskTestMemberID)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrUserNotFound)
	})

	t.Run("assignee_not_project_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		delete(env.memberRepo.members, taskMemberKey(taskTestProjectID, taskTestMemberID))

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrAssigneeNotProjectMember)
	})

	t.Run("forbidden_normal_member_not_creator_or_manager", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestAssigneeID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
	})

	t.Run("update_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.updateErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskAssignee(env.ctx, &service.UpdateTaskAssigneeParam{
			UserID:     taskTestOwnerID,
			TaskID:     taskTestTaskID,
			AssigneeID: taskUint64Ptr(taskTestMemberID),
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

func TestTaskService_UpdateTaskSortOrder(t *testing.T) {
	t.Run("success_update_sort_order", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		env.taskRepo.tasks[103] = taskBuildTestTask(
			103,
			taskTestProjectID,
			taskTestMemberID,
			taskUint64Ptr(taskTestAssigneeID),
			"todo",
			"low",
			"任务-103",
		)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 20},
				{TaskID: 103, SortOrder: 10},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int(20), env.taskRepo.tasks[taskTestTaskID].SortOrder)
		assert.Equal(t, int(10), env.taskRepo.tasks[103].SortOrder)
	})

	t.Run("success_duplicate_task_id_keep_last", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
				{TaskID: taskTestTaskID, SortOrder: 99},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int(99), env.taskRepo.tasks[taskTestTaskID].SortOrder)
	})

	t.Run("nil_param", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, nil)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_user_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    0,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_project_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: 0,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("empty_items", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items:     []*dto.TaskSortItem{},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrEmptyTaskSortItems)
	})

	t.Run("nil_item", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				nil,
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("invalid_task_id", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: 0, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskParam)
	})

	t.Run("project_exists_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.countErr = errTaskRepoMock // 这个接口不查询 project 是否存在 通过任务接口就能顺便找到

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("project_not_found", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		// delete(env.projectRepo.projects, taskTestProjectID)
		env.taskRepo.countErr = errTaskRepoMock // 这个接口不查询 project 是否存在 通过任务接口就能顺便找到

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("forbidden_normal_member", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestMemberID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrTaskForbidden)
	})

	t.Run("count_tasks_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.countErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})

	t.Run("task_not_belong_to_project", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestOtherTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrInvalidTaskSortOrderItem)
	})

	t.Run("batch_update_repo_error", func(t *testing.T) {
		env := taskNewServiceTestEnv(t)
		env.taskRepo.batchUpdateErr = errTaskRepoMock

		resp, err := env.svc.UpdateTaskSortOrder(env.ctx, &service.UpdateTaskSortOrderParam{
			UserID:    taskTestOwnerID,
			ProjectID: taskTestProjectID,
			Items: []*dto.TaskSortItem{
				{TaskID: taskTestTaskID, SortOrder: 10},
			},
		})

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, errTaskRepoMock)
	})
}

// taskStringPtr 构造 string 指针。
func taskStringPtr(v string) *string {
	return &v
}

// 请将原 mock 中的 UpdateTaskByIDWithTx 替换为下面这个版本。
// 该版本会真实更新 mock 内存任务，便于断言 UpdateTaskBaseInfo / UpdateTaskStatus / UpdateTaskAssignee 的结果。
func (r *taskMockTaskRepo) UpdateTaskByIDWithTx(ctx context.Context, tx *gorm.DB, taskID uint64, param *repository.UpdateTaskParam) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.updateErr != nil {
		return r.updateErr
	}

	if taskID == 0 || param == nil {
		return repository.ErrInvalidTaskParam
	}

	task, ok := r.tasks[taskID]
	if !ok || task == nil || r.deletedTaskIDs[taskID] {
		return repository.ErrTaskNotFound
	}

	r.lastUpdateTaskID = taskID
	taskApplyUpdateTaskParam(task, param)
	task.UpdatedAt = time.Now()

	return nil
}

// taskApplyUpdateTaskParam 使用反射兼容 repository.UpdateTaskParam 字段变化。
func taskApplyUpdateTaskParam(task *model.Task, param *repository.UpdateTaskParam) {
	if task == nil || param == nil {
		return
	}

	rv := reflect.ValueOf(param)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return
	}

	// 更新标题
	if taskReflectBool(rv, "UpdateTitle") || taskReflectFieldExists(rv, "Title") {
		if v, ok := taskReflectString(rv, "Title"); ok && v != "" {
			task.Title = v
		}
	}

	// 更新优先级
	if taskReflectBool(rv, "UpdatePriority") || taskReflectFieldExists(rv, "Priority") {
		if v, ok := taskReflectString(rv, "Priority"); ok && v != "" {
			task.Priority = v
		}
	}

	// 更新状态
	if taskReflectBool(rv, "UpdateStatus") || taskReflectFieldExists(rv, "Status") {
		if v, ok := taskReflectString(rv, "Status"); ok && v != "" {
			task.Status = v
		}
	}

	// 更新描述：nil 表示清空，非 nil 表示更新
	if taskReflectBool(rv, "UpdateDescription") || taskReflectFieldExists(rv, "Description") {
		if v, ok := taskReflectStringPtr(rv, "Description"); ok {
			task.Description = v
		}
	}

	// 更新截止时间：nil 表示清空，非 nil 表示更新
	if taskReflectBool(rv, "UpdateDueDate") || taskReflectFieldExists(rv, "DueDate") {
		if v, ok := taskReflectTimePtr(rv, "DueDate"); ok {
			task.DueDate = v
		}
	}

	// 更新开始时间
	if taskReflectBool(rv, "UpdateStartTime") || taskReflectFieldExists(rv, "StartTime") {
		if v, ok := taskReflectTimePtr(rv, "StartTime"); ok {
			task.StartTime = v
		}
	}

	// 更新完成时间：nil 表示清空，非 nil 表示更新
	if taskReflectBool(rv, "UpdateCompletedAt") || taskReflectFieldExists(rv, "CompletedAt") {
		if v, ok := taskReflectTimePtr(rv, "CompletedAt"); ok {
			task.CompletedAt = v
		}
	}

	// 更新负责人：nil 表示清空，非 nil 表示更新
	if taskReflectBool(rv, "UpdateAssignee") || taskReflectFieldExists(rv, "AssigneeID") {
		if v, ok := taskReflectUint64Ptr(rv, "AssigneeID"); ok {
			task.AssigneeID = v
			if v == nil {
				task.Assignee = nil
			} else {
				task.Assignee = taskBuildTestUser(*v, fmt.Sprintf("assignee_%d", *v), "负责人")
			}
		}
	}

	// 更新排序值
	if taskReflectBool(rv, "UpdateSortOrder") || taskReflectFieldExists(rv, "SortOrder") {
		if v, ok := taskReflectInt64(rv, "SortOrder"); ok {
			task.SortOrder = int(v)
		}
	}
}

func taskReflectFieldExists(rv reflect.Value, fieldName string) bool {
	return rv.FieldByName(fieldName).IsValid()
}

func taskReflectBool(rv reflect.Value, fieldName string) bool {
	field := rv.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}

	return field.Bool()
}

func taskReflectString(rv reflect.Value, fieldName string) (string, bool) {
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return "", false
	}

	if field.Kind() == reflect.String {
		return field.String(), true
	}

	if field.Kind() == reflect.Ptr && !field.IsNil() && field.Elem().Kind() == reflect.String {
		return field.Elem().String(), true
	}

	return "", false
}

func taskReflectStringPtr(rv reflect.Value, fieldName string) (*string, bool) {
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, false
	}

	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return nil, true
		}

		if field.Elem().Kind() == reflect.String {
			v := field.Elem().String()
			return &v, true
		}
	}

	if field.Kind() == reflect.String {
		v := field.String()
		return &v, true
	}

	return nil, false
}

func taskReflectUint64Ptr(rv reflect.Value, fieldName string) (*uint64, bool) {
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, false
	}

	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return nil, true
		}

		if field.Elem().Kind() == reflect.Uint64 {
			v := uint64(field.Elem().Uint())
			return &v, true
		}
	}

	if field.Kind() == reflect.Uint64 {
		v := uint64(field.Uint())
		return &v, true
	}

	return nil, false
}

func taskReflectTimePtr(rv reflect.Value, fieldName string) (*time.Time, bool) {
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, false
	}

	timeType := reflect.TypeOf(time.Time{})

	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return nil, true
		}

		if field.Elem().Type() == timeType {
			v := field.Elem().Interface().(time.Time)
			return &v, true
		}
	}

	if field.Type() == timeType {
		v := field.Interface().(time.Time)
		return &v, true
	}

	return nil, false
}

func taskReflectInt64(rv reflect.Value, fieldName string) (int64, bool) {
	field := rv.FieldByName(fieldName)
	if !field.IsValid() {
		return 0, false
	}

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(field.Uint()), true
	default:
		return 0, false
	}
}

// taskBuildTestUser 构造测试用户。
func taskBuildTestUser(id uint64, username, nickname string) *model.User {
	return &model.User{
		ID:       id,
		Username: username,
		Email:    fmt.Sprintf("%s@example.com", username),
		Nickname: nickname,
		Avatar:   fmt.Sprintf("https://example.com/avatar/%d.png", id),
	}
}

// taskBuildTestProject 构造测试项目。
func taskBuildTestProject(id uint64) *model.Project {
	return &model.Project{
		ID:        id,
		Name:      fmt.Sprintf("测试项目-%d", id),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// taskBuildTestTask 构造测试任务。
func taskBuildTestTask(
	id uint64,
	projectID uint64,
	creatorID uint64,
	assigneeID *uint64,
	status string,
	priority string,
	title string,
) *model.Task {
	now := time.Now()

	task := &model.Task{
		ID:         id,
		ProjectID:  projectID,
		CreatorID:  creatorID,
		AssigneeID: assigneeID,
		Title:      title,
		Status:     status,
		Priority:   priority,
		SortOrder:  int(id),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	task.Project = taskBuildTestProject(projectID)
	task.Creator = taskBuildTestUser(creatorID, fmt.Sprintf("creator_%d", creatorID), "创建人")

	if assigneeID != nil {
		task.Assignee = taskBuildTestUser(*assigneeID, fmt.Sprintf("assignee_%d", *assigneeID), "负责人")
	}

	return task
}

// taskMemberKey 构造成员关系 key。
func taskMemberKey(projectID, userID uint64) string {
	return fmt.Sprintf("%d:%d", projectID, userID)
}

// taskUint64Ptr 构造 uint64 指针。
func taskUint64Ptr(v uint64) *uint64 {
	return &v
}

// taskCloneUser 克隆用户。
func taskCloneUser(u *model.User) *model.User {
	if u == nil {
		return nil
	}

	cp := *u
	return &cp
}

// taskCloneProject 克隆项目。
func taskCloneProject(p *model.Project) *model.Project {
	if p == nil {
		return nil
	}

	cp := *p
	return &cp
}

// taskCloneTask 克隆任务。
func taskCloneTask(task *model.Task) *model.Task {
	if task == nil {
		return nil
	}

	cp := *task

	if task.Description != nil {
		v := *task.Description
		cp.Description = &v
	}

	if task.AssigneeID != nil {
		v := *task.AssigneeID
		cp.AssigneeID = &v
	}

	if task.DueDate != nil {
		v := *task.DueDate
		cp.DueDate = &v
	}

	if task.StartTime != nil {
		v := *task.StartTime
		cp.StartTime = &v
	}

	if task.CompletedAt != nil {
		v := *task.CompletedAt
		cp.CompletedAt = &v
	}

	cp.Project = taskCloneProject(task.Project)
	cp.Creator = taskCloneUser(task.Creator)
	cp.Assignee = taskCloneUser(task.Assignee)

	return &cp
}

// taskCloneSearchQuery 克隆查询参数。
func taskCloneSearchQuery(query *repository.TaskSearchQuery) *repository.TaskSearchQuery {
	if query == nil {
		return nil
	}

	cp := *query
	if query.AssigneeID != nil {
		v := *query.AssigneeID
		cp.AssigneeID = &v
	}

	return &cp
}

// taskMockUserRepo 用户仓储 mock。
type taskMockUserRepo struct {
	mu sync.RWMutex

	users map[uint64]*model.User

	getErr    error
	existsErr error
}

func taskNewMockUserRepo() *taskMockUserRepo {
	return &taskMockUserRepo{
		users: make(map[uint64]*model.User),
	}
}

func (r *taskMockUserRepo) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.getErr != nil {
		return nil, r.getErr
	}

	user, ok := r.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}

	return taskCloneUser(user), nil
}

func (r *taskMockUserRepo) ExistsByUserID(ctx context.Context, userID uint64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.existsErr != nil {
		return false, r.existsErr
	}

	_, ok := r.users[userID]
	return ok, nil
}

// taskMockProjectRepo 项目仓储 mock。
type taskMockProjectRepo struct {
	mu sync.RWMutex

	projects map[uint64]bool

	existsErr error
}

func taskNewMockProjectRepo() *taskMockProjectRepo {
	return &taskMockProjectRepo{
		projects: make(map[uint64]bool),
	}
}

func (r *taskMockProjectRepo) ExistsByProjectID(ctx context.Context, projectID uint64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.existsErr != nil {
		return false, r.existsErr
	}

	return r.projects[projectID], nil
}

// taskMockProjectMemberRepo 项目成员仓储 mock。
type taskMockProjectMemberRepo struct {
	mu sync.RWMutex

	members map[string]string

	existsErr  error
	getRoleErr error
}

func taskNewMockProjectMemberRepo() *taskMockProjectMemberRepo {
	return &taskMockProjectMemberRepo{
		members: make(map[string]string),
	}
}

func (r *taskMockProjectMemberRepo) ExistsByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.existsErr != nil {
		return false, r.existsErr
	}

	_, ok := r.members[taskMemberKey(projectID, userID)]
	return ok, nil
}

func (r *taskMockProjectMemberRepo) GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.getRoleErr != nil {
		return "", r.getRoleErr
	}

	role, ok := r.members[taskMemberKey(projectID, userID)]
	if !ok {
		return "", nil
	}

	return role, nil
}

// taskMockTaskRepo 任务仓储 mock。
type taskMockTaskRepo struct {
	mu sync.RWMutex

	tasks          map[uint64]*model.Task
	deletedTaskIDs map[uint64]bool

	nextID uint64

	lastCreatedTask      *model.Task
	lastSearchQuery      *repository.TaskSearchQuery
	lastUpdateTaskID     uint64
	lastSoftDeleteTaskID uint64

	createErr      error
	searchErr      error
	getTaskErr     error
	updateErr      error
	batchUpdateErr error
	softDeleteErr  error
	countErr       error
}

func taskNewMockTaskRepo() *taskMockTaskRepo {
	return &taskMockTaskRepo{
		tasks:          make(map[uint64]*model.Task),
		deletedTaskIDs: make(map[uint64]bool),
		nextID:         100000,
	}
}

func (r *taskMockTaskRepo) CreateWithTx(ctx context.Context, tx *gorm.DB, task *model.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.createErr != nil {
		return r.createErr
	}

	if task == nil {
		return repository.ErrInvalidTaskParam
	}

	cp := taskCloneTask(task)

	if cp.ID == 0 {
		cp.ID = r.nextID
		task.ID = cp.ID
		r.nextID++
	}

	now := time.Now()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
		task.CreatedAt = now
	}
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = now
		task.UpdatedAt = now
	}

	cp.Project = taskBuildTestProject(cp.ProjectID)
	cp.Creator = taskBuildTestUser(cp.CreatorID, fmt.Sprintf("creator_%d", cp.CreatorID), "创建人")

	if cp.AssigneeID != nil {
		cp.Assignee = taskBuildTestUser(*cp.AssigneeID, fmt.Sprintf("assignee_%d", *cp.AssigneeID), "负责人")
	}

	r.tasks[cp.ID] = cp
	r.lastCreatedTask = taskCloneTask(cp)

	return nil
}

func (r *taskMockTaskRepo) SearchTasks(ctx context.Context, query *repository.TaskSearchQuery) ([]*model.Task, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.searchErr != nil {
		return nil, 0, r.searchErr
	}

	if query == nil {
		return nil, 0, repository.ErrInvalidTaskParam
	}

	r.lastSearchQuery = taskCloneSearchQuery(query)

	tasks := make([]*model.Task, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task == nil || r.deletedTaskIDs[task.ID] {
			continue
		}

		if query.ProjectID != 0 && task.ProjectID != query.ProjectID {
			continue
		}

		if query.AssigneeID != nil {
			if *query.AssigneeID == 0 {
				if task.AssigneeID != nil {
					continue
				}
			} else {
				if task.AssigneeID == nil || *task.AssigneeID != *query.AssigneeID {
					continue
				}
			}
		}

		if query.Keyword != "" && !strings.HasPrefix(task.Title, query.Keyword) {
			continue
		}

		if query.Status != "" && task.Status != query.Status {
			continue
		}

		if query.Priority != "" && task.Priority != query.Priority {
			continue
		}

		tasks = append(tasks, taskCloneTask(task))
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].SortOrder != tasks[j].SortOrder {
			return tasks[i].SortOrder < tasks[j].SortOrder
		}
		return tasks[i].ID < tasks[j].ID
	})

	total := int64(len(tasks))

	page := query.Page
	pageSize := query.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	if offset >= len(tasks) {
		return []*model.Task{}, total, nil
	}

	end := offset + pageSize
	if end > len(tasks) {
		end = len(tasks)
	}

	return tasks[offset:end], total, nil
}

func (r *taskMockTaskRepo) GetTaskByID(ctx context.Context, taskID uint64) (*model.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.getTaskErr != nil {
		return nil, r.getTaskErr
	}

	if r.deletedTaskIDs[taskID] {
		return nil, repository.ErrTaskNotFound
	}

	task, ok := r.tasks[taskID]
	if !ok || task == nil {
		return nil, repository.ErrTaskNotFound
	}

	return taskCloneTask(task), nil
}

func (r *taskMockTaskRepo) BatchUpdateTaskSortWithTx(
	ctx context.Context,
	tx *gorm.DB,
	projectID uint64,
	items []*repository.TaskSortItem,
	updatedAt time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.batchUpdateErr != nil {
		return r.batchUpdateErr
	}

	if projectID == 0 {
		return repository.ErrInvalidTaskParam
	}

	for _, item := range items {
		if item == nil || item.TaskID == 0 {
			return repository.ErrInvalidTaskParam
		}

		task, ok := r.tasks[item.TaskID]
		if !ok || task == nil || r.deletedTaskIDs[item.TaskID] {
			return repository.ErrTaskNotFound
		}

		if task.ProjectID != projectID {
			return repository.ErrInvalidTaskParam
		}

		task.SortOrder = item.SortOrder
		task.UpdatedAt = updatedAt
	}

	return nil
}

func (r *taskMockTaskRepo) SoftDeleteTaskByIDWithTx(ctx context.Context, tx *gorm.DB, taskID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.softDeleteErr != nil {
		return r.softDeleteErr
	}

	if taskID == 0 {
		return repository.ErrInvalidTaskParam
	}

	if _, ok := r.tasks[taskID]; !ok || r.deletedTaskIDs[taskID] {
		return repository.ErrTaskNotFound
	}

	r.deletedTaskIDs[taskID] = true
	r.lastSoftDeleteTaskID = taskID

	return nil
}

func (r *taskMockTaskRepo) CountTasksByProjectIDAndIDs(ctx context.Context, projectID uint64, taskIDs []uint64) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.countErr != nil {
		return 0, r.countErr
	}

	if projectID == 0 {
		return 0, repository.ErrInvalidTaskParam
	}

	seen := make(map[uint64]struct{}, len(taskIDs))
	var count int64

	for _, taskID := range taskIDs {
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}

		task, ok := r.tasks[taskID]
		if !ok || task == nil || r.deletedTaskIDs[taskID] {
			continue
		}

		if task.ProjectID == projectID {
			count++
		}
	}

	return count, nil
}
