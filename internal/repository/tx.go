// Package repository 封装数据库操作，提供事务管理功能。

package repository

import (
	"context"

	"gorm.io/gorm"
)

// TxManager 事务管理器
type TxManager struct {
	db *gorm.DB
}

// NewTxManager 创建事务管理器
func NewTxManager(db *gorm.DB) *TxManager {
	return &TxManager{db: db}
}

// Transaction 开启事务
// 执行 fn 函数，传入事务对象 tx，如果 fn 返回错误，则事务回滚；如果 fn 返回 nil，则事务提交。
// fn 是一个函数参数，接受一个 *gorm.DB 类型的事务对象，并返回一个 error 类型的结果。
func (m *TxManager) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return m.db.WithContext(ctx).
		Transaction(
			func(tx *gorm.DB) error {
				return fn(tx)
			})
}
