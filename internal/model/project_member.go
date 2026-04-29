// internal/model/project_member.go
// Package model 完成项目成员表的映射
package model

import (
	"time"
)

// 0 最高级
var RoleLevel map[string]int = map[string]int{
	ProjectMemberRoleOwner:  0,
	ProjectMemberRoleAdmin:  1,
	ProjectMemberRoleMember: 2,
}

const (
	ProjectMemberRoleOwner  = "owner"  // 拥有者
	ProjectMemberRoleAdmin  = "admin"  // 管理员
	ProjectMemberRoleMember = "member" // 普通成员
)

const (
	ProjectMemberTableName       = "project_members" // 表名
	ProjectMemberColumnID        = "id"              // 主键ID
	ProjectMemberColumnProjectID = "project_id"      // 项目ID
	ProjectMemberColumnUserID    = "user_id"         // 用户ID
	ProjectMemberColumnRole      = "role"            // 项目角色
	ProjectMemberColumnInvitedBy = "invited_by"      // 邀请人ID
	ProjectMemberColumnJoinedAt  = "joined_at"       // 加入时间
	ProjectMemberColumnCreatedAt = "created_at"      // 创建时间
	ProjectMemberColumnUpdatedAt = "updated_at"      // 更新时间

	// ==============================
	// 关联关系常量（必须 = 结构体字段名，大写）
	// ==============================
	ProjectMemberAssocProject = "Project" // 关联项目
	ProjectMemberAssocUser    = "User"    // 关联成员用户
	ProjectMemberAssocInviter = "Inviter" // 关联邀请人
)

// ProjectMember 项目成员表映射
type ProjectMember struct {
	ID        uint64    `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement;comment:记录ID" json:"id"`
	ProjectID uint64    `gorm:"column:project_id;type:bigint unsigned;not null;uniqueIndex:uk_project_user;comment:项目ID" json:"project_id"`
	UserID    uint64    `gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_project_user;index:idx_project_members_user_id;comment:用户ID" json:"user_id"`
	Role      string    `gorm:"column:role;type:varchar(20);not null;default:'member';index:idx_project_members_role;comment:项目角色" json:"role"`
	InvitedBy *uint64   `gorm:"column:invited_by;type:bigint unsigned;comment:邀请人ID" json:"invited_by"`
	JoinedAt  time.Time `gorm:"column:joined_at;type:datetime;not null;comment:加入时间" json:"joined_at"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime;not null;comment:创建时间" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime;not null;comment:更新时间" json:"updated_at"`

	// 关联关系
	Project *Project `gorm:"foreignKey:ProjectID;references:ID" json:"project"` // 所属项目
	User    *User    `gorm:"foreignKey:UserID;references:ID" json:"user"`       // 项目成员
	Inviter *User    `gorm:"foreignKey:InvitedBy;references:ID" json:"inviter"` // 邀请人
}

// TableName 指定表名
func (ProjectMember) TableName() string {
	return "project_members"
}
