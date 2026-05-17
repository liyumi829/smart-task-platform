// internal/service/cachesvc/cacheobj/project.go
// Package cacheobj
// 定义项目缓存对象结构。

package cacheobj

import (
	"smart-task-platform/internal/model"
	"time"
)

// ProjectBriefInfo 项目简要信息缓存对象。
// 注意：只保存业务响应常用的项目公共字段，不保存无关字段。
type ProjectBriefInfo struct {
	ID        uint64     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	OwnerID   uint64     `json:"owner_id"`
}

// ToModel 将项目简要信息缓存对象转换为 model.Project。
// 注意：这里只填充缓存中存在的字段，其它字段保持零值。
func (p *ProjectBriefInfo) ToModel() *model.Project {
	if p == nil || p.ID == 0 {
		return nil
	}

	return &model.Project{
		ID:        p.ID,
		Name:      p.Name,
		Status:    p.Status,
		StartDate: p.StartDate,
		EndDate:   p.EndDate,
		OwnerID:   p.OwnerID,
	}
}
