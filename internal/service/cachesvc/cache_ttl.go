// internal/service/cachesvc/cache_ttl.go
// Package cacheobj
// 定义服务的缓存对象的 ttl 常量值
package cachesvc

import "time"

const (
	// CacheDeleteDelay 延迟双删等待时间
	CacheDeleteDelay = 500 * time.Millisecond
)

const (
	// UserExistsTTL 用户存在缓存 TTL
	UserExistsTTL = 20 * time.Minute

	// UserExistsNullTTL 用户不存在缓存 TTL
	UserExistsNullTTL = 2 * time.Minute

	// UserBriefInfoTTL 用户公共信息缓存 TTL
	UserBriefInfoTTL = 20 * time.Minute

	// UserBriefInfoNullTTL 用户公共信息不存在缓存 TTL
	UserBriefInfoNullTTL = 2 * time.Minute

	// UserListTTL 用户列表缓存 TTL
	UserListTTL = 60 * time.Second

	// UserProjectsIDsTTL 某个成员的项目ID缓存 TTL
	UserProjectIDsTTL = 5 * time.Minute

	// UesrProjectIDsNullTTL 某个成员的项目ID不存在缓存 TTL
	UserProjectIDsNullTTL = 1 * time.Minute

	// UserProjectListTTL 用户项目列表TTL
	UserProjectListTTL = 60 * time.Second
)

const (
	// ProjectExistsTTL 项目存在缓存 TTL
	ProjectExistsTTL = 20 * time.Minute

	// ProjectExistsNullTTL 项目不存在缓存 TTL
	ProjectExistsNullTTL = 2 * time.Minute

	// ProjectBriefInfoTTL 项目简要信息缓存 TTL
	ProjectBriefInfoTTL = 20 * time.Minute

	// ProjectBriefInfoNullTTL 项目简要信息不存在缓存 TTL
	ProjectBriefInfoNullTTL = 2 * time.Minute
)

const (
	// ProjectMemberRoleTTL 项目成员角色缓存 TTL
	ProjectMemberRoleTTL = 20 * time.Minute

	// ProjectMemberRoleNullTTL 项目成员不存在缓存 TTL
	ProjectMemberRoleNullTTL = 2 * time.Minute
)

const (
	// TaskPermissionInfoTTL 任务权限判定信息缓存 TTL
	TaskPermissionInfoTTL = 20 * time.Minute

	// TaskPermissionInfoNullTTL 任务权限不存在空值缓存 TTL
	TaskPermissionInfoNullTTL = 2 * time.Minute

	// TaskDetailInfoTTL 任务详情判定信息缓存 TTL
	TaskDetailInfoTTL = 20 * time.Minute

	// TaskDetailInfoNullTTL 任务详情不存在空值缓存 TTL
	TaskDetailInfoNullTTL = 2 * time.Minute

	// TaskListItemTTL 任务列表项缓存 TTL
	TaskListItemTTL = 15 * time.Minute

	// TaskListItemNullTTL 任务列表项空值缓存 TTL
	TaskListItemNullTTL = 2 * time.Minute

	// TaskListTTL 任务列表缓存
	TaskListTTL = 60 * time.Second
)
