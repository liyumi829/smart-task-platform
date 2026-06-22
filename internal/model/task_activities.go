// internal/model/task_activities.go
// Package dto
// 任务动态表的存储模块

package model

import (
	"time"

	"gorm.io/datatypes"
)

// TaskActivity 任务动态表 表名与列名常量

const (
	TaskActivityTableName         = "task_activities" // 表名
	TaskActivityColumnID          = "id"              // 任务动态ID
	TaskActivityColumnTaskID      = "task_id"         // 任务ID
	TaskActivityColumnProjectID   = "project_id"      // 项目ID
	TaskActivityColumnOperatorID  = "operator_id"     // 操作人ID
	TaskActivityColumnAction      = "action"          // 动作类型
	TaskActivityColumnContent     = "content"         // 动态内容
	TaskActivityColumnRelatedType = "related_type"    // 关联资源类型
	TaskActivityColumnRelatedID   = "related_id"      // 关联资源ID
	TaskActivityColumnExtraJSON   = "extra_json"      // 扩展字段
	TaskActivityColumnCreatedAt   = "created_at"      // 创建时间
)

// ActivityAction 任务动态动作类型

const (
	ActivityActionTaskCreated       = "task_created"        // 任务创建
	ActivityActionTaskUpdated       = "task_updated"        // 任务更新
	ActivityActionTaskAssigned      = "task_assigned"       // 任务指派
	ActivityActionTaskStatusChanged = "task_status_changed" // 任务状态变更
	ActivityActionCommentAdded      = "comment_added"       // 添加评论
	ActivityActionCommentDeleted    = "comment_deleted"     // 删除评论
)

// ActivityContent 任务动态内容

const (
	ActivityContentCreated         = "%s created a task"
	ActivityContentUpdatedBaseInfo = "%s updated task basic information"
	ActivityContentUpdatedStatus   = "%s changed the status of task '%s' from '%s' to '%s'"
	ActivityContentUpdatedAssignee = "%s assigned task '%s'"
	ActivityContentCommentCreated  = "%s commented on task '%s'"
	ActivityContentCommentDeleted  = "%s deleted a comment on task '%s'"
)

// RelatedType 关联资源类型

const (
	RelatedTypeTask    = "task"    // 任务
	RelatedTypeProject = "project" // 项目
	RelatedTypeComment = "comment" // 评论
	RelatedTypeSystem  = "system"  // 系统
)

// 关联信息

const (
	ActivityAssocOperator = "Operator" // 关联操作者
)

// TaskActivity 任务动态表
type TaskActivity struct {
	ID          uint64         `gorm:"column:id;primaryKey;autoIncrement;comment:任务动态ID" json:"id"`
	TaskID      uint64         `gorm:"column:task_id;not null;index:idx_task_activities_task_created,priority:1;comment:任务ID" json:"task_id"`
	ProjectID   uint64         `gorm:"column:project_id;not null;index:idx_task_activities_project_created,priority:1;comment:项目ID" json:"project_id"`
	OperatorID  uint64         `gorm:"column:operator_id;not null;index:idx_task_activities_operator_id;comment:操作人ID" json:"operator_id"`
	Action      string         `gorm:"column:action;type:varchar(50);not null;index:idx_task_activities_action;comment:动作类型" json:"action"`
	Content     string         `gorm:"column:content;type:varchar(500);not null;comment:动态内容" json:"content"`
	RelatedType *string        `gorm:"column:related_type;type:varchar(50);comment:关联资源类型，例如 task/comment/project" json:"related_type"`
	RelatedID   *uint64        `gorm:"column:related_id;comment:关联资源ID" json:"related_id"`
	ExtraJSON   datatypes.JSON `gorm:"column:extra_json;type:json;comment:扩展字段，记录变更前后等信息" json:"extra_json"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;autoCreateTime;index:idx_task_activities_task_created,priority:2;index:idx_task_activities_project_created,priority:2;comment:创建时间" json:"created_at"`

	Operator *User `gorm:"foreignKey:OperatorID;references:ID" json:"operator,omitempty"`
}

// TableName 指定表名
func (TaskActivity) TableName() string {
	return "task_activities"
}
