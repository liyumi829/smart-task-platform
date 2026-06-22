// internal/service/notification_service.go
// Package service
// 通知服务
package service

import (
	"context"
	"errors"
	"fmt"
	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// notificationSvcActivityRepo 任务活动接口
type notificationSvcActivityRepo interface {
	CreateWithTx(ctx context.Context, tx *gorm.DB, notification *model.Notification) error
	SearchNotifications(ctx context.Context, query *repository.SearchNotificationQuery) (*repository.SearchNotificationResult, error)
	CountUnreadByUserID(ctx context.Context, userID uint64) (int64, error)
	MarkAsRead(ctx context.Context, userID uint64, notificationID uint64) error
	MarkAllAsRead(ctx context.Context, userID uint64) error
}

// notificationSvcOutboxRepo Outbox接口
type notificationSvcOutboxRepo interface {
	CreateWithTx(ctx context.Context, tx *gorm.DB, message *model.OutboxMessage) error
}

// NotificationService 任务活动服务
type NotificationService struct {
	txMgr  *repository.TxManager       // 事务管理器
	nr     notificationSvcActivityRepo // 任务活动接口
	or     notificationSvcOutboxRepo   // Outbox接口
	logger *zap.Logger
}

// NewNotificationService 创建任务活动服务实例
func NewNotificationService(
	txMgr *repository.TxManager,
	notificationRepo notificationSvcActivityRepo,
	outboxRepo notificationSvcOutboxRepo,
	logger *zap.Logger,
) *NotificationService {

	if logger == nil {
		logger = zap.L()
	}

	return &NotificationService{
		txMgr:  txMgr,
		nr:     notificationRepo,
		or:     outboxRepo,
		logger: logger,
	}
}

//======
// 业务接口
//======

// ListNotificationsParam 获取用户的通知列表参数
type ListNotificationsParam struct {
	UserID    uint64
	Page      int
	PageSize  int
	NeedTotal bool
	IsRead    *bool
}

// ListNotifications 获取某个用户的通知列表
func (s *NotificationService) ListNotifications(ctx context.Context, param *ListNotificationsParam) (*dto.ListNotificationsResp, error) {
	logger := zap.L()

	// 参数校验
	if param == nil {
		logger.Warn("list notifications failed: invalid nil param")
		return nil, ErrInvalidNotificationParam
	}

	logger = logger.With(
		zap.Uint64("user_id", param.UserID),
		zap.Bool("need_total", param.NeedTotal),
	)

	if param.UserID == 0 {
		logger.Warn("list notifications failed: invalid user_id")
		return nil, ErrInvalidNotificationParam
	}

	// 参数修正
	page, pageSize := fixPageParams(param.Page, param.PageSize)
	logger = logger.With(
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
	)

	// 这里不需要进行验证
	// 直接进行搜索
	result, err := s.nr.SearchNotifications(ctx, &repository.SearchNotificationQuery{
		IsRead: param.IsRead,
		SearchQuery: repository.SearchQuery{
			Page:      page,
			PageSize:  pageSize,
			NeedTotal: param.NeedTotal,
		},
	})
	if err != nil {
		logger.Error("list notifications failed: search notifications error",
			zap.Error(err),
		)
		return nil, err
	}

	// 搜索成功，构造list
	list := make([]*dto.NotificationListItem, 0, len(result.List))
	for _, notification := range result.List {
		list = append(list, buildNotificationItem(notification))
	}

	logger.Info("list notifications success",
		zap.Int("list_count", len(list)),
		zap.Bool("has_more", result.HasMore),
	)

	return &dto.ListNotificationsResp{
		List:     list,
		Total:    utils.SafePtrClone(result.Total),
		Page:     page,
		PageSize: pageSize,
		HasMore:  result.HasMore,
	}, nil
}

// GetUnreadCount 获取未读的条数
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID uint64) (*dto.GetUnReadCountResp, error) {
	logger := zap.L()

	// 参数校验
	if userID == 0 {
		logger.Warn("get notification unread count failed: invalid user_id")
		return nil, ErrInvalidNotificationParam
	}

	// 日志添加入参
	logger = logger.With(zap.Uint64("user_id", userID))

	// 直接搜索
	count, err := s.nr.CountUnreadByUserID(ctx, userID)
	if err != nil {
		logger.Error("get notification unread count failed: count unread error",
			zap.Error(err),
		)
		return nil, err
	}

	// 成功日志
	logger.Info("get notification unread count success",
		zap.Int64("unread_count", count),
	)

	return &dto.GetUnReadCountResp{
		UnreadCount: count,
	}, nil
}

// MarkAsRead 标记某个通知为已读
func (s *NotificationService) MarkAsRead(ctx context.Context, userID uint64, notificationID uint64) (*dto.MarkNotificationAsReadResp, error) {
	logger := zap.L()

	// 参数校验
	if userID == 0 || notificationID == 0 {
		logger.Warn("mark notification as read failed: invalid user_id or notification_id")
		return nil, ErrInvalidNotificationParam
	}

	// 日志添加入参
	logger = logger.With(
		zap.Uint64("user_id", userID),
		zap.Uint64("notification_id", notificationID),
	)

	// 事务执行标记已读
	err := s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.nr.MarkAsRead(ctx, userID, notificationID)
	})
	if err != nil {
		logger.Error("mark notification as read failed: transaction error",
			zap.Error(err),
		)

		if errors.Is(err, repository.ErrNotificationNotFound) {
			return nil, ErrNotificationNotFound
		}
		return nil, err
	}

	// 成功日志
	logger.Info("mark notification as read success")

	return &dto.MarkNotificationAsReadResp{
		ID:     notificationID,
		IsRead: true,
	}, nil
}

// MarkAllAsRead 标记所有通知为已读
func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID uint64) (*dto.MarkNotificationAsReadResp, error) {
	logger := zap.L()

	// 参数校验
	if userID == 0 {
		logger.Warn("mark all notifications as read failed: invalid user_id")
		return nil, ErrInvalidNotificationParam
	}

	// 日志添加入参
	logger = logger.With(zap.Uint64("user_id", userID))

	// 事务执行全部已读
	err := s.txMgr.Transaction(ctx, func(tx *gorm.DB) error {
		return s.nr.MarkAllAsRead(ctx, userID)
	})
	if err != nil {
		logger.Error("mark all notifications as read failed: transaction error",
			zap.Error(err),
		)
		return nil, err
	}

	// 成功日志
	logger.Info("mark all notifications as read success")

	return &dto.MarkNotificationAsReadResp{}, nil
}

//=========
// 服务内部使用接口
//==========

type RecordTaskAssignedRequest struct {
	TaskID       uint64
	ProjectID    uint64
	AssigneeID   uint64
	TaskTitle    string
	OperatorID   uint64
	OperatorName string
}

// RecordTaskAssigned 创建任务指派通知，并写入 Outbox
func (s *NotificationService) RecordTaskAssigned(ctx context.Context, tx *gorm.DB, req *RecordTaskAssignedRequest) error {
	logger := s.logger.With(
		zap.Uint64("task_id", req.TaskID),
		zap.Uint64("assignee_id", req.AssigneeID),
		zap.Uint64("operator_id", req.OperatorID),
	)
	if tx == nil {
		logger.Warn("notification.record_task_assigned.invalid_tx")
		return fmt.Errorf("invalid transaction")
	}
	if err := validateRecordTaskAssignedNotificationRequest(req); err != nil {
		logger.Warn("notification.record_task_assigned.invalid_request",
			zap.Error(err),
		)
		return err
	}

	logger.Info("notification.record_task_assigned.start")

	notification := &model.Notification{
		UserID:      req.AssigneeID,
		SenderID:    utils.SafeGetPtr(req.OperatorID),
		Type:        model.NotificationTypeTaskAssigned,
		Title:       "You've received a new task",
		Content:     fmt.Sprintf("'%s' assigned the task '%s' to you", req.OperatorName, req.TaskTitle),
		IsRead:      false,
		RelatedType: utils.StringPtr(model.RelatedTypeTask),
		RelatedID:   utils.SafeGetPtr(req.TaskID),
	}

	if err := s.nr.CreateWithTx(ctx, tx, notification); err != nil {
		return err
	}

	outboxMessage, err := buildNotificationOutboxMessage(model.OutboxEventTypeTaskAssigned, notification)
	if err != nil {
		logger.Error("notification.record_task_assigned.build_outbox_failed",
			zap.Error(err),
			zap.Uint64("notification_id", notification.ID))

		return err
	}

	if err := s.or.CreateWithTx(ctx, tx, outboxMessage); err != nil {
		return err
	}

	logger.Info("notification.record_task_assigned.success")

	return nil
}

type RecordTaskStatusChangedRequest struct {
	TaskID       uint64
	ProjectID    uint64
	OperatorID   uint64
	ReceiverID   uint64
	TaskTitle    string
	OperatorName string
	OldStatus    string
	NewStatus    string
}

// RecordTaskStatusChanged 创建任务状态变更通知，并写入 Outbox
func (s *NotificationService) RecordTaskStatusChanged(ctx context.Context, tx *gorm.DB, req *RecordTaskStatusChangedRequest) error {
	logger := s.logger.With(
		zap.Uint64("task_id", req.TaskID),
		zap.Uint64("receiver_id", req.ReceiverID),
		zap.Uint64("operator_id", req.OperatorID),
		zap.String("old_status", req.OldStatus),
		zap.String("new_status", req.NewStatus),
	)

	if tx == nil {
		logger.Warn("notification.record_task_status_changed.invalid_tx")
		return fmt.Errorf("invalid transaction")
	}
	if err := validateRecordTaskStatusChangedNotificationRequest(req); err != nil {
		logger.Warn("notification.record_task_status_changed.invalid_request",
			zap.Error(err),
		)
		return err
	}

	logger.Info("notification.record_task_status_changed.start")

	notification := &model.Notification{
		UserID:      req.ReceiverID,
		SenderID:    utils.SafeGetPtr(req.OperatorID),
		Type:        model.NotificationTypeTaskStatusChanged,
		Title:       "Task status updated",
		Content:     fmt.Sprintf("'%s' changed the status of task '%s' from '%s' to '%s'", req.OperatorName, req.TaskTitle, req.OldStatus, req.NewStatus),
		IsRead:      false,
		RelatedType: utils.StringPtr(model.RelatedTypeTask),
		RelatedID:   utils.SafeGetPtr(req.TaskID),
	}

	if err := s.nr.CreateWithTx(ctx, tx, notification); err != nil {
		return err
	}

	outboxMessage, err := buildNotificationOutboxMessage(model.OutboxEventTypeTaskStatusChanged, notification)
	if err != nil {
		logger.Error("notification.record_task_status_changed.build_outbox_failed",
			zap.Error(err),
			zap.Uint64("notification_id", notification.ID),
		)
		return err
	}

	if err := s.or.CreateWithTx(ctx, tx, outboxMessage); err != nil {
		return err
	}
	logger.Info("notification.record_task_status_changed.success")
	return nil
}

type RecordTaskCommentCreatedRequest struct {
	TaskID       uint64
	ProjectID    uint64
	CommentID    uint64
	OperatorID   uint64
	ReceiverID   uint64
	TaskTitle    string
	OperatorName string
	Content      string
	isReply      bool // 是回复某人还是在任务下评论
}

// RecordTaskCommentCreated 创建任务评论通知，并写入 Outbox
func (s *NotificationService) RecordTaskCommentCreated(ctx context.Context, tx *gorm.DB, req *RecordTaskCommentCreatedRequest) error {
	logger := s.logger.With(
		zap.Uint64("task_id", req.TaskID),
		zap.Uint64("comment_id", req.CommentID),
		zap.Uint64("receiver_id", req.ReceiverID),
		zap.Uint64("operator_id", req.OperatorID),
		zap.Bool("is_reply", req.isReply),
	)
	if tx == nil {
		logger.Warn("notification.record_task_comment_created.invalid_tx")
		return fmt.Errorf("invalid transaction")
	}
	if err := validateRecordTaskCommentCreatedNotificationRequest(req); err != nil {
		logger.Warn("notification.record_task_comment_created.invalid_request",
			zap.Error(err),
		)
		return err
	}

	logger.Info("notification.record_task_comment_created.start")

	notification := &model.Notification{
		UserID:      req.ReceiverID,
		SenderID:    utils.SafeGetPtr(req.OperatorID),
		Type:        model.NotificationTypeCommentReply,
		Title:       "New comment on task",
		Content:     fmt.Sprintf("'%s' commented on task '%s': %s", req.OperatorName, req.TaskTitle, req.Content),
		IsRead:      false,
		RelatedType: utils.StringPtr(model.RelatedTypeTask),
		RelatedID:   utils.SafeGetPtr(req.TaskID),
	}

	if err := s.nr.CreateWithTx(ctx, tx, notification); err != nil {
		return err
	}

	var outboxMessage *model.OutboxMessage
	var err error
	if !req.isReply {
		outboxMessage, err = buildNotificationOutboxMessage(model.OutboxEventTypeCommentCreated, notification)
	} else {
		outboxMessage, err = buildNotificationOutboxMessage(model.OutboxEventTypeCommentReply, notification)
	}
	if err != nil {
		logger.Error("notification.record_task_comment_created.build_outbox_failed",
			zap.Error(err),
			zap.Uint64("notification_id", notification.ID),
		)
		return err
	}

	if err := s.or.CreateWithTx(ctx, tx, outboxMessage); err != nil {
		return err
	}

	logger.Info("notification.record_task_comment_created.success")
	return nil
}

func validateRecordTaskAssignedNotificationRequest(req *RecordTaskAssignedRequest) error {
	if req.TaskID == 0 {
		return fmt.Errorf("task_id is required")
	}
	if req.AssigneeID == 0 {
		return fmt.Errorf("assignee_id is required")
	}
	if req.OperatorID == 0 {
		return fmt.Errorf("operator_id is required")
	}
	if strings.TrimSpace(req.TaskTitle) == "" {
		return fmt.Errorf("task_title is required")
	}
	if strings.TrimSpace(req.OperatorName) == "" {
		return fmt.Errorf("operator_name is required")
	}

	return nil
}

func validateRecordTaskStatusChangedNotificationRequest(req *RecordTaskStatusChangedRequest) error {
	if req.TaskID == 0 {
		return fmt.Errorf("task_id is required")
	}
	if req.ReceiverID == 0 {
		return fmt.Errorf("receiver_id is required")
	}
	if req.OperatorID == 0 {
		return fmt.Errorf("operator_id is required")
	}
	if strings.TrimSpace(req.TaskTitle) == "" {
		return fmt.Errorf("task_title is required")
	}
	if strings.TrimSpace(req.OperatorName) == "" {
		return fmt.Errorf("operator_name is required")
	}
	if strings.TrimSpace(req.OldStatus) == "" {
		return fmt.Errorf("old_status is required")
	}
	if strings.TrimSpace(req.NewStatus) == "" {
		return fmt.Errorf("new_status is required")
	}

	return nil
}

func validateRecordTaskCommentCreatedNotificationRequest(req *RecordTaskCommentCreatedRequest) error {
	if req.TaskID == 0 {
		return fmt.Errorf("task_id is required")
	}
	if req.CommentID == 0 {
		return fmt.Errorf("comment_id is required")
	}
	if req.ReceiverID == 0 {
		return fmt.Errorf("receiver_id is required")
	}
	if req.OperatorID == 0 {
		return fmt.Errorf("operator_id is required")
	}
	if strings.TrimSpace(req.TaskTitle) == "" {
		return fmt.Errorf("task_title is required")
	}
	if strings.TrimSpace(req.OperatorName) == "" {
		return fmt.Errorf("operator_name is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return fmt.Errorf("content is required")
	}

	return nil
}
