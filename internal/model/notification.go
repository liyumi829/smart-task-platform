// internal/model/notification.go
// Package model
// 通知表 GORM 映射对象

package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	NotificationTableName = "notifications" // 表名

	NotificationColumnID          = "id"           // 通知ID
	NotificationColumnUserID      = "user_id"      // 接收通知用户ID
	NotificationColumnSenderID    = "sender_id"    // 发送人/触发人ID
	NotificationColumnType        = "type"         // 通知类型
	NotificationColumnTitle       = "title"        // 通知标题
	NotificationColumnContent     = "content"      // 通知内容
	NotificationColumnIsRead      = "is_read"      // 是否已读
	NotificationColumnReadAt      = "read_at"      // 阅读时间
	NotificationColumnRelatedType = "related_type" // 关联资源类型
	NotificationColumnRelatedID   = "related_id"   // 关联资源ID
	NotificationColumnCreatedAt   = "created_at"   // 创建时间
	NotificationColumnUpdatedAt   = "updated_at"   // 更新时间
	NotificationColumnDeletedAt   = "deleted_at"   // 软删除时间
)

const (
	NotificationTypeTaskAssigned      = "task_assigned"       // 任务指派
	NotificationTypeTaskStatusChanged = "task_status_changed" // 任务状态变更
	NotificationTypeCommentReply      = "comment_reply"       // 评论回复
	//NotificationTypeTaskDeleted          = "task_deleted"           // 任务删除
	//NotificationTypeTaskDueReminded      = "task_due_reminded"      // 任务截止提醒
	//NotificationTypeProjectMemberAdded   = "project_member_added"   // 项目成员添加
	//NotificationTypeProjectMemberRemoved = "project_member_removed" // 项目成员移除
	//NotificationTypeProjectRoleChanged   = "project_role_changed"   // 项目权限变更
	//NotificationTypeSystem               = "system"                 // 系统通知
)

// Notification 通知表
type Notification struct {
	ID          uint64         `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID      uint64         `gorm:"column:user_id;not null;index:idx_notifications_user_created,priority:1;index:idx_notifications_user_read_created,priority:1" json:"user_id"`
	SenderID    *uint64        `gorm:"column:sender_id;index:idx_notifications_sender_id" json:"sender_id"`
	Type        string         `gorm:"column:type;type:varchar(50);not null;index:idx_notifications_type_created,priority:1" json:"type"`
	Title       string         `gorm:"column:title;type:varchar(200);not null" json:"title"`
	Content     string         `gorm:"column:content;type:varchar(500);not null" json:"content"`
	IsRead      bool           `gorm:"column:is_read;not null;default:false;index:idx_notifications_user_read_created,priority:2" json:"is_read"`
	ReadAt      *time.Time     `gorm:"column:read_at" json:"read_at"`
	RelatedType *string        `gorm:"column:related_type;type:varchar(50);index:idx_notifications_related,priority:1" json:"related_type"`
	RelatedID   *uint64        `gorm:"column:related_id;index:idx_notifications_related,priority:2" json:"related_id"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;autoCreateTime;index:idx_notifications_user_created,priority:2;index:idx_notifications_user_read_created,priority:3;index:idx_notifications_type_created,priority:2" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index:idx_notifications_deleted_at" json:"deleted_at"`
}

// TableName 指定表名
func (Notification) TableName() string {
	return NotificationTableName
}
