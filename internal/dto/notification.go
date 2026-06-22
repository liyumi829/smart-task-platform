// internal/dto/notification.go
// Package dto
// 通知传输对象

package dto

import "time"

// NotificationIDUri 通知路径 uri id
type NotificationIDUri struct {
	NotificationID uint64 `uri:"notificationId"`
}

// ListNotificationsQuery 获取通知列表参数
type ListNotificationsQuery struct {
	PageQuery
	IsRead *bool `json:"is_read"`
}

// NotificationListItem 任务动态表项
type NotificationListItem struct {
	ID        uint64    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// ListNotificationsResp 通知列表响应
type ListNotificationsResp = PageResp[*NotificationListItem]

// GetUnReadCountResp 未读的条数
type GetUnReadCountResp struct {
	UnreadCount int64 `json:"unread_count"`
}

// MarkNotificationAsReadResp 读取一条通知响应
type MarkNotificationAsReadResp struct {
	ID     uint64 `json:"id"`
	IsRead bool   `json:"is_read"`
}

// MarkAllNotificationsAsReadResp 读取所有未读通知响应
type MarkAllNotificationsAsReadResp struct{}

// ForwardMessage 转发的消息结构
type ForwardMessage struct {
	NotificationID uint64  `json:"notification_id"`        // 该通知的ID
	UserID         uint64  `json:"user_id"`                // 接收者的ID
	SenderID       *uint64 `json:"sender_id,omitempty"`    // 发送者的ID
	Type           string  `json:"type"`                   // 通知类型
	Title          string  `json:"title"`                  // 标题
	Content        string  `json:"content"`                // 通知内容
	RelatedType    *string `json:"related_type,omitempty"` // 关联类型
	RelatedID      *uint64 `json:"related_id,omitempty"`   // 关联ID
	CreatedAt      string  `json:"created_at"`             // 创建时间
}
