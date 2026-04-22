// Package model 定义了用户相关的数据模型和数据库交互逻辑。
package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	UserStatusActive   = "active"   // 用户状态：活跃
	UserStatusDisabled = "disabled" // 用户状态：禁用
)

// User 用户模型
type User struct {
	//主键
	ID uint64 `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement;comment:用户ID" json:"id"`
	// 用户名称
	Username string `gorm:"column:username;type:varchar(50);not null;uniqueIndex:uk_users_username;comment:用户名" json:"username"`
	// 用户邮箱
	Email string `gorm:"column:email;type:varchar(100);not null;uniqueIndex:uk_users_email;comment:邮箱" json:"email"`
	// 加密后的密码
	PasswordHash string `gorm:"column:password_hash;type:varchar(255);not null;comment:加密后的密码" json:"-"`
	// 用户昵称
	Nickname string `gorm:"column:nickname;type:varchar(50);default:null;comment:昵称" json:"nickname"`
	// 用户头像 URL
	Avatar string `gorm:"column:avatar;type:varchar(255);default:null;comment:头像URL" json:"avatar"`
	// 用户状态
	Status string `gorm:"column:status;type:varchar(20);not null;default:active;index:idx_users_status;comment:用户状态" json:"status"`
	// 最后登录时间
	LastLoginAt *time.Time     `gorm:"column:last_login_at;type:datetime;default:null;comment:最后登录时间" json:"last_login_at"`
	CreatedAt   time.Time      `gorm:"column:created_at;type:datetime;not null;autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;type:datetime;not null;autoUpdateTime;comment:更新时间" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index:idx_users_deleted_at;comment:软删除时间" json:"-"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
