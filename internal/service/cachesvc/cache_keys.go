// internal/service/cachesvc/cache_keys.go
// 构造缓存存储的 key 值
package cachesvc

import (
	"fmt"
	"strings"
)

// VersionController 版本控制器
type VersionController interface {
	Get(kind ListKind) uint64
	Bump(kind ListKind) uint64
}

// keyBuilder 构建 Key
type keyBuilder struct {
	reg VersionController
}

// NewkeyBuilder 创建 key 构造器实例
func NewkeyBuilder(reg VersionController) *keyBuilder {
	if reg == nil {
		reg = NewListVersionRegistry()
	}

	return &keyBuilder{
		reg: reg,
	}
}

// UserExistsKey 用户是否存在缓存 Key。
func (b keyBuilder) UserExistsKey(userID uint64) string {
	return fmt.Sprintf("%s:%d", UserExistsKeyPrefix, userID)
}

// UserBriefInfoKey 用户公共信息缓存 Key。
func (b keyBuilder) UserBriefInfoKey(userID uint64) string {
	return fmt.Sprintf("%s:%d", UserPublicProfileKeyPrefix, userID)
}

// ProjectExistsKey 项目是否存在缓存 Key。
func (b keyBuilder) ProjectExistsKey(projectID uint64) string {
	return fmt.Sprintf("%s:%d", ProjectExistsKeyPrefix, projectID)
}

// ProjectBriefInfoKey 项目简要信息缓存 Key。
func (b keyBuilder) ProjectBriefInfoKey(projectID uint64) string {
	return fmt.Sprintf("%s:%d", ProjectBriefInfoKeyPrefix, projectID)
}

// ProjectMemberRoleKey 项目成员角色缓存 Key。
func (b keyBuilder) ProjectMemberRoleKey(projectID, userID uint64) string {
	return fmt.Sprintf("%s:%s:%d:%s:%d", ProjectMemberRoleKeyPrefix, Project, projectID, User, userID)
}

// TaskPermissionInfoKey 生成任务权限判定信息缓存 Key。
func (b keyBuilder) TaskPermissionInfoKey(taskID uint64) string {
	return fmt.Sprintf("%s:%d", TaskPermissionInfoKeyPrefix, taskID)
}

// TaskDetailInfoKey 生成任务权限判定信息缓存 Key。
func (b keyBuilder) TaskDetailInfoKey(taskID uint64) string {
	return fmt.Sprintf("%s:%d", TaskDetailInfoKeyPrefix, taskID)
}

// TaskListItemKey 生成任务列表项缓存 Key。
func (b keyBuilder) TaskListItemKey(taskID uint64) string {
	return fmt.Sprintf("%s:%d", TaskListItemKeyPrefix, taskID)
}

//=======
// 带有版本号的列表构造
//=======

// TaskListKey 构造 task 列表缓存 key。
func (b *keyBuilder) TaskListKey(query CacheKeyPart) string {
	return b.buildKey(ListKindTask, TaskListKeyPrefix, query)
}

// BumpTaskVersion 提升 task 列表版本。
func (b *keyBuilder) BumpTaskVersion() uint64 {
	return b.reg.Bump(ListKindTask)
}

// UserListKey 构造 user 列表缓存 key。
func (b *keyBuilder) UserListKey(query CacheKeyPart) string {
	return b.buildKey(ListKindUser, UserListKeyPrefix, query)
}

// BumpUserVersion 提升 user 列表版本。
func (b *keyBuilder) BumpUserVersion() uint64 {
	return b.reg.Bump(ListKindUser)
}

// UserProjectLists 构造 user 参与的的 project 列表缓存 key。
func (b *keyBuilder) UserProjectIDs(userID uint64) string {
	return fmt.Sprintf("%s:v%d:user_id:%d", UserProjectIDsKeyPrefix, b.CurrentVersion(ListKindUserProject), userID)
}

// UserProjectLists 构造 user 查找 project 缓存列表
func (b *keyBuilder) UserProjectListKey(query CacheKeyPart) string {
	return b.buildKey(ListKindUserProject, UserProjectListKeyPrefix, query)
}

// BumpUserProjectVersion 提升用户下的 project 列表版本。
func (b *keyBuilder) BumpUserProjectListVersion() uint64 {
	return b.reg.Bump(ListKindUserProject)
}

// TaskCommentListKey 构造 taskcomment 列表缓存 key。
func (b *keyBuilder) TaskCommentListKey(query CacheKeyPart) string {
	return b.buildKey(ListKindTaskComment, TaskCommentListKeyPrefix, query)
}

// BumpTaskCommentVersion 提升 taskcomment 列表版本。
func (b *keyBuilder) BumpTaskCommentVersion() uint64 {
	return b.reg.Bump(ListKindTaskComment)
}

// CurrentVersion 获取当前列表版本。
func (b *keyBuilder) CurrentVersion(kind ListKind) uint64 {
	if b == nil || b.reg == nil {
		return 1
	}

	return b.reg.Get(kind)
}

// buildKey 构造单个版本 key。
func (b *keyBuilder) buildKey(kind ListKind, prefix string, query CacheKeyPart) string {
	version := b.CurrentVersion(kind)
	return b.composeKey(prefix, version, query)
}

// composeKey 统一拼接 key。
// 格式示例：
// list:task:v2:page=1:size=20:project=10:keyword=xxxx
func (b *keyBuilder) composeKey(prefix string, version uint64, query CacheKeyPart) string {
	parts := []string{
		prefix,
		fmt.Sprintf("v%d", version),
	}

	if query != nil {
		for _, part := range query.CacheKeyParts() {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, ":")
}
