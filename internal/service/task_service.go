// internal/service/task_service.go
// 实现任务模块的业务处理函数
package service

import (
	"context"
	"errors"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/pkg/validator"
	"smart-task-platform/internal/repository"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TaskService 任务服务
type TaskService struct {
	txMgr *repository.TxManager // 事务管理器
	ur    taskSvcUserRepo
	pr    taskSvcProjectRepo
	pmr   taskSvcProjectMemberRepo
	tr    taskSvcTaskrRepo
}

// NewTaskService 创建任务服务实例
func NewTaskService(
	txMgr *repository.TxManager,
	userRepo taskSvcUserRepo,
	projectRepo taskSvcProjectRepo,
	projectMemberRepo taskSvcProjectMemberRepo,
	taskRepo taskSvcTaskrRepo,
) *TaskService {
	return &TaskService{
		txMgr: txMgr,
		ur:    userRepo,
		pr:    projectRepo,
		pmr:   projectMemberRepo,
		tr:    taskRepo,
	}
}

// CreateTaskParam 创建项目的参数
type CreateTaskParam struct {
	CreatorID   uint64 // 创建者ID
	ProjectID   uint64 // 项目ID
	Title       string // 标题 需要检查是否合法
	Description string // 描述 "" 都存储为 NULL
	Priority    string // 优先级 需要检查是否合法
	AssigneeID  uint64 // 0 存储为 NULL
	DueDate     string // "" 存储为 NULL
}

// CreateTask 创建项目
func (s *TaskService) CreateTask(ctx context.Context, param *CreateTaskParam) (*dto.CreateTaskResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("create task failed: invalid param")
		return nil, ErrInvalidTaskParam
	}
	if param.CreatorID == 0 || param.ProjectID == 0 {
		zap.L().Warn("create task failed: invalid creator_id or invalid project_id",
			zap.Uint64("creator_id", param.CreatorID),
			zap.Uint64("project_id", param.ProjectID),
		)
		return nil, ErrInvalidTaskParam
	}
	// 上层对数据做了 TrimSpace 判断
	// 这里只做业务逻辑判断

	logger := zap.L().With(
		zap.Uint64("creator_id", param.CreatorID),
		zap.Uint64("project_id", param.ProjectID),
		zap.String("title", param.Title),
		zap.String("priority", param.Priority),
		zap.Uint64("assignee_id", param.AssigneeID),
		zap.String("due_date", param.DueDate),
	)
	// 检查标题
	if param.Title == "" {
		logger.Warn("create task failed: title is empty")
		return nil, ErrEmptyTaskTitle
	}
	if !validator.IsValidTaskTitle(param.Title) {
		logger.Warn("create task failed: title is invalid")
		return nil, ErrInvalidTaskTitle
	}
	// description 业务语义：
	// - "" 表示用户没有填写，数据库存储为 NULL
	// - 非空字符串必须满足描述规则
	var newDescription *string
	if param.Description == "" {
		newDescription = nil
	} else if validator.IsValidDescription(param.Description) {
		v := param.Description
		newDescription = &v
	} else {
		logger.Warn("create task failed: description is invalid")
		return nil, ErrInvalidTaskDescription
	}
	// 检查优先级
	if param.Priority == "" || !isValidTaskPriority(param.Priority) {
		// 返回错误的优先级参数
		logger.Warn("create task failed: priority is invalid")
		return nil, ErrInvalidTaskPriority
	}
	// 检查预期时间
	newDueDate, err := parseOptionalISOTime(param.DueDate)
	if err != nil {
		logger.Warn("create task failed: due date is invalid", zap.Error(err))
		return nil, ErrInvalidTaskTime
	}
	// 下面查数据库对参数校验
	// 项目是否存在、创建者是否存在、负责人是否存在
	// 检查项目是否存在
	exists, err := s.pr.ExistsByProjectID(ctx, param.ProjectID)
	if err != nil {
		logger.Error("create task failed: check project exists error", zap.Error(err))
		return nil, err
	}
	if !exists {
		logger.Warn("create task failed: project not found")
		return nil, ErrProjectNotFound
	}

	// 检查创建者是否存在
	creator, err := s.ur.GetByID(ctx, param.CreatorID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			logger.Warn("create task failed: creator not found")
			return nil, ErrUserNotFound
		}

		logger.Error("create task failed: get creator error", zap.Error(err))
		return nil, err
	}
	// 存在检查创建者是否是项目成员（普通成员也可以创建任务）
	isMember, err := s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.CreatorID)
	if err != nil {
		logger.Error("create task failed: check creator project member error", zap.Error(err))
		return nil, err
	}
	if !isMember {
		logger.Warn("create task failed: creator is not project member")
		return nil, ErrProjectForbidden
	}
	// 检查负责人是否存在
	var newAssigneeID *uint64 // 避免指向param结构，默认nil
	var assignee *model.User  // 指向 model 实例不复制
	if param.AssigneeID != 0 {
		assignee, err = s.ur.GetByID(ctx, param.AssigneeID)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				logger.Warn("create task failed: assignee user not found")
				return nil, ErrAssigneeNotFount // 指派的负责人不存在
			}
			logger.Error("create task failed: get assignee error", zap.Error(err))
			return nil, err
		}
		// 检查负责人是否是项目成员（普通成员也可以创建任务）
		isMember, err = s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.AssigneeID)
		if err != nil {
			logger.Error("create task failed: check assignee project member error", zap.Error(err))
			return nil, err
		}
		if !isMember {
			logger.Warn("create task failed: assignee is not project member")
			return nil, ErrAssigneeNotProjectMember // 返回成员没有找到
		}
		// 找到了
		v := param.AssigneeID // 复制一份
		newAssigneeID = &v
	}
	// 如果没有走if，这里两者都为空

	// 所有参数检查完成，事务调用
	task := &model.Task{
		CreatorID:   param.CreatorID,
		ProjectID:   param.ProjectID,
		Title:       param.Title,
		Description: newDescription,
		Priority:    param.Priority,
		AssigneeID:  newAssigneeID,
		DueDate:     newDueDate,
	}

	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tr.CreateWithTx(ctx, tx, task)
	})
	if err != nil {
		logger.Error("create task failed: transaction error", zap.Error(err))
		return nil, err
	}

	logger.Info("create task success",
		zap.Uint64("task_id", task.ID),
		zap.Bool("has_description", task.Description != nil),
		zap.Bool("has_assignee", task.AssigneeID != nil),
		zap.Bool("has_due_date", task.DueDate != nil),
	)

	// 构造响应
	return &dto.CreateTaskResp{
		TaskBaseFields: buildTaskBaseFields(task),
		Description:    utils.SafeStringValue(task.Description),
		Assignee:       buildUserPublicProfile(assignee),
		CreatorID:      task.CreatorID,
		Creator:        buildUserPublicProfile(creator),
	}, nil
}

// ListProjectTasksParam 获取项目下的任务列表参数
type ListProjectTasksParam struct {
	UserID                                       uint64  // 操作人
	ProjectID                                    uint64  // 项目
	AssigneeID                                   *uint64 // 负责人：nil表示全量、0表示找NULL
	Page                                         int     // 页码
	PageSize                                     int     // 条数
	Status, Priority, Keyword, SortBy, SortOrder string
	// 状态、优先级、关键词、排序规则、排序顺序
}

// ListProjectTasks 获取项目下的任务列表
func (s *TaskService) ListProjectTasks(ctx context.Context, param *ListProjectTasksParam) (*dto.ProjectTaskListResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("list project tasks failed: invalid param")
		return nil, ErrInvalidTaskParam
	}
	if param.UserID == 0 || param.ProjectID == 0 {
		zap.L().Warn("list project tasks failed: invalid user_id or project_id",
			zap.Uint64("user_id", param.UserID),
			zap.Uint64("project_id", param.ProjectID),
		)
		return nil, ErrInvalidTaskParam
	}

	page, pageSize := fixPageParams(param.Page, param.PageSize) // 分页参数兜底。

	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
		zap.String("keyword", param.Keyword),
		zap.String("status", param.Status),
		zap.String("priority", param.Priority),
		zap.String("sort_by", param.SortBy),
		zap.String("sort_order", param.SortOrder),
	)

	if param.AssigneeID != nil {
		logger = logger.With(zap.Uint64("assignee_id", *param.AssigneeID))
	}

	// 检查参数
	// 检查状态是否合法
	if param.Status != "" && !isValidTaskStatus(param.Status) {
		logger.Warn("list project tasks failed: status is invalid")
		return nil, ErrInvalidTaskStatus
	}

	// 检查优先级是否合法
	if param.Priority != "" && !isValidTaskPriority(param.Priority) {
		logger.Warn("list project tasks failed: priority is invalid")
		return nil, ErrInvalidTaskPriority
	}

	// 检查排序规则是否合法
	if param.SortBy != "" && !isValidTaskSortBy(param.SortBy) {
		logger.Warn("list project tasks failed: sort by is invalid")
		return nil, ErrInvalidTaskSortBy
	}

	// 检查排序顺序是否合法
	if param.SortOrder != "" && !IsValidTaskSortOrder(param.SortOrder) {
		logger.Warn("list project tasks failed: sort order is invalid")
		return nil, ErrInvalidTaskSortOrder
	}

	// 对数据库数据进行校验
	// 判断检查用户是否存在、项目是否存在、判断用户是否有权限、负责人是否是项目成员

	// 用户是否存在
	exists, err := s.ur.ExistsByUserID(ctx, param.UserID)
	if err != nil {
		logger.Error("list project tasks failed: check user exists error", zap.Error(err))
		return nil, err
	}
	if !exists {
		logger.Warn("list project tasks failed: user not found")
		return nil, ErrUserNotFound
	}

	// 项目是否存在
	exists, err = s.pr.ExistsByProjectID(ctx, param.ProjectID)
	if err != nil {
		logger.Error("list project tasks failed: check project exists error", zap.Error(err))
		return nil, err
	}
	if !exists {
		logger.Warn("list project tasks failed: project not found")
		return nil, ErrProjectNotFound
	}

	// 存在检查用户是否是项目成员（普通成员也可以查询任务）
	isMember, err := s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.UserID)
	if err != nil {
		logger.Error("list project tasks failed: check user project member error", zap.Error(err))
		return nil, err
	}
	if !isMember {
		logger.Warn("list project tasks failed: user is not project member")
		return nil, ErrProjectForbidden
	}

	var assigneeID *uint64
	// 判断负责人是否存在、是否是项目成员
	if param.AssigneeID != nil {
		v := *param.AssigneeID
		// assignee_id = 0 表示查询未分配任务，不需要校验成员关系，但必须继续传给 repository
		if v != 0 { // 有效的
			exists, err = s.ur.ExistsByUserID(ctx, *param.AssigneeID)
			if err != nil {
				logger.Error("list project tasks failed: check assignee exists error", zap.Error(err))
				return nil, err
			}
			if !exists {
				logger.Warn("list project tasks failed: assignee user not found")
				return nil, ErrAssigneeNotFount
			}

			isMember, err = s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, *param.AssigneeID)
			if err != nil {
				logger.Error("list project tasks failed: check assignee project member error", zap.Error(err))
				return nil, err
			}
			if !isMember {
				logger.Warn("list project tasks failed: assignee is not project member")
				return nil, ErrAssigneeNotProjectMember
			}
		}
		// 不管是否是0都给他赋值
		assigneeID = &v
	}

	// 查询
	tasks, total, err := s.tr.SearchTasks(ctx, &repository.TaskSearchQuery{
		Page:       page,
		PageSize:   pageSize,
		ProjectID:  param.ProjectID,
		AssigneeID: assigneeID,
		Keyword:    param.Keyword,
		Status:     param.Status,
		Priority:   param.Priority,
		SortBy:     param.SortBy,
		SortOrder:  param.SortOrder,
	})
	if err != nil {
		logger.Error("list project tasks failed: search tasks error", zap.Error(err))
		return nil, err
	}

	// 搜索成功 构造list
	list := make([]*dto.ProjectTaskListItem, 0, len(tasks))
	for _, task := range tasks {
		if task == nil { // 防御
			continue
		}

		item := &dto.ProjectTaskListItem{
			TaskBaseFields: buildTaskBaseFields(task),
			Assignee:       buildUserPublicProfile(task.Assignee),
		}
		list = append(list, item)
	}

	logger.Info("list project tasks success",
		zap.Int64("total", total),
		zap.Int("list_size", len(list)),
	)

	// 成功 构造响应
	return &dto.ProjectTaskListResp{
		List:     list,
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// ListUserTasksParam 获取用户下的任务列表参数
type ListUserTasksParam struct {
	UserID                                       uint64 // 操作人
	Page                                         int    // 页码
	PageSize                                     int    // 条数
	ProjectID                                    uint64 // 项目作为筛选条件 0 表示全量查询
	Status, Priority, Keyword, SortBy, SortOrder string
	// 状态、优先级、关键词、排序规则、排序顺序
}

// ListUserTasks 获取某个用户的任务列表
func (s *TaskService) ListUserTasks(ctx context.Context, param *ListUserTasksParam) (*dto.UserTaskListResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("list user tasks failed: invalid param")
		return nil, ErrInvalidTaskParam
	}
	if param.UserID == 0 {
		zap.L().Warn("list user tasks failed: invalid user_id",
			zap.Uint64("user_id", param.UserID),
		)
		return nil, ErrInvalidTaskParam
	}

	page, pageSize := fixPageParams(param.Page, param.PageSize) // 分页参数兜底。

	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
		zap.String("keyword", param.Keyword),
		zap.String("status", param.Status),
		zap.String("priority", param.Priority),
		zap.String("sort_by", param.SortBy),
		zap.String("sort_order", param.SortOrder),
	)

	// 检查参数
	// 检查状态是否合法
	if param.Status != "" && !isValidTaskStatus(param.Status) {
		logger.Warn("list user tasks failed: status is invalid")
		return nil, ErrInvalidTaskStatus
	}

	// 检查优先级是否合法
	if param.Priority != "" && !isValidTaskPriority(param.Priority) {
		logger.Warn("list user tasks failed: priority is invalid")
		return nil, ErrInvalidTaskPriority
	}

	// 检查排序规则是否合法
	if param.SortBy != "" && !isValidTaskSortBy(param.SortBy) {
		logger.Warn("list user tasks failed: sort by is invalid")
		return nil, ErrInvalidTaskSortBy
	}

	// 检查排序顺序是否合法
	if param.SortOrder != "" && !IsValidTaskSortOrder(param.SortOrder) {
		logger.Warn("list user tasks failed: sort order is invalid")
		return nil, ErrInvalidTaskSortOrder
	}

	// 对数据库数据进行校验
	// 判断检查用户是否存在、项目是否存在、判断用户是否有权限

	// 用户是否存在
	exists, err := s.ur.ExistsByUserID(ctx, param.UserID)
	if err != nil {
		logger.Error("list user tasks failed: check user exists error", zap.Error(err))
		return nil, err
	}
	if !exists {
		logger.Warn("list user tasks failed: user not found")
		return nil, ErrUserNotFound
	}

	if param.ProjectID != 0 {
		// 如果项目ID != 0，检查该用户是否是项目成员
		// 项目是否存在
		exists, err = s.pr.ExistsByProjectID(ctx, param.ProjectID)
		if err != nil {
			logger.Error("list user tasks failed: check project exists error", zap.Error(err))
			return nil, err
		}
		if !exists {
			logger.Warn("list user tasks failed: project not found")
			return nil, ErrProjectNotFound
		}

		// 存在检查用户是否是项目成员
		isMember, err := s.pmr.ExistsByProjectIDAndUserID(ctx, param.ProjectID, param.UserID)
		if err != nil {
			logger.Error("list user tasks failed: check user project member error", zap.Error(err))
			return nil, err
		}
		if !isMember {
			logger.Warn("list user tasks failed: user is not project member")
			return nil, ErrProjectForbidden
		}
	}

	assigneeID := param.UserID                                              // 避免直接把结构体字段地址传入 repository
	tasks, total, err := s.tr.SearchTasks(ctx, &repository.TaskSearchQuery{ // 进行批量查询
		Page:       page,
		PageSize:   pageSize,
		ProjectID:  param.ProjectID,
		AssigneeID: &assigneeID,
		Keyword:    param.Keyword,
		Status:     param.Status,
		Priority:   param.Priority,
		SortBy:     param.SortBy,
		SortOrder:  param.SortOrder,
	})
	if err != nil {
		logger.Error("list user tasks failed: search tasks error", zap.Error(err))
		return nil, err
	}

	// 搜索成功 构造list
	list := make([]*dto.UserTaskListItem, 0, len(tasks))
	for _, task := range tasks {
		if task == nil { // 防御
			continue
		}
		item := &dto.UserTaskListItem{
			TaskBaseFields: buildTaskBaseFields(task),
			Project:        buildProjectPublicProfile(task.Project),
		}
		list = append(list, item)
	}

	logger.Info("list user tasks success",
		zap.Int64("total", total),
		zap.Int("list_size", len(list)),
	)

	// 成功 构造响应
	return &dto.UserTaskListResp{
		List:     list,
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetTaskDetailParam 获取任务详情参数
type GetTaskDetailParam struct {
	UserID uint64 // 用户ID
	TaskID uint64 // 任务ID
}

// GetTaskDetail 获取任务详情
func (s *TaskService) GetTaskDetail(ctx context.Context, param *GetTaskDetailParam) (*dto.GetTaskDetailResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("get task detail failed: invalid param")
		return nil, ErrInvalidTaskParam
	}
	if param.UserID == 0 || param.TaskID == 0 {
		zap.L().Warn("get task detail failed: invalid user_id or task_id",
			zap.Uint64("user_id", param.UserID),
			zap.Uint64("task_id", param.TaskID),
		)
		return nil, ErrInvalidTaskParam
	}

	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("task_id", param.TaskID),
	)

	// 查询数据库查看是否符合权限
	// 先查看用户是否存在
	exists, err := s.ur.ExistsByUserID(ctx, param.UserID)
	if err != nil {
		logger.Error("get task detail failed: check user exists error", zap.Error(err))
		return nil, err
	}
	if !exists {
		logger.Warn("get task detail failed: user not found")
		return nil, ErrUserNotFound
	}

	// 获取任务详情
	// 一个用户可以查看的用户详情：他也是这个项目中的成员、他自己的任务
	task, err := s.tr.GetTaskByID(ctx, param.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("get task detail failed: task not found")
			return nil, ErrTaskNotFound
		}

		logger.Error("get task detail failed: get task by id error", zap.Error(err))
		return nil, err
	}

	logger = logger.With(
		zap.Uint64("project_id", task.ProjectID),
		zap.Uint64("creator_id", task.CreatorID),
	)

	// 找到了，进行合法性判断
	// 检查用户是否是这个任务所属的项目的成员
	isMember, err := s.pmr.ExistsByProjectIDAndUserID(ctx, task.ProjectID, param.UserID)
	if err != nil {
		logger.Error("get task detail failed: check user project member error", zap.Error(err))
		return nil, err
	}

	// 检查任务是否是这个任务的负责人
	var isAssignee bool
	if task.AssigneeID != nil {
		isAssignee = param.UserID == *task.AssigneeID
		logger = logger.With(zap.Uint64("assignee_id", *task.AssigneeID))
	}

	// 权限判断
	if !isMember && !isAssignee {
		logger.Warn("get task detail failed: user has no permission",
			zap.Bool("is_project_member", isMember),
			zap.Bool("is_assignee", isAssignee),
		)
		return nil, ErrTaskForbidden // 没有查看权限
	}

	logger.Info("get task detail success",
		zap.Bool("is_project_member", isMember),
		zap.Bool("is_assignee", isAssignee),
	)

	// 返回响应
	return &dto.GetTaskDetailResp{
		TaskBaseFields: buildTaskBaseFields(task),
		Description:    utils.SafeStringValue(task.Description),
		Project:        buildProjectPublicProfile(task.Project),
		Assignee:       buildUserPublicProfile(task.Assignee),
		CreatorID:      task.CreatorID,
		Creator:        buildUserPublicProfile(task.Creator),
	}, nil
}

// UpdateTaskBaseInfoParam 更新任务基础信息参数
//   - Title / Priority："" 表示不更新
//   - Description：nil 表示不更新，"" 表示清空为 NULL
//   - DueDate：nil 表示不更新，"" 表示清空为 NULL
type UpdateTaskBaseInfoParam struct {
	UserID      uint64  // 用户ID
	TaskID      uint64  // 任务ID
	Title       string  // 标题
	Priority    string  // 优先级
	Description *string // 描述
	DueDate     *string // 预期时间
}

// UpdateTaskBaseInfo 更新任务基础信息
func (s *TaskService) UpdateTaskBaseInfo(ctx context.Context, param *UpdateTaskBaseInfoParam) (*dto.UpdateTaskBaseInfoResp, error) {
	// 参数校验
	if param == nil {
		zap.L().Warn("update task base info failed: invalid nil param")
		return nil, ErrInvalidTaskParam
	}
	logger := zap.L().With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("task_id", param.TaskID),
	)

	if param.UserID == 0 || param.TaskID == 0 {
		logger.Warn("update task base info failed: invalid user_id or task_id")
		return nil, ErrInvalidTaskParam
	}

	// 校验标题：
	// - "" 表示不更新
	// - 非空字符串必须符合标题规则
	var newTitle *string
	if param.Title == "" {
		newTitle = nil
	} else if validator.IsValidTaskTitle(param.Title) {
		v := param.Title
		newTitle = &v
	} else {
		logger.Warn("update task base info failed: invalid task title",
			zap.String("title", param.Title),
		)
		return nil, ErrInvalidTaskTitle
	}

	// 校验优先级：
	// - "" 表示不更新
	// - 非空字符串必须是合法优先级
	var newPriority *string
	if param.Priority == "" {
		newPriority = nil
	} else if isValidTaskPriority(param.Priority) {
		v := param.Priority
		newPriority = &v
	} else {
		logger.Warn("update task base info failed: invalid task priority",
			zap.String("priority", param.Priority),
		)
		return nil, ErrInvalidTaskPriority
	}

	// 校验描述：
	// - nil 表示不更新
	// - "" 表示清空描述，数据库设置为 NULL
	// - 非空字符串必须符合描述规则
	var newDescription *string
	updateDescription := false
	if param.Description == nil {
		newDescription = nil
		updateDescription = false
	} else if *param.Description == "" {
		newDescription = nil
		updateDescription = true
	} else if validator.IsValidDescription(*param.Description) {
		v := *param.Description
		newDescription = &v
		updateDescription = true
	} else {
		logger.Warn("update task base info failed: invalid task description")
		return nil, ErrInvalidTaskDescription
	}

	// 校验截止时间：
	// - nil 表示不更新
	// - "" 表示清空截止时间，数据库设置为 NULL
	// - 非空字符串必须是合法时间格式
	var newDueDate *time.Time
	updateDueDate := false
	if param.DueDate != nil {
		parsedDueDate, err := parseOptionalISOTime(*param.DueDate)
		if err != nil {
			logger.Warn("update task base info failed: invalid task due date",
				zap.String("due_date", *param.DueDate),
				zap.Error(err),
			)
			return nil, ErrInvalidTaskTime
		}

		newDueDate = parsedDueDate
		updateDueDate = true
	}

	// 查询数据库查看是否符合权限
	// owner、admin、任务创建人可以操作。
	// 获取任务信息得到任务创建人(isCreator)
	// 查询任务信息，用于权限判断和构建返回数据
	task, err := s.tr.GetTaskByID(ctx, param.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("update task base info failed: task not found")
			return nil, ErrTaskNotFound
		}

		logger.Error("update task base info failed: get task error",
			zap.Error(err),
		)
		return nil, err
	}

	logger = logger.With(
		zap.Uint64("project_id", task.ProjectID),
		zap.Uint64("creator_id", task.CreatorID),
	)

	// 权限规则：
	// - 任务创建人可以更新
	// - 项目 owner/admin 可以更新
	isCreator := task.CreatorID == param.UserID
	hasPermission := isCreator
	if !hasPermission {
		hasPermission, err = hasProjectManagePermission(ctx, s.pmr, task.ProjectID, param.UserID, logger)
		if err != nil {
			logger.Warn("update task base info failed: project manage permission check error",
				zap.Error(err),
			)
			return nil, err
		}
	}
	if !hasPermission {
		logger.Warn("update task base info failed: permission denied",
			zap.Bool("is_creator", isCreator),
		)
		return nil, ErrTaskForbidden
	}

	// 判断：如果没有一个需要更新字段就直接返回原数据
	if newTitle == nil && newPriority == nil && !updateDescription && !updateDueDate {
		logger.Info("update task base info skipped: no fields to update")
		return &dto.UpdateTaskBaseInfoResp{
			TaskBaseFields: buildTaskBaseFields(task),
			Description:    utils.SafeStringValue(task.Description),
			CreatorID:      task.CreatorID,
			StartTime:      task.StartTime,
			CompletedAt:    task.CompletedAt,
			SortOrder:      task.SortOrder,
		}, nil
	}

	// 执行事务更新
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tr.UpdateTaskByIDWithTx(ctx, tx, task.ID, &repository.UpdateTaskParam{
			Title:             newTitle,
			Priority:          newPriority,
			UpdateDescription: updateDescription,
			Description:       newDescription,
			UpdateDueDate:     updateDueDate,
			DueDate:           newDueDate,
			// 其余字段默认值表示不更新
		})
	})
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("update task base info failed: task not found when updating")
			return nil, ErrTaskNotFound
		}

		logger.Error("update task base info failed: update task transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	// 基于原任务构建返回字段，再覆盖本次更新后的字段
	baseField := buildTaskBaseFields(task)

	// 没有更新都返回原字段，更新了返回新字段
	if newTitle != nil {
		baseField.Title = *newTitle
	}
	if newPriority != nil {
		baseField.Priority = *newPriority
	}
	if updateDueDate {
		baseField.DueDate = newDueDate
	}

	// 描述未更新时返回原描述；描述已更新时返回新描述
	respDescription := utils.SafeStringValue(task.Description) // 原描述
	if updateDescription {
		respDescription = utils.SafeStringValue(newDescription) // 新描述
	}

	logger.Info("update task base info success",
		zap.Bool("update_title", newTitle != nil),
		zap.Bool("update_priority", newPriority != nil),
		zap.Bool("update_description", updateDescription),
		zap.Bool("update_due_date", updateDueDate),
	)

	return &dto.UpdateTaskBaseInfoResp{
		TaskBaseFields: baseField,
		Description:    respDescription,
		CreatorID:      task.CreatorID,
		StartTime:      task.StartTime,
		CompletedAt:    task.CompletedAt,
		SortOrder:      task.SortOrder,
	}, nil
}

// UpdateTaskStatusParam 更新任务状态参数
// 更新状态规则：
//   - "" 不更新，直接返回
//   - 其它，校验参数更新
type UpdateTaskStatusParam struct {
	UserID uint64
	TaskID uint64
	Status string
}

// UpdateTaskStatus 更新任务状态
// 注意：
//   - todo：任务回到待处理状态，如果原状态为 done，清空 completed_at
//   - in_progress：任务进入进行中，如果 start_time 为空，自动设置为当前时间；
//     如果原状态为 `done`，清空 `completed_at`
//   - done：任务完成，自动设置 completed_at 为当前时间
//   - cancelled：任务取消，不自动设置 completed_at
func (s *TaskService) UpdateTaskStatus(ctx context.Context, param *UpdateTaskStatusParam) (*dto.UpdateTaskStatusResp, error) {
	logger := zap.L()
	// 参数校验
	if param == nil {
		logger.Warn("update task status failed: invalid nil param")
		return nil, ErrInvalidTaskParam
	}
	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("task_id", param.TaskID),
	)
	if param.UserID == 0 || param.TaskID == 0 {
		logger.Warn("update task status failed: invalid user_id or task_id")
		return nil, ErrInvalidTaskParam
	}

	// 判断状态是否合法
	// 空状态在后面会返回之前的 task 状态
	if param.Status != "" && !isValidTaskStatus(param.Status) {
		logger.Warn("update task status failed: invalid task status",
			zap.String("status", param.Status),
		)
		return nil, ErrInvalidTaskStatus
	}

	// 从数据库中取数据判断当前身份是否合法
	// 查询任务信息，用于权限判断和状态流转判断
	task, err := s.tr.GetTaskByID(ctx, param.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("update task status failed: task not found")
			return nil, ErrTaskNotFound
		}

		logger.Error("update task status failed: get task error",
			zap.Error(err),
		)
		return nil, err
	}
	logger = logger.With(
		zap.Uint64("project_id", task.ProjectID),
		zap.String("old_status", task.Status),
		zap.String("new_status", param.Status),
	)

	// 权限规则：
	// - 任务创建人可以更新
	// - 任务负责人可以更新
	// - 项目 owner/admin 可以更新
	isCreator := task.CreatorID == param.UserID
	isAssignee := task.AssigneeID != nil && *task.AssigneeID == param.UserID
	hasPermission := isCreator || isAssignee
	if !hasPermission {
		hasPermission, err = hasProjectManagePermission(ctx, s.pmr, task.ProjectID, param.UserID, logger)
		if err != nil {
			logger.Warn("update task status failed: project manage permission check error",
				zap.Error(err),
			)
			return nil, err
		}
	}
	if !hasPermission {
		logger.Warn("update task status failed: permission denied",
			zap.Bool("is_creator", isCreator),
			zap.Bool("is_assignee", isAssignee),
		)
		return nil, ErrTaskForbidden
	}

	// 真正更新之前先检查：如果更新的状态为空/更新前后是同一个状态就直接返回，不更新
	if param.Status == "" || param.Status == task.Status {
		logger.Info("update task status skipped: status not changed")
		return &dto.UpdateTaskStatusResp{
			ID:          task.ID,
			ProjectID:   task.ProjectID,
			Title:       task.Title,
			Status:      task.Status,
			Priority:    task.Priority,
			AssigneeID:  task.AssigneeID,
			DueDate:     task.DueDate,
			StartTime:   task.StartTime,
			CompletedAt: task.CompletedAt,
			UpdatedAt:   task.UpdatedAt,
		}, nil
	}
	// 需要更新, 根据规则不同的更新方案
	now := time.Now()

	// 根据状态流转规则构造更新参数
	updateParam, err := buildTaskStatusUpdateParam(task, param.Status, now)
	if err != nil {
		logger.Warn("update task status failed: build update param error",
			zap.Error(err),
		)

		return nil, err
	}

	// 执行事务更新
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tr.UpdateTaskByIDWithTx(ctx, tx, task.ID, updateParam)
	})
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("update task status failed: task not found when updating")
			return nil, ErrTaskNotFound
		}
		logger.Error("update task status failed: update task transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	// 基于旧任务数据构造更新后的返回字段
	startTime := task.StartTime // 旧时间
	if updateParam.UpdateStartTime {
		startTime = updateParam.StartTime // 更新之后的新时间
	}

	completedAt := task.CompletedAt
	if updateParam.UpdateCompletedAt {
		completedAt = updateParam.CompletedAt
	}

	newStatus := task.Status
	if updateParam.Status != nil {
		newStatus = *updateParam.Status
	}

	return &dto.UpdateTaskStatusResp{
		ID:          task.ID,
		ProjectID:   task.ProjectID,
		Title:       task.Title,
		Status:      newStatus,
		Priority:    task.Priority,
		AssigneeID:  task.AssigneeID,
		DueDate:     task.DueDate,
		StartTime:   startTime,
		CompletedAt: completedAt,
		UpdatedAt:   now,
	}, nil
}

// UpdateTaskAssigneeParam 更新任务负责人参数
// 更新负责人规则：
//   - AssigneeID == nil：清空负责人，数据库 assignee_id 更新为 NULL
//   - AssigneeID != nil：更新为指定负责人，需要校验用户存在且是项目成员
type UpdateTaskAssigneeParam struct {
	UserID     uint64  // 当前操作用户ID
	TaskID     uint64  // 任务ID
	AssigneeID *uint64 // 新负责人ID，nil 表示取消负责人
}

// UpdateTaskAssignee 更新任务的负责人
func (s *TaskService) UpdateTaskAssignee(ctx context.Context, param *UpdateTaskAssigneeParam) (*dto.UpdateTaskAssigneeResp, error) {
	logger := zap.L()
	// 参数校验
	if param == nil {
		logger.Warn("update task assignee failed: invalid nil param")
		return nil, ErrInvalidTaskParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("task_id", param.TaskID),
	)

	if param.UserID == 0 || param.TaskID == 0 {
		logger.Warn("update task assignee failed: invalid user_id or task_id")
		return nil, ErrInvalidTaskParam
	}

	if param.AssigneeID != nil && *param.AssigneeID == 0 {
		logger.Warn("update task assignee failed: invalid assignee_id",
			zap.Uint64("assignee_id", *param.AssigneeID),
		)
		return nil, ErrInvalidTaskParam
	}

	// 从数据库中取数据判断当前身份是否合法
	// 查询任务信息，用于权限判断和构建返回数据
	task, err := s.tr.GetTaskByID(ctx, param.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("update task assignee failed: task not found")
			return nil, ErrTaskNotFound
		}

		logger.Error("update task assignee failed: get task error",
			zap.Error(err),
		)
		return nil, err
	}

	logger = logger.With(
		zap.Uint64("project_id", task.ProjectID),
		zap.Uint64("creator_id", task.CreatorID),
	)

	// 权限规则：
	// - 任务创建人可以更新
	// - 项目 owner/admin 可以更新
	isCreator := task.CreatorID == param.UserID
	hasPermission := isCreator

	if !hasPermission {
		hasPermission, err = hasProjectManagePermission(ctx, s.pmr, task.ProjectID, param.UserID, logger)
		if err != nil {
			logger.Warn("update task assignee failed: project manage permission check error",
				zap.Error(err),
			)
			return nil, err
		}
	}

	if !hasPermission {
		logger.Warn("update task assignee failed: permission denied",
			zap.Bool("is_creator", isCreator),
		)
		return nil, ErrTaskForbidden
	}

	// 看看负责人是否存在
	// 如果设置了新负责人，需要校验负责人是否存在且属于当前项目
	var assignee *model.User
	if param.AssigneeID != nil {
		assigneeLogger := logger.With(zap.Uint64("assignee_id", *param.AssigneeID))

		assignee, err = s.ur.GetByID(ctx, *param.AssigneeID)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				assigneeLogger.Warn("update task assignee failed: assignee user not found")
				return nil, ErrAssigneeNotFount
			}

			assigneeLogger.Error("update task assignee failed: get assignee user error",
				zap.Error(err),
			)
			return nil, err
		}

		// 负责人必须是项目成员
		isMember, err := s.pmr.ExistsByProjectIDAndUserID(ctx, task.ProjectID, assignee.ID)
		if err != nil {
			assigneeLogger.Error("update task assignee failed: check assignee project member error",
				zap.Error(err),
			)
			return nil, err
		}

		if !isMember {
			assigneeLogger.Warn("update task assignee failed: assignee is not project member")
			return nil, ErrAssigneeNotProjectMember
		}
	}

	// 正式更新前，先判断，如果更新后和更新前时同一个就不更新直接返回
	if isSameAssignee(task.AssigneeID, param.AssigneeID) {
		// 如果是同一个 assignee 不进行更新，直接返回
		logger.Info("update task assignee skipped: assignee not changed")
		return &dto.UpdateTaskAssigneeResp{
			ID:         task.ID,
			ProjectID:  task.ProjectID,
			Title:      task.Title,
			Status:     task.Status,
			Priority:   task.Priority,
			AssigneeID: task.AssigneeID,
			Assignee:   buildUserPublicProfile(task.Assignee),
			UpdatedAt:  task.UpdatedAt,
		}, nil
	}

	var newAssigneeID *uint64 // 复制一份负责人ID，避免直接持有入参指针
	if param.AssigneeID != nil {
		v := *param.AssigneeID
		newAssigneeID = &v
	}
	// 执行事务更新负责人
	now := time.Now()
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tr.UpdateTaskByIDWithTx(ctx, tx, task.ID, &repository.UpdateTaskParam{
			UpdateAssignee: true,
			AssigneeID:     newAssigneeID,
		})
	})
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("update task assignee failed: task not found when updating")
			return nil, ErrTaskNotFound
		}
		logger.Error("update task assignee failed: update task transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	logger.Info("update task assignee success",
		zap.Bool("clear_assignee", newAssigneeID == nil),
	)
	return &dto.UpdateTaskAssigneeResp{
		ID:         task.ID,
		ProjectID:  task.ProjectID,
		Title:      task.Title,
		Status:     task.Status,
		Priority:   task.Priority,
		AssigneeID: newAssigneeID,
		Assignee:   buildUserPublicProfile(assignee),
		UpdatedAt:  now,
	}, nil

}

// UpdateTaskSortOrderParam 更新任务排序值参数
type UpdateTaskSortOrderParam struct {
	UserID    uint64              // 当前操作用户ID
	ProjectID uint64              // 项目ID
	Items     []*dto.TaskSortItem // 排序项
}

// UpdateTaskSortOrder 更新任务的排序值操作
func (s *TaskService) UpdateTaskSortOrder(ctx context.Context, param *UpdateTaskSortOrderParam) (*dto.UpdateTaskSortOrderResp, error) {
	logger := zap.L()
	// 参数校验
	if param == nil {
		logger.Warn("update task sort order failed: invalid nil param")
		return nil, ErrInvalidTaskParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("project_id", param.ProjectID),
	)

	if param.UserID == 0 || param.ProjectID == 0 {
		logger.Warn("update task sort order failed: invalid user_id or project_id")
		return nil, ErrInvalidTaskParam
	}

	if len(param.Items) == 0 {
		logger.Warn("update task sort order failed: items is empty")
		return nil, ErrEmptyTaskSortItems
	}

	// 规范化排序项：
	// 1. 校验 nil item：为空的去除
	// 2. 校验 task_id：task_id 不能为 0
	// 		如果发生1、2返回 ErrInvalidTaskParam
	// 3. 根据 task_id 去重
	// 4. 如果同一个 task_id 重复出现，保留最后一次传入的 sort_order
	items, taskIDs, err := normalizeTaskSortItems(param.Items)
	if err != nil {
		logger.Warn("update task sort order failed: invalid sort items",
			zap.Error(err),
		)
		return nil, err
	}

	// 权限校验：
	// 只有项目 owner/admin 可以调整项目任务排序
	hasPermission, err := hasProjectManagePermission(ctx, s.pmr, param.ProjectID, param.UserID, logger)
	if err != nil {
		logger.Warn("update task sort order failed: project manage permission check error",
			zap.Error(err),
		)
		return nil, err
	}

	if !hasPermission {
		logger.Warn("update task sort order failed: permission denied")
		return nil, ErrTaskForbidden
	}

	// 校验所有 task_id 都属于当前 project_id
	existsCount, err := s.tr.CountTasksByProjectIDAndIDs(ctx, param.ProjectID, taskIDs)
	if err != nil {
		logger.Error("update task sort order failed: count project tasks error",
			zap.Error(err),
		)
		return nil, err
	}

	if existsCount != int64(len(taskIDs)) {
		logger.Warn("update task sort order failed: task not belong to project",
			zap.Int64("exists_count", existsCount),
			zap.Int("request_count", len(taskIDs)),
		)
		return nil, ErrInvalidTaskSortOrderItem
	}

	updatedAt := time.Now()

	// 执行事务批量更新排序值
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tr.BatchUpdateTaskSortWithTx(ctx, tx, param.ProjectID, toRepositoryTaskSortItems(items), updatedAt)
	})
	if err != nil {
		if errors.Is(err, repository.ErrTaskSortNoRowsUpdated) {
			logger.Warn("update task sort order failed: no rows updated")
			return nil, ErrTaskSortNoRowsUpdated
		}

		logger.Error("update task sort order failed: batch update transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	logger.Info("update task sort order success",
		zap.Int("item_count", len(items)),
	)

	return &dto.UpdateTaskSortOrderResp{
		Items:     items,
		UpdatedAt: updatedAt,
	}, nil
}

// RemoveTaskParam 软删除任务参数
type RemoveTaskParam struct {
	UserID uint64 // 当前操作用户ID
	TaskID uint64 // 任务ID
}

// RemoveTask 软删除任务
func (s *TaskService) RemoveTask(ctx context.Context, param *RemoveTaskParam) (*dto.RemoveTaskResp, error) {
	logger := zap.L()

	// 参数校验
	if param == nil {
		logger.Warn("delete task failed: invalid nil param")
		return nil, ErrInvalidTaskParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Uint64("task_id", param.TaskID),
	)

	if param.UserID == 0 || param.TaskID == 0 {
		logger.Warn("delete task failed: invalid user_id or task_id")
		return nil, ErrInvalidTaskParam
	}

	// 查询任务信息，用于权限判断
	task, err := s.tr.GetTaskByID(ctx, param.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("delete task failed: task not found")
			return nil, ErrTaskNotFound
		}

		logger.Error("delete task failed: get task error",
			zap.Error(err),
		)
		return nil, err
	}

	logger = logger.With(
		zap.Uint64("project_id", task.ProjectID),
		zap.Uint64("creator_id", task.CreatorID),
	)

	// 权限规则：
	// - 任务创建人可以删除
	// - 项目 owner/admin 可以删除
	isCreator := task.CreatorID == param.UserID
	hasPermission := isCreator

	if !hasPermission {
		hasPermission, err = hasProjectManagePermission(ctx, s.pmr, task.ProjectID, param.UserID, logger)
		if err != nil {
			logger.Warn("delete task failed: project manage permission check error",
				zap.Error(err),
			)
			return nil, err
		}
	}

	if !hasPermission {
		logger.Warn("delete task failed: permission denied",
			zap.Bool("is_creator", isCreator),
		)
		return nil, ErrTaskForbidden
	}

	// 执行事务软删除任务
	err = s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.tr.SoftDeleteTaskByIDWithTx(ctx, tx, task.ID)
	})
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			logger.Warn("delete task failed: task not found when deleting")
			return nil, ErrTaskNotFound
		}

		logger.Error("delete task failed: soft delete transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	logger.Info("delete task success")

	return &dto.RemoveTaskResp{}, nil
}

// taskSvcUserRepo 用户仓储接口
type taskSvcUserRepo interface {
	GetByID(ctx context.Context, id uint64) (*model.User, error)
	ExistsByUserID(ctx context.Context, userID uint64) (bool, error)
}

// taskSvcProjectRepo 项目仓储接口
type taskSvcProjectRepo interface {
	ExistsByProjectID(ctx context.Context, projectID uint64) (bool, error)
}

// taskSvcProjectMemberRepo 项目成员仓储接口
type taskSvcProjectMemberRepo interface {
	ExistsByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (bool, error)
	GetProjectMemberRoleByProjectIDAndUserID(ctx context.Context, projectID, userID uint64) (string, error)
}

// taskSvcTaskRepo 任务仓储接口
type taskSvcTaskrRepo interface {
	CreateWithTx(ctx context.Context, tx *gorm.DB, task *model.Task) error
	SearchTasks(ctx context.Context, query *repository.TaskSearchQuery) ([]*model.Task, int64, error)
	GetTaskByID(ctx context.Context, taskID uint64) (*model.Task, error)
	UpdateTaskByIDWithTx(ctx context.Context, tx *gorm.DB, taskID uint64, param *repository.UpdateTaskParam) error
	BatchUpdateTaskSortWithTx(ctx context.Context,
		tx *gorm.DB,
		projectID uint64,
		items []*repository.TaskSortItem,
		updatedAt time.Time,
	) error
	SoftDeleteTaskByIDWithTx(ctx context.Context, tx *gorm.DB, taskID uint64) error

	CountTasksByProjectIDAndIDs(ctx context.Context, projectID uint64, taskIDs []uint64) (int64, error)
}

// isSameAssignee 判断新旧 assginee 是否是同一个
func isSameAssignee(oldAssigneeID, newAssigneeID *uint64) bool {
	if oldAssigneeID == nil && newAssigneeID == nil {
		return true
	}

	if oldAssigneeID == nil || newAssigneeID == nil {
		return false
	}

	return *oldAssigneeID == *newAssigneeID
}

// buildTaskStatusUpdateParam 根据任务状态流转规则构建 repository 更新参数。
// 字段语义：
//   - Status：始终更新
//   - UpdateStartTime：true 表示需要更新 start_time
//   - StartTime：非 nil 表示设置开始时间；nil 表示清空开始时间
//   - UpdateCompletedAt：true 表示需要更新 completed_at
//   - CompletedAt：非 nil 表示设置完成时间；nil 表示清空完成时间
func buildTaskStatusUpdateParam(task *model.Task, targetStatus string, now time.Time) (*repository.UpdateTaskParam, error) {
	updateParam := &repository.UpdateTaskParam{
		Status: utils.StringPtr(targetStatus),
	}

	// 根据目标状态处理关联时间字段
	switch targetStatus {
	case model.TaskStatusTodo:
		// done -> todo：清空 completed_at
		if task.Status == model.TaskStatusDone {
			updateParam.UpdateCompletedAt = true
			updateParam.CompletedAt = nil
		}

	case model.TaskStatusInProgress:
		// 首次进入进行中：如果 start_time 为空，则设置为当前时间
		if task.StartTime == nil {
			updateParam.UpdateStartTime = true
			updateParam.StartTime = utils.TimePtr(now)
		}

		// done -> in_progress：清空 completed_at
		if task.Status == model.TaskStatusDone {
			updateParam.UpdateCompletedAt = true
			updateParam.CompletedAt = nil
		}

	case model.TaskStatusDone:
		// 进入完成状态：设置 completed_at 为当前时间
		updateParam.UpdateCompletedAt = true
		updateParam.CompletedAt = utils.TimePtr(now)

		// 如果任务从未开始，也可以顺手补充 start_time
		if task.StartTime == nil {
			updateParam.UpdateStartTime = true
			updateParam.StartTime = utils.TimePtr(now)
		}

	case model.TaskStatusCancelled:
		// cancelled：只更新 status，不自动处理 completed_at
	}

	return updateParam, nil
}

// normalizeTaskSortItems 规范化任务排序项
// 规则：
//   - item 不能为 nil
//   - task_id 不能为 0
//   - 相同 task_id 重复出现时，保留最后一次的 sort_order
//   - 返回值保持最终有效项的顺序，顺序以最后一次出现的位置为准
func normalizeTaskSortItems(items []*dto.TaskSortItem) ([]*dto.TaskSortItem, []uint64, error) {
	if len(items) == 0 {
		return nil, nil, ErrEmptyTaskSortItems
	}

	itemMap := make(map[uint64]*dto.TaskSortItem, len(items))
	order := make([]uint64, 0, len(items))

	for _, item := range items {
		if item == nil || item.TaskID == 0 {
			return nil, nil, ErrInvalidTaskParam
		}

		// 如果重复出现，先删除旧顺序
		// 后面重新追加，保证顺序以最后一次出现为准
		if _, exists := itemMap[item.TaskID]; exists {
			order = utils.RemoveTarget(order, item.TaskID)
		}

		taskID := item.TaskID
		sortOrder := item.SortOrder

		// 复制，防止指向同一个 struct
		itemMap[taskID] = &dto.TaskSortItem{
			TaskID:    taskID,
			SortOrder: sortOrder,
		}
		order = append(order, taskID)
	}

	normalizedItems := make([]*dto.TaskSortItem, 0, len(order))
	taskIDs := make([]uint64, 0, len(order))

	for _, taskID := range order {
		normalizedItems = append(normalizedItems, itemMap[taskID])
		taskIDs = append(taskIDs, taskID)
	}

	return normalizedItems, taskIDs, nil
}

// toRepositoryTaskSortItems 将 dto 排序项转换为 repository 排序项
func toRepositoryTaskSortItems(items []*dto.TaskSortItem) []*repository.TaskSortItem {
	result := make([]*repository.TaskSortItem, 0, len(items))

	for _, item := range items {
		result = append(result, &repository.TaskSortItem{
			TaskID:    item.TaskID,
			SortOrder: item.SortOrder,
		})
	}

	return result
}
