// internal/service/cachesvc/cacheobj/user_cache_object.go
// Package cacheobj
// 用户的缓存存储对象
package cacheobj

import (
	"fmt"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/repository"
)

// UserBriefInfo 用户公共信息缓存对象
type UserBriefInfo struct {
	ID       uint64 `json:"id"`                           // 用户 ID
	Username string `json:"username"`                     // 用户名
	Nickname string `json:"nickname" binding:"omitempty"` // 昵称
	Avatar   string `json:"avatar" binding:"omitempty"`   // 头像
}

// ToModel 将用户简要信息缓存对象转换为 model.User。
// 注意：这里只填充缓存中存在的字段，其它字段保持 model.User 零值。
func (u *UserBriefInfo) ToModel() *model.User {
	if u == nil || u.ID == 0 {
		return nil
	}

	return &model.User{
		ID:       u.ID,
		Username: u.Username,
		Nickname: u.Nickname,
		Avatar:   u.Avatar,
	}
}

type UserListPage = ListPage

type UserListQuery struct {
	Page      int
	PageSize  int
	Keyword   string
	NeedTotal bool
}

func (q UserListQuery) ToRepository() *repository.UserSearchQuery {
	return &repository.UserSearchQuery{
		SearchQuery: repository.SearchQuery{
			Page:      q.Page,
			PageSize:  q.PageSize,
			NeedTotal: q.NeedTotal,
		},
		Keyword: q.Keyword,
	}
}

func (q *UserListQuery) CacheKeyParts() []string {
	if q == nil {
		return nil
	}

	return []string{
		fmt.Sprintf("page=%d", q.Page),
		fmt.Sprintf("size=%d", q.PageSize),
		fmt.Sprintf("need_total=%t", q.NeedTotal),
		"keyword=" + hashKeyword(q.Keyword),
	}
}

// UserProjectIDs 用户参与的项目ID集合缓存对象
type UserProjectIDs struct {
	ProjectIDs []uint64 `json:"project_ids"`
}

type UserProjectListPage = ListPage

type UserProjectListQuery struct {
	UserID     uint64 // 带个id，分配构造key的时候带上用户信息
	Page       int
	PageSize   int
	NeedTotal  bool
	Status     string
	Keyword    string
	ProjectIDs []uint64
}

func (q UserProjectListQuery) ToRepository() *repository.ProjectSearchQuery {
	return &repository.ProjectSearchQuery{
		SearchQuery: repository.SearchQuery{
			Page:      q.Page,
			PageSize:  q.PageSize,
			NeedTotal: q.NeedTotal,
		},
		Keyword:    q.Keyword,
		Status:     q.Status,
		ProjectIDs: q.ProjectIDs,
	}
}

func (q *UserProjectListQuery) CacheKeyParts() []string {
	if q == nil {
		return nil
	}

	return []string{
		fmt.Sprintf("user_id=%d", q.UserID),
		fmt.Sprintf("page=%d", q.Page),
		fmt.Sprintf("size=%d", q.PageSize),
		fmt.Sprintf("need_total=%t", q.NeedTotal),
		"keyword=" + hashKeyword(q.Keyword),
		"status=" + normalizeKeyPart(q.Status),
		"projects=" + utils.SliceToUniqueCacheKey(q.ProjectIDs),
	}
}
