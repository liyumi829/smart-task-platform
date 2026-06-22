// internal/dto/task_activities.go
// Package dto
// 任务动态接口的任务传输对象

package dto

import "time"

type TaskActivityIDUri struct {
	ProjectID uint64 `uri:"projectId"`
	TaskID    uint64 `uri:"taskId"`
}

// ListTaskActivitiesQuery 获取任务动态列表参数
type ListTaskActivitiesQuery struct {
	PageQuery
}

// TaskActivityListItem 任务动态表项
type TaskActivityListItem struct {
	ID         uint64             `json:"id"`
	TaskID     uint64             `json:"task_id"`
	Type       string             `json:"type"`
	Content    string             `json:"content"`
	OperatorID uint64             `json:"operator_id"`
	Operator   *UserPublicProfile `json:"operator"`
	CreatedAt  time.Time          `json:"created_at"`
}

type ListTaskActivitiesResp = PageResp[*TaskActivityListItem]
