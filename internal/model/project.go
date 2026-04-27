// internal/model/project.go
// Package model projects 表的 gorm 关系映射
package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ProjectStatusActive   = "active"   // 激活中
	ProjectStatusArchived = "archived" // 已归档
)

const (
	ProjectTableName         = "projects"    // 表名
	ProjectColumnID          = "id"          // 主键ID
	ProjectColumnName        = "name"        // 项目名称
	ProjectColumnDescription = "description" // 项目描述
	ProjectColumnOwnerID     = "owner_id"    // 拥有者ID
	ProjectColumnStatus      = "status"      // 状态
	ProjectColumnStartDate   = "start_date"  // 开始日期
	ProjectColumnEndDate     = "end_date"    // 结束日期
	ProjectColumnCreatedAt   = "created_at"  // 创建时间
	ProjectColumnUpdatedAt   = "updated_at"  // 更新时间
	ProjectColumnDeletedAt   = "deleted_at"  // 软删除

	// ======================
	// 关联关系 字段常量（预加载/查询使用）
	// ======================
	ProjectAssocOwner   = "Owner"   // 关联拥有者
	ProjectAssocMembers = "Members" // 关联项目成员
)

// Project 项目表映射
type Project struct {
	ID          uint64         `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement;comment:项目ID" json:"id"`
	Name        string         `gorm:"column:name;type:varchar(100);not null;comment:项目名称" json:"name"`
	Description string         `gorm:"column:description;type:text;comment:项目描述" json:"description"`
	OwnerID     uint64         `gorm:"column:owner_id;type:bigint unsigned;not null;index:idx_projects_owner_id;comment:创建者/拥有者ID" json:"owner_id"`
	Status      string         `gorm:"column:status;type:varchar(20);not null;default:'active';index:idx_projects_status;comment:项目状态" json:"status"`
	StartDate   *time.Time     `gorm:"column:start_date;type:date;comment:项目开始日期" json:"start_date"`
	EndDate     *time.Time     `gorm:"column:end_date;type:date;comment:项目结束日期" json:"end_date"`
	CreatedAt   time.Time      `gorm:"column:created_at;type:datetime;not null;index:idx_projects_created_at;comment:创建时间" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;type:datetime;not null;comment:更新时间" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;type:datetime;index;comment:软删除时间" json:"deleted_at"`

	// 关联关系
	Owner   User            `gorm:"foreignKey:OwnerID;references:ID" json:"owner"`     // 项目拥有者
	Members []ProjectMember `gorm:"foreignKey:ProjectID;references:ID" json:"members"` // 项目成员列表
}

// TableName 指定表名
func (Project) TableName() string {
	return "projects"
}
