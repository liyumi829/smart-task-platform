// internal/model/comment.go

// Package model 定义了任务评论相关的数据模型和数据库交互逻辑。
package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	TaskCommentTableName = "task_comments" // 表名
)

const (
	TaskCommentColumnID            = "id"               // 评论 ID
	TaskCommentColumnTaskID        = "task_id"          // 任务 ID
	TaskCommentColumnAuthorID      = "author_id"        // 评论人 ID
	TaskCommentColumnParentID      = "parent_id"        // 父评论 ID
	TaskCommentColumnReplyToUserID = "reply_to_user_id" // 被回复用户 ID
	TaskCommentColumnContent       = "content"          // 评论内容
	TaskCommentColumnCreatedAt     = "created_at"       // 创建时间
	TaskCommentColumnUpdatedAt     = "updated_at"       // 更新时间
	TaskCommentColumnDeletedAt     = "deleted_at"       // 软删除时间
)

const (
	TaskCommentAssocAuthor      = "Author"      // Gorm 关联：评论人
	TaskCommentAssocReplyToUser = "ReplyToUser" // Gorm 关联：被回复用户
)

// TaskComment 任务评论模型
type TaskComment struct {
	// 主键 ID
	ID uint64 `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement;comment:评论 ID" json:"id"`

	// 任务 ID
	TaskID uint64 `gorm:"column:task_id;type:bigint unsigned;not null;index:idx_task_id;index:idx_task_created_at,priority:1;comment:任务 ID" json:"task_id"`

	// 评论人 ID
	AuthorID uint64 `gorm:"column:author_id;type:bigint unsigned;not null;index:idx_author_id;comment:评论人 ID" json:"author_id"`

	// 父评论 ID，支持回复
	ParentID *uint64 `gorm:"column:parent_id;type:bigint unsigned;default:null;index:idx_parent_id;comment:父评论 ID，支持回复" json:"parent_id"`

	// 被回复用户 ID
	ReplyToUserID *uint64 `gorm:"column:reply_to_user_id;type:bigint unsigned;default:null;index:idx_reply_to_user_id;comment:被回复用户 ID" json:"reply_to_user_id"`

	// 评论内容
	Content string `gorm:"column:content;type:text;not null;comment:评论内容" json:"content"`

	// 创建时间
	CreatedAt time.Time `gorm:"column:created_at;type:datetime;not null;autoCreateTime;index:idx_task_created_at,priority:2;comment:创建时间" json:"created_at"`

	// 更新时间
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime;not null;autoUpdateTime;comment:更新时间" json:"updated_at"`

	// 软删除时间
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index:idx_deleted_at;comment:软删除时间" json:"-"`

	// Author 评论人信息，仅用于 Gorm 预加载
	Author *User `gorm:"foreignKey:AuthorID;references:ID" json:"-"`

	// ReplyToUser 被回复用户信息，仅用于 Gorm 预加载
	ReplyToUser *User `gorm:"foreignKey:ReplyToUserID;references:ID" json:"-"`
}

// TableName 指定表名
func (TaskComment) TableName() string {
	return TaskCommentTableName
}
