// internal/repository/notification_repo.go
// Package repository
// 实现 notifications 表的仓储操作

package repository

import (
	"context"
	"errors"
	"smart-task-platform/internal/model"
	"time"

	"gorm.io/gorm"
)

var (
	ErrInvalidNotificationParam = errors.New("invalid notification param")          // 不合法的通知参数
	ErrCreateNotificationEmpty  = errors.New("create notification params is empty") // 创建通知参数为空
	ErrNotificationNotFound     = errors.New("notification not found")              // 通知不存在/未找到
)

// notificationRepository 通知仓储
type notificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository 创建通知仓储
func NewNotificationRepository(db *gorm.DB) *notificationRepository {
	return &notificationRepository{
		db: db,
	}
}

// CreateWithTx 在通知表中插入一条数据
//
// 用于业务事务：创建任务动态 + 创建通知 + 创建 outbox message
func (r *notificationRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, notification *model.Notification) error {
	if notification == nil {
		return ErrCreateNotificationEmpty
	}

	return getDB(ctx, r.db, tx).
		Create(notification).Error
}

// SearchNotificationQuery 通知列表查询参数
//
// 用于接口：GET /api/v1/notifications
type SearchNotificationQuery struct {
	SearchQuery
	UserID uint64 // 接收通知用户ID
	IsRead *bool  // 是否已读，nil 表示全部
}

// SearchNotificationResult 通知搜索结果
type SearchNotificationResult = SearchResult[*model.Notification]

// SearchNotifications 查询用户通知列表
//
// 用于接口：GET /api/v1/notifications
func (r *notificationRepository) SearchNotifications(ctx context.Context, query *SearchNotificationQuery) (*SearchNotificationResult, error) {
	if query == nil || query.UserID == 0 {
		return nil, ErrInvalidNotificationParam
	}

	db := getDB(ctx, r.db, nil).
		Model(&model.Notification{}).
		Where(model.NotificationColumnUserID+" = ?", query.UserID)

	if query.IsRead != nil {
		db = db.Where(model.NotificationColumnIsRead+" = ?", *query.IsRead)
	}

	result := &SearchNotificationResult{
		List:    []*model.Notification{},
		Total:   nil,
		HasMore: false,
	}

	// 只有 need_total=true 时才执行 COUNT(*)
	if query.NeedTotal {
		var total int64
		if err := db.Count(&total).Error; err != nil {
			return nil, err
		}

		result.Total = &total
		if total == 0 {
			return result, nil
		}
	}

	offset := (query.Page - 1) * query.PageSize
	limit := query.PageSize + 1 // 多查一条判断是否还有下一页

	notifications := make([]*model.Notification, 0, query.PageSize)

	err := db.
		Order(model.NotificationColumnCreatedAt + " DESC").
		Order(model.NotificationColumnID + " DESC").
		Offset(offset).
		Limit(limit).
		Find(&notifications).Error
	if err != nil {
		return nil, err
	}

	if len(notifications) > query.PageSize {
		result.HasMore = true
		notifications = notifications[:query.PageSize]
	}

	result.List = notifications

	return result, nil
}

// CountUnreadByUserID 统计用户未读通知数量
//
// 用于接口：GET /api/v1/notifications/unread-count
func (r *notificationRepository) CountUnreadByUserID(ctx context.Context, userID uint64) (int64, error) {
	if userID == 0 {
		return 0, ErrInvalidNotificationParam
	}

	var count int64

	err := getDB(ctx, r.db, nil).
		Model(&model.Notification{}).
		Where(model.NotificationColumnUserID+" = ?", userID).
		Where(model.NotificationColumnIsRead+" = ?", false).
		Count(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
}

// MarkAsRead 标记单条通知为已读
func (r *notificationRepository) MarkAsRead(ctx context.Context, userID uint64, notificationID uint64) error {
	if userID == 0 || notificationID == 0 {
		return ErrInvalidNotificationParam
	}

	now := time.Now()

	result := getDB(ctx, r.db, nil).
		Model(&model.Notification{}).
		Where(model.NotificationColumnID+" = ?", notificationID).
		Where(model.NotificationColumnUserID+" = ?", userID).
		Updates(map[string]interface{}{
			model.NotificationColumnIsRead:    true,
			model.NotificationColumnReadAt:    now,
			model.NotificationColumnUpdatedAt: now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}

	return nil
}

// MarkAllAsRead 标记用户所有通知为已读
func (r *notificationRepository) MarkAllAsRead(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return ErrInvalidNotificationParam
	}

	now := time.Now()

	return getDB(ctx, r.db, nil).
		Model(&model.Notification{}).
		Where(model.NotificationColumnUserID+" = ?", userID).
		Where(model.NotificationColumnIsRead+" = ?", false).
		Updates(map[string]interface{}{
			model.NotificationColumnIsRead:    true,
			model.NotificationColumnReadAt:    now,
			model.NotificationColumnUpdatedAt: now,
		}).Error
}

// SoftDeleteByID 软删除用户通知
//
// 用于接口：DELETE /api/v1/notifications/:id
func (r *notificationRepository) SoftDeleteByID(ctx context.Context, userID uint64, notificationID uint64) error {
	if userID == 0 || notificationID == 0 {
		return ErrInvalidNotificationParam
	}

	now := time.Now()

	result := getDB(ctx, r.db, nil).
		Model(&model.Notification{}).
		Where(model.NotificationColumnID+" = ?", notificationID).
		Where(model.NotificationColumnUserID+" = ?", userID).
		Updates(map[string]interface{}{
			model.NotificationColumnDeletedAt: now,
			model.NotificationColumnUpdatedAt: now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}

	return nil
}
