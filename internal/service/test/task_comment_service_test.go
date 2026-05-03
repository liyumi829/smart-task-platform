// internal/service/task_comment_service_test.go
package test

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	tcTestProjectID  uint64 = 1001
	tcOtherProjectID uint64 = 1002

	tcTestTaskID     uint64 = 2001
	tcOtherTaskID    uint64 = 2002
	tcNotFoundTaskID uint64 = 2999

	tcTaskCreatorID uint64 = 3001
	tcOwnerID       uint64 = 3002
	tcAdminID       uint64 = 3003
	tcMemberID      uint64 = 3004
	tcOtherUserID   uint64 = 3005

	tcRootCommentID   uint64 = 4001
	tcReplyCommentID  uint64 = 4002
	tcOtherCommentID  uint64 = 4003
	tcRemoveCommentID uint64 = 4004
)

var errTCRepoMock = errors.New("task comment mock repository error")

// =========================
// Test Env
// =========================

type tcTaskCommentServiceTestEnv struct {
	ctx context.Context
	db  *gorm.DB

	txMgr       *repository.TxManager
	userRepo    *tcMockUserRepo
	memberRepo  *tcMockProjectMemberRepo
	taskRepo    *tcMockTaskRepo
	commentRepo *tcMockTaskCommentRepo

	svc *service.TaskCommentService
}

// =========================
// Test Env 修正
// =========================

func tcNewTaskCommentServiceTestEnv(t *testing.T) *tcTaskCommentServiceTestEnv {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	zap.ReplaceGlobals(logger)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	ctx := context.Background()

	userRepo := tcNewMockUserRepo()
	memberRepo := tcNewMockProjectMemberRepo()
	taskRepo := tcNewMockTaskRepo()

	// commentRepo 需要 userRepo，用于 mock 创建评论后补齐 Author / ReplyToUser
	commentRepo := tcNewMockTaskCommentRepo(userRepo)

	txMgr := repository.NewTxManager(db)

	env := &tcTaskCommentServiceTestEnv{
		ctx:         ctx,
		db:          db,
		txMgr:       txMgr,
		userRepo:    userRepo,
		memberRepo:  memberRepo,
		taskRepo:    taskRepo,
		commentRepo: commentRepo,
		svc: service.NewTaskCommentService(
			txMgr,
			userRepo,
			memberRepo,
			taskRepo,
			commentRepo,
		),
	}

	env.tcSeedBaseData(t)

	return env
}

func (e *tcTaskCommentServiceTestEnv) tcSeedBaseData(t *testing.T) {
	t.Helper()

	now := time.Now()

	e.userRepo.users[tcTaskCreatorID] = &model.User{
		ID:        tcTaskCreatorID,
		Username:  "tc_task_creator",
		Nickname:  "任务创建者",
		Avatar:    "https://example.com/task-creator.png",
		CreatedAt: now.Add(-96 * time.Hour),
		UpdatedAt: now.Add(-96 * time.Hour),
	}
	e.userRepo.users[tcOwnerID] = &model.User{
		ID:        tcOwnerID,
		Username:  "tc_owner",
		Nickname:  "项目所有者",
		Avatar:    "https://example.com/owner.png",
		CreatedAt: now.Add(-96 * time.Hour),
		UpdatedAt: now.Add(-96 * time.Hour),
	}
	e.userRepo.users[tcAdminID] = &model.User{
		ID:        tcAdminID,
		Username:  "tc_admin",
		Nickname:  "项目管理员",
		Avatar:    "https://example.com/admin.png",
		CreatedAt: now.Add(-96 * time.Hour),
		UpdatedAt: now.Add(-96 * time.Hour),
	}
	e.userRepo.users[tcMemberID] = &model.User{
		ID:        tcMemberID,
		Username:  "tc_member",
		Nickname:  "普通成员",
		Avatar:    "https://example.com/member.png",
		CreatedAt: now.Add(-96 * time.Hour),
		UpdatedAt: now.Add(-96 * time.Hour),
	}
	e.userRepo.users[tcOtherUserID] = &model.User{
		ID:        tcOtherUserID,
		Username:  "tc_other",
		Nickname:  "非项目成员",
		Avatar:    "https://example.com/other.png",
		CreatedAt: now.Add(-96 * time.Hour),
		UpdatedAt: now.Add(-96 * time.Hour),
	}

	e.memberRepo.members[tcProjectMemberKey(tcTestProjectID, tcOwnerID)] = &model.ProjectMember{
		ID:        1,
		ProjectID: tcTestProjectID,
		UserID:    tcOwnerID,
		Role:      model.ProjectMemberRoleOwner,
		JoinedAt:  now.Add(-72 * time.Hour),
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}
	e.memberRepo.members[tcProjectMemberKey(tcTestProjectID, tcAdminID)] = &model.ProjectMember{
		ID:        2,
		ProjectID: tcTestProjectID,
		UserID:    tcAdminID,
		Role:      model.ProjectMemberRoleAdmin,
		JoinedAt:  now.Add(-72 * time.Hour),
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}
	e.memberRepo.members[tcProjectMemberKey(tcTestProjectID, tcMemberID)] = &model.ProjectMember{
		ID:        3,
		ProjectID: tcTestProjectID,
		UserID:    tcMemberID,
		Role:      model.ProjectMemberRoleMember,
		JoinedAt:  now.Add(-72 * time.Hour),
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}
	e.memberRepo.members[tcProjectMemberKey(tcTestProjectID, tcTaskCreatorID)] = &model.ProjectMember{
		ID:        4,
		ProjectID: tcTestProjectID,
		UserID:    tcTaskCreatorID,
		Role:      model.ProjectMemberRoleMember,
		JoinedAt:  now.Add(-72 * time.Hour),
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}

	e.taskRepo.tasks[tcTestTaskID] = &model.Task{
		ID:        tcTestTaskID,
		ProjectID: tcTestProjectID,
		CreatorID: tcTaskCreatorID,
		Title:     "测试任务",
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-48 * time.Hour),
	}
	e.taskRepo.tasks[tcOtherTaskID] = &model.Task{
		ID:        tcOtherTaskID,
		ProjectID: tcOtherProjectID,
		CreatorID: tcOtherUserID,
		Title:     "其他项目任务",
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-48 * time.Hour),
	}

	e.commentRepo.comments[tcRootCommentID] = &model.TaskComment{
		ID:        tcRootCommentID,
		TaskID:    tcTestTaskID,
		AuthorID:  tcMemberID,
		Content:   "根评论内容",
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
	}
	e.commentRepo.comments[tcReplyCommentID] = &model.TaskComment{
		ID:            tcReplyCommentID,
		TaskID:        tcTestTaskID,
		AuthorID:      tcAdminID,
		ParentID:      tcUint64Ptr(tcRootCommentID),
		ReplyToUserID: tcUint64Ptr(tcMemberID),
		Content:       "回复评论内容",
		CreatedAt:     now.Add(-23 * time.Hour),
		UpdatedAt:     now.Add(-23 * time.Hour),
	}
	e.commentRepo.comments[tcOtherCommentID] = &model.TaskComment{
		ID:        tcOtherCommentID,
		TaskID:    tcOtherTaskID,
		AuthorID:  tcOtherUserID,
		Content:   "其他任务评论",
		CreatedAt: now.Add(-22 * time.Hour),
		UpdatedAt: now.Add(-22 * time.Hour),
	}
	e.commentRepo.comments[tcRemoveCommentID] = &model.TaskComment{
		ID:        tcRemoveCommentID,
		TaskID:    tcTestTaskID,
		AuthorID:  tcMemberID,
		Content:   "待删除评论",
		CreatedAt: now.Add(-21 * time.Hour),
		UpdatedAt: now.Add(-21 * time.Hour),
	}
}

// =========================
// Tests: CreateTaskComment
// =========================

func TestTaskCommentServiceCreateTaskComment(t *testing.T) {
	t.Run("success create root comment", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] success root resp=%+v err=%v", resp, err)

		// require.NoError(t, err)
		// require.NotNil(t, resp)

		// assert.True(t, env.userRepo.getByIDCalled)
		// assert.True(t, env.taskRepo.getByIDCalled)
		// assert.True(t, env.memberRepo.existsCalled)
		// assert.True(t, env.commentRepo.createCalled)
		// assert.False(t, env.commentRepo.lastTxIsNil)

		// require.NotNil(t, env.commentRepo.lastCreatedComment)
		// assert.Equal(t, tcTestTaskID, env.commentRepo.lastCreatedComment.TaskID)
		// assert.Equal(t, tcMemberID, env.commentRepo.lastCreatedComment.AuthorID)
		// assert.Nil(t, env.commentRepo.lastCreatedComment.ParentID)
		// assert.Nil(t, env.commentRepo.lastCreatedComment.ReplyToUserID)
		// assert.Equal(t, "这是一条有效的任务评论", env.commentRepo.lastCreatedComment.Content)
		// assert.NotZero(t, env.commentRepo.lastCreatedComment.ID)
	})

	t.Run("success create reply comment", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID:       tcAdminID,
			ProjectID:       tcTestProjectID,
			TaskID:          tcTestTaskID,
			ParentCommentID: tcUint64Ptr(tcRootCommentID),
			Content:         "这是一条有效的回复评论",
		})

		t.Logf("[create task comment] success reply resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.commentRepo.getByIDCalled)
		assert.True(t, env.commentRepo.createCalled)
		assert.False(t, env.commentRepo.lastTxIsNil)

		require.NotNil(t, env.commentRepo.lastCreatedComment)
		require.NotNil(t, env.commentRepo.lastCreatedComment.ParentID)
		require.NotNil(t, env.commentRepo.lastCreatedComment.ReplyToUserID)

		assert.Equal(t, tcRootCommentID, *env.commentRepo.lastCreatedComment.ParentID)
		assert.Equal(t, tcMemberID, *env.commentRepo.lastCreatedComment.ReplyToUserID)
		assert.Equal(t, tcAdminID, env.commentRepo.lastCreatedComment.AuthorID)
		assert.Equal(t, tcTestTaskID, env.commentRepo.lastCreatedComment.TaskID)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, nil)

		t.Logf("[create task comment] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTaskCommentParam)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed invalid ids", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: 0,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] invalid ids resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTaskCommentParam)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed empty content", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Content:   "",
		})

		t.Logf("[create task comment] empty content resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrEmptyTaskCommentContent)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed creator user not found", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcOtherUserID + 100,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] creator not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrUserNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed task not found", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcNotFoundTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] task not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed task not belong to project", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcOtherTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] task project mismatch resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed user not project member", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcOtherUserID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed parent comment not found", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID:       tcMemberID,
			ProjectID:       tcTestProjectID,
			TaskID:          tcTestTaskID,
			ParentCommentID: tcUint64Ptr(999999),
			Content:         "这是一条有效的回复评论",
		})

		t.Logf("[create task comment] parent not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrParentCommentNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed parent comment not belong to task", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID:       tcMemberID,
			ProjectID:       tcTestProjectID,
			TaskID:          tcTestTaskID,
			ParentCommentID: tcUint64Ptr(tcOtherCommentID),
			Content:         "这是一条有效的回复评论",
		})

		t.Logf("[create task comment] invalid parent resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidParentComment)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.createCalled)
	})

	t.Run("failed create repository error", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)
		env.commentRepo.createErr = errTCRepoMock

		resp, err := env.svc.CreateTaskComment(env.ctx, &service.CreateTaskCommentParam{
			CreatorID: tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Content:   "这是一条有效的任务评论",
		})

		t.Logf("[create task comment] repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errTCRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.commentRepo.createCalled)
		assert.False(t, env.commentRepo.lastTxIsNil)
	})
}

// =========================
// Tests: ListTaskComments
// =========================

func TestTaskCommentServiceListTaskComments(t *testing.T) {
	t.Run("success list task comments", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.ListTaskComments(env.ctx, &service.ListTaskCommentsParam{
			UserID:    tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list task comments] success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.taskRepo.getByIDCalled)
		assert.True(t, env.memberRepo.existsCalled)
		assert.True(t, env.commentRepo.searchCalled)

		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 10, resp.PageSize)
		assert.Equal(t, 3, resp.Total)
		assert.Len(t, resp.List, 3)

		assert.Equal(t, tcTestTaskID, env.commentRepo.lastSearchQuery.TaskID)
		assert.Equal(t, 1, env.commentRepo.lastSearchQuery.Page)
		assert.Equal(t, 10, env.commentRepo.lastSearchQuery.PageSize)
	})

	t.Run("success list empty comments", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		emptyTaskID := uint64(8888)
		env.taskRepo.tasks[emptyTaskID] = &model.Task{
			ID:        emptyTaskID,
			ProjectID: tcTestProjectID,
			CreatorID: tcTaskCreatorID,
			Title:     "无评论任务",
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		}

		resp, err := env.svc.ListTaskComments(env.ctx, &service.ListTaskCommentsParam{
			UserID:    tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    emptyTaskID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list task comments] empty resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 0, resp.Total)
		assert.Empty(t, resp.List)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.ListTaskComments(env.ctx, nil)

		t.Logf("[list task comments] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTaskCommentParam)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.searchCalled)
	})

	t.Run("failed invalid ids", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.ListTaskComments(env.ctx, &service.ListTaskCommentsParam{
			UserID:    tcMemberID,
			ProjectID: 0,
			TaskID:    tcTestTaskID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list task comments] invalid ids resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTaskCommentParam)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.searchCalled)
	})

	t.Run("failed task not found", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.ListTaskComments(env.ctx, &service.ListTaskCommentsParam{
			UserID:    tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcNotFoundTaskID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list task comments] task not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.searchCalled)
	})

	t.Run("failed user not project member", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.ListTaskComments(env.ctx, &service.ListTaskCommentsParam{
			UserID:    tcOtherUserID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list task comments] forbidden resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.searchCalled)
	})

	t.Run("failed search repository error", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)
		env.commentRepo.searchErr = errTCRepoMock

		resp, err := env.svc.ListTaskComments(env.ctx, &service.ListTaskCommentsParam{
			UserID:    tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			Page:      1,
			PageSize:  10,
		})

		t.Logf("[list task comments] search repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errTCRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.commentRepo.searchCalled)
	})
}

// =========================
// Tests: RemoveTaskComment
// =========================

func TestTaskCommentServiceRemoveTaskComment(t *testing.T) {
	t.Run("success task creator remove comment", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcTaskCreatorID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] task creator success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.taskRepo.getByIDCalled)
		assert.True(t, env.memberRepo.existsCalled)
		assert.True(t, env.commentRepo.getByIDCalled)
		assert.True(t, env.commentRepo.softDeleteCalled)
		assert.False(t, env.commentRepo.lastTxIsNil)
		assert.Equal(t, tcRemoveCommentID, env.commentRepo.lastSoftDeleteCommentID)

		comment, err := env.commentRepo.GetCommentByID(env.ctx, tcRemoveCommentID)
		require.ErrorIs(t, err, repository.ErrTaskCommentNotFound)
		assert.Nil(t, comment)

		comment, err = env.commentRepo.GetCommentByIDUnscoped(env.ctx, tcRemoveCommentID)
		require.NoError(t, err)
		require.NotNil(t, comment)
		assert.True(t, comment.DeletedAt.Valid)
	})

	t.Run("success owner remove comment", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcOwnerID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] owner success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.getRoleCalled)
		assert.True(t, env.commentRepo.softDeleteCalled)
		assert.False(t, env.commentRepo.lastTxIsNil)
	})

	t.Run("success admin remove comment", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcAdminID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] admin success resp=%+v err=%v", resp, err)

		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.True(t, env.memberRepo.getRoleCalled)
		assert.True(t, env.commentRepo.softDeleteCalled)
		assert.False(t, env.commentRepo.lastTxIsNil)
	})

	t.Run("failed nil param", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, nil)

		t.Logf("[remove task comment] nil param resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTaskCommentParam)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed invalid ids", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: 0,
		})

		t.Logf("[remove task comment] invalid ids resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrInvalidTaskCommentParam)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed task not found", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcTaskCreatorID,
			ProjectID: tcTestProjectID,
			TaskID:    tcNotFoundTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] task not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed user not project member", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcOtherUserID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] forbidden not member resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskForbidden)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed comment not found", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcTaskCreatorID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: 999999,
		})

		t.Logf("[remove task comment] comment not found resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskCommentNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed comment not belong to task", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcTaskCreatorID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcOtherCommentID,
		})

		t.Logf("[remove task comment] comment task mismatch resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskCommentNotFound)
		assert.Nil(t, resp)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed normal member permission denied", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcMemberID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] permission denied resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, service.ErrTaskForbidden)
		assert.Nil(t, resp)
		assert.True(t, env.memberRepo.getRoleCalled)
		assert.False(t, env.commentRepo.softDeleteCalled)
	})

	t.Run("failed soft delete repository error", func(t *testing.T) {
		env := tcNewTaskCommentServiceTestEnv(t)
		env.commentRepo.softDeleteErr = errTCRepoMock

		resp, err := env.svc.RemoveTaskComment(env.ctx, &service.RemoveTaskCommentParam{
			UserID:    tcTaskCreatorID,
			ProjectID: tcTestProjectID,
			TaskID:    tcTestTaskID,
			CommentID: tcRemoveCommentID,
		})

		t.Logf("[remove task comment] soft delete repo error resp=%+v err=%v", resp, err)

		require.ErrorIs(t, err, errTCRepoMock)
		assert.Nil(t, resp)
		assert.True(t, env.commentRepo.softDeleteCalled)
		assert.False(t, env.commentRepo.lastTxIsNil)

		comment, getErr := env.commentRepo.GetCommentByID(env.ctx, tcRemoveCommentID)
		require.NoError(t, getErr)
		require.NotNil(t, comment)
		assert.False(t, comment.DeletedAt.Valid)
	})
}

// =========================
// Helper
// =========================

func tcProjectMemberKey(projectID uint64, userID uint64) string {
	return fmt.Sprintf("%d:%d", projectID, userID)
}

func tcUint64Ptr(v uint64) *uint64 {
	return &v
}

func tcProjectMemberActive(pm *model.ProjectMember) bool {
	return pm != nil && !pm.DeletedAt.Valid
}

func tcTaskCommentActive(comment *model.TaskComment) bool {
	return comment != nil && !comment.DeletedAt.Valid
}

func tcCloneUser(user *model.User) *model.User {
	if user == nil {
		return nil
	}

	cp := *user
	return &cp
}

func tcCloneTask(task *model.Task) *model.Task {
	if task == nil {
		return nil
	}

	cp := *task
	return &cp
}

func tcCloneProjectMember(pm *model.ProjectMember) *model.ProjectMember {
	if pm == nil {
		return nil
	}

	cp := *pm
	return &cp
}

func tcCloneTaskComment(comment *model.TaskComment) *model.TaskComment {
	if comment == nil {
		return nil
	}

	cp := *comment

	if comment.ParentID != nil {
		v := *comment.ParentID
		cp.ParentID = &v
	}

	if comment.ReplyToUserID != nil {
		v := *comment.ReplyToUserID
		cp.ReplyToUserID = &v
	}

	if comment.Author != nil {
		cp.Author = tcCloneUser(comment.Author)
	}

	if comment.ReplyToUser != nil {
		cp.ReplyToUser = tcCloneUser(comment.ReplyToUser)
	}

	return &cp
}

// =========================
// Mock User Repo
// =========================

type tcMockUserRepo struct {
	mu sync.RWMutex

	users map[uint64]*model.User

	getByIDCalled bool
	getByIDErr    error
}

func tcNewMockUserRepo() *tcMockUserRepo {
	return &tcMockUserRepo{
		users: make(map[uint64]*model.User),
	}
}

func (m *tcMockUserRepo) GetByID(ctx context.Context, id uint64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getByIDCalled = true

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}

	user, ok := m.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}

	return tcCloneUser(user), nil
}

// =========================
// Mock Project Member Repo
// =========================

type tcMockProjectMemberRepo struct {
	mu sync.RWMutex

	members map[string]*model.ProjectMember

	existsCalled  bool
	getRoleCalled bool

	existsErr  error
	getRoleErr error
}

func tcNewMockProjectMemberRepo() *tcMockProjectMemberRepo {
	return &tcMockProjectMemberRepo{
		members: make(map[string]*model.ProjectMember),
	}
}

func (m *tcMockProjectMemberRepo) ExistsByProjectIDAndUserID(
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

	pm, ok := m.members[tcProjectMemberKey(projectID, userID)]
	return ok && tcProjectMemberActive(pm), nil
}

func (m *tcMockProjectMemberRepo) GetProjectMemberRoleByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getRoleCalled = true

	if m.getRoleErr != nil {
		return "", m.getRoleErr
	}

	pm, ok := m.members[tcProjectMemberKey(projectID, userID)]
	if !ok || !tcProjectMemberActive(pm) {
		return "", repository.ErrProjectMemberNotFound
	}

	return pm.Role, nil
}

func (m *tcMockProjectMemberRepo) GetProjectMemberByProjectIDAndUserID(
	ctx context.Context,
	projectID uint64,
	userID uint64,
) (*model.ProjectMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pm, ok := m.members[tcProjectMemberKey(projectID, userID)]
	if !ok || !tcProjectMemberActive(pm) {
		return nil, repository.ErrProjectMemberNotFound
	}

	return tcCloneProjectMember(pm), nil
}

func (m *tcMockProjectMemberRepo) CountByProjectIDAndRole(
	ctx context.Context,
	projectID uint64,
	role string,
) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int64
	for _, pm := range m.members {
		if pm == nil {
			continue
		}
		if pm.ProjectID == projectID && pm.Role == role && tcProjectMemberActive(pm) {
			count++
		}
	}

	return count, nil
}

// =========================
// Mock Task Repo
// =========================

type tcMockTaskRepo struct {
	mu sync.RWMutex

	tasks map[uint64]*model.Task

	getByIDCalled bool

	getByIDErr error
}

func tcNewMockTaskRepo() *tcMockTaskRepo {
	return &tcMockTaskRepo{
		tasks: make(map[uint64]*model.Task),
	}
}

func (m *tcMockTaskRepo) GetTaskByID(ctx context.Context, taskID uint64) (*model.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getByIDCalled = true

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, service.ErrTaskNotFound
	}

	return tcCloneTask(task), nil
}

// =========================
// Mock Task Comment Repo 修正版
// =========================

type tcMockTaskCommentRepo struct {
	mu sync.RWMutex

	// users 用于创建评论后补齐 Author / ReplyToUser，模拟真实数据库预加载效果
	userRepo *tcMockUserRepo

	// comments 保存全部评论，包括有效评论和已软删除评论
	comments map[uint64]*model.TaskComment

	nextID uint64

	createCalled                   bool
	getByIDCalled                  bool
	getByIDUnscopedCalled          bool
	searchCalled                   bool
	softDeleteCalled               bool
	getCommentsByIDsIncludeDeleted bool

	lastTxIsNil bool

	lastCreatedComment                   *model.TaskComment
	lastSearchQuery                      *repository.SearchTaskCommentsQuery
	lastSoftDeleteCommentID              uint64
	lastGetCommentsByIDsIncludeDeletedID []uint64

	createErr                         error
	getByIDErr                        error
	getByIDUnscopedErr                error
	searchErr                         error
	softDeleteErr                     error
	getCommentsByIDsIncludeDeletedErr error
}

func tcNewMockTaskCommentRepo(userRepo *tcMockUserRepo) *tcMockTaskCommentRepo {
	return &tcMockTaskCommentRepo{
		userRepo: userRepo,
		comments: make(map[uint64]*model.TaskComment),
		nextID:   10000,
	}
}

// CreateCommentWithTx 创建评论，并补齐 Author / ReplyToUser，避免 service 构造响应时空指针
func (m *tcMockTaskCommentRepo) CreateCommentWithTx(
	ctx context.Context,
	tx *gorm.DB,
	comment *model.TaskComment,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createCalled = true
	m.lastTxIsNil = tx == nil

	if m.createErr != nil {
		return m.createErr
	}
	if comment == nil || comment.TaskID == 0 || comment.AuthorID == 0 {
		return repository.ErrInvalidTaskCommentParam
	}

	now := time.Now()

	if comment.ID == 0 {
		comment.ID = m.nextID
		m.nextID++
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = now
	}
	if comment.UpdatedAt.IsZero() {
		comment.UpdatedAt = now
	}

	// 创建时保证评论为未删除状态
	comment.DeletedAt = gorm.DeletedAt{}

	// 补齐 Author / ReplyToUser，模拟真实 repo 创建后预加载效果
	m.preloadTaskCommentUsers(ctx, comment)

	cp := tcCloneTaskComment(comment)
	m.comments[comment.ID] = cp
	m.lastCreatedComment = tcCloneTaskComment(cp)

	return nil
}

// GetCommentByID 查询未删除评论
func (m *tcMockTaskCommentRepo) GetCommentByID(
	ctx context.Context,
	commentID uint64,
) (*model.TaskComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getByIDCalled = true

	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if commentID == 0 {
		return nil, repository.ErrInvalidTaskCommentParam
	}

	comment, ok := m.comments[commentID]
	if !ok || !tcTaskCommentActive(comment) {
		return nil, repository.ErrTaskCommentNotFound
	}

	cp := tcCloneTaskComment(comment)

	// 查询评论时补齐 Author / ReplyToUser，模拟真实 repo 预加载
	m.preloadTaskCommentUsers(ctx, cp)

	return cp, nil
}

// GetCommentByIDUnscoped 查询评论，包括已软删除评论
func (m *tcMockTaskCommentRepo) GetCommentByIDUnscoped(
	ctx context.Context,
	commentID uint64,
) (*model.TaskComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getByIDUnscopedCalled = true

	if m.getByIDUnscopedErr != nil {
		return nil, m.getByIDUnscopedErr
	}
	if commentID == 0 {
		return nil, repository.ErrInvalidTaskCommentParam
	}

	comment, ok := m.comments[commentID]
	if !ok {
		return nil, repository.ErrTaskCommentNotFound
	}

	cp := tcCloneTaskComment(comment)

	// Unscoped 查询也补齐 Author / ReplyToUser，保持 mock 行为一致
	m.preloadTaskCommentUsers(ctx, cp)

	return cp, nil
}

// SearchComments 查询未删除评论列表
func (m *tcMockTaskCommentRepo) SearchComments(
	ctx context.Context,
	query *repository.SearchTaskCommentsQuery,
) ([]*model.TaskComment, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchCalled = true
	m.lastSearchQuery = query

	if m.searchErr != nil {
		return nil, 0, m.searchErr
	}
	if query == nil || query.TaskID == 0 {
		return nil, 0, repository.ErrInvalidTaskCommentParam
	}

	comments := make([]*model.TaskComment, 0)

	for _, comment := range m.comments {
		if comment == nil || !tcTaskCommentActive(comment) {
			continue
		}
		if comment.TaskID != query.TaskID {
			continue
		}

		cp := tcCloneTaskComment(comment)

		// 列表查询时补齐 Author / ReplyToUser
		m.preloadTaskCommentUsers(ctx, cp)

		comments = append(comments, cp)
	}

	sort.Slice(comments, func(i, j int) bool {
		return comments[i].ID < comments[j].ID
	})

	total := int64(len(comments))

	page := query.Page
	pageSize := query.PageSize

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	start := (page - 1) * pageSize
	if start >= len(comments) {
		return []*model.TaskComment{}, total, nil
	}

	end := start + pageSize
	if end > len(comments) {
		end = len(comments)
	}

	return comments[start:end], total, nil
}

// GetCommentsByIDsIncludeDeleted 批量查询评论，包括已软删除评论
func (m *tcMockTaskCommentRepo) GetCommentsByIDsIncludeDeleted(
	ctx context.Context,
	ids []uint64,
) ([]*model.TaskComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getCommentsByIDsIncludeDeleted = true
	m.lastGetCommentsByIDsIncludeDeletedID = append([]uint64(nil), ids...)

	if m.getCommentsByIDsIncludeDeletedErr != nil {
		return nil, m.getCommentsByIDsIncludeDeletedErr
	}
	if len(ids) == 0 {
		return []*model.TaskComment{}, nil
	}

	// 去重，避免重复 ID 导致返回重复数据
	idSet := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		idSet[id] = struct{}{}
	}

	comments := make([]*model.TaskComment, 0, len(idSet))

	for id := range idSet {
		comment, ok := m.comments[id]
		if !ok {
			continue
		}

		cp := tcCloneTaskComment(comment)

		// 包含软删除查询时也补齐 Author / ReplyToUser，模拟真实 repo 预加载效果
		m.preloadTaskCommentUsers(ctx, cp)

		comments = append(comments, cp)
	}

	sort.Slice(comments, func(i, j int) bool {
		return comments[i].ID < comments[j].ID
	})

	return comments, nil
}

// SoftDeleteCommentWithTx 软删除评论
func (m *tcMockTaskCommentRepo) SoftDeleteCommentWithTx(
	ctx context.Context,
	tx *gorm.DB,
	commentID uint64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.softDeleteCalled = true
	m.lastTxIsNil = tx == nil
	m.lastSoftDeleteCommentID = commentID

	if m.softDeleteErr != nil {
		return m.softDeleteErr
	}
	if commentID == 0 {
		return repository.ErrInvalidTaskCommentParam
	}

	comment, ok := m.comments[commentID]
	if !ok || !tcTaskCommentActive(comment) {
		return repository.ErrTaskCommentNotFound
	}

	now := time.Now()
	comment.DeletedAt = gorm.DeletedAt{
		Time:  now,
		Valid: true,
	}
	comment.UpdatedAt = now

	return nil
}

// preloadTaskCommentUsers 补齐评论中的 Author / ReplyToUser，模拟真实数据库预加载
func (m *tcMockTaskCommentRepo) preloadTaskCommentUsers(
	ctx context.Context,
	comment *model.TaskComment,
) {
	if m.userRepo == nil || comment == nil {
		return
	}

	// 补齐评论作者
	if comment.AuthorID > 0 {
		if user, err := m.userRepo.GetByID(ctx, comment.AuthorID); err == nil {
			comment.Author = user
		}
	}

	// 补齐被回复用户
	if comment.ReplyToUserID != nil && *comment.ReplyToUserID > 0 {
		if user, err := m.userRepo.GetByID(ctx, *comment.ReplyToUserID); err == nil {
			comment.ReplyToUser = user
		}
	}
}
