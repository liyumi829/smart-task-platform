// internal/service/cachesvc/const.go
// Package cachesvc
// 常量定义
package cachesvc

// Redis Key 前缀常量

const (
	UserExistsKeyPrefix        = "user:exists"         // UserExistsKeyPrefix 用户是否存在缓存 Key 前缀。
	UserPublicProfileKeyPrefix = "user:public_profile" // UserPublicProfileKeyPrefix 用户公共信息缓存 Key 前缀。
	UserProjectIDsKeyPrefix    = "user:project_ids"    // UserProjectIDsKeyPrefix 用户拥有的项目IDs
)

const (
	ProjectExistsKeyPrefix    = "project:exists"     // ProjectExistsKeyPrefix 项目是否存在缓存 Key 前缀。
	ProjectBriefInfoKeyPrefix = "project:brief_info" // ProjectBriefInfoKeyPrefix 项目简要信息缓存 Key 前缀。
)

const (
	ProjectMemberRoleKeyPrefix = "project_member:role" // ProjectMemberRoleKeyPrefix 项目成员角色缓存 Key 前缀。
)

const (
	TaskPermissionInfoKeyPrefix = "task:permission_info" // TaskPermissionInfoKeyPrefix 任务权限判定信息缓存 Key 前缀。
	TaskDetailInfoKeyPrefix     = "task:detail_info"     // TaskDetailInfoKeyPrefix 任务详情信息缓存 key 前缀
	TaskListItemKeyPrefix       = "task:list_item"       // TaskListItemKeyPrefix 任务列表项缓存前缀
)

const (
	UserListKeyPrefix        = "list:user"
	UserProjectListKeyPrefix = "list:user:project"
	TaskListKeyPrefix        = "list:task"
	TaskCommentListKeyPrefix = "list:task_comment"
)

const (
	CacheNullValue = "__NULL__" // CacheNullValue 空值缓存标记，用于防止缓存穿透。
	CacheBoolTrue  = "1"        // CacheBoolTrue bool true 缓存值。
	CacheBoolFalse = "0"        // CacheBoolFalse bool false 缓存值。
)

// Key 的关键字

const (
	User        = "user"
	Project     = "project"
	Task        = "task"
	TaskComment = "task_comment"
)
