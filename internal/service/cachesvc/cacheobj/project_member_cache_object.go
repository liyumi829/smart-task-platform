// internal/service/cachesvc/cacheobj/project_member_cache_object.go
// Package cacheobj 定义项目成员缓存对象。
package cacheobj

// ProjectMemberRoleInfo 项目成员角色缓存对象。
//   - 注意：这里只缓存 role，不缓存其它成员冗余字段。
type ProjectMemberRoleInfo struct {
	Role string `json:"role"`
}
