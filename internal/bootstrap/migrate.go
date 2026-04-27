// Package bootstrap
// 完成项目启动对数据库的表结构定义操作

package bootstrap

import (
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"smart-task-platform/internal/model"
)

// AutoMigrate 自动迁移所有需要的表
// 注意：迁移顺序应根据表之间的依赖关系进行调整，以避免外键约束问题00
// 例如，如果 Task 表依赖于 User 表，那么应该先迁移 User 表，再迁移 Task 表。
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	// 按依赖顺序迁移
	if err := db.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.ProjectMember{},
	); err != nil {
		zap.L().Error("AutoMigrate failed", zap.Error(err))
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	return nil
}
