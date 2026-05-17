// internal/service/cachesvc/project_member_cache.go
// Package cachesvc
// 实现项目成员模块的缓存服务操作。
package cachesvc

import (
	"context"
	"strings"

	cacheStore "smart-task-platform/internal/cache"
	"smart-task-platform/internal/pkg/codec"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"

	"go.uber.org/zap"
)

// ProjectMemberCache 项目成员缓存
type ProjectMemberCache struct {
	store cacheStore.Store // 操作句柄 Set/Del/Get/MGet
	kb    *keyBuilder      // key建造者
}

// NewProjectMemberCache 创建项目成员缓存实例
func NewProjectMemberCache(store cacheStore.Store, kb *keyBuilder) *ProjectMemberCache {
	return &ProjectMemberCache{
		store: store,
		kb:    kb,
	}
}

// RoleKey 获取项目成员角色缓存 Key
func (c *ProjectMemberCache) RoleKey(projectID, userID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.ProjectMemberRoleKey(projectID, userID)
}

// GetRole 从缓存读取项目成员角色。
//   - 返回值：role 表示项目成员角色，exists 表示是否是项目成员，hit 表示缓存是否有效命中。
//   - 注意：缓存失败只记录日志，不返回错误。
func (c *ProjectMemberCache) GetRole(ctx context.Context, projectID, userID uint64) (role string, exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 || userID == 0 {
		return "", false, false
	}

	key := c.kb.ProjectMemberRoleKey(projectID, userID)

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get project member role cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return "", false, false
	}

	if !ok {
		return "", false, false
	}

	// 命中空值缓存，表示用户不是项目成员。
	if value == CacheNullValue {
		return "", false, true
	}

	var info cacheObj.ProjectMemberRoleInfo
	if err := codec.UnmarshalString(value, &info); err == nil {
		role = strings.TrimSpace(info.Role)
		if role != "" {
			return role, true, true
		}
	} else {
		zap.L().Warn("unmarshal project member role cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}

	// 缓存内容异常，删除脏缓存后继续回源 DB。
	if delErr := c.store.Del(ctx, key); delErr != nil {
		zap.L().Warn("delete invalid project member role cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(delErr),
		)
	}

	return "", false, false
}

// SetRoleNull 写入项目成员角色空值缓存。
//   - 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *ProjectMemberCache) SetRoleNull(ctx context.Context, projectID, userID uint64) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 || userID == 0 {
		return
	}

	key := c.kb.ProjectMemberRoleKey(projectID, userID)

	if err := c.store.Set(ctx, key, CacheNullValue, ProjectMemberRoleNullTTL); err != nil {
		zap.L().Warn("set project member role null cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// SetRole 写入项目成员角色缓存。
//   - 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *ProjectMemberCache) SetRole(ctx context.Context, projectID, userID uint64, role string) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 || userID == 0 {
		return
	}

	role = strings.TrimSpace(role)
	if role == "" {
		return
	}

	key := c.kb.ProjectMemberRoleKey(projectID, userID)

	info := &cacheObj.ProjectMemberRoleInfo{
		Role: role,
	}

	raw, err := codec.MarshalString(info)
	if err != nil {
		zap.L().Warn("marshal project member role cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return
	}

	if err := c.store.Set(ctx, key, raw, ProjectMemberRoleTTL); err != nil {
		zap.L().Warn("set project member role cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.String("role", role),
			zap.Error(err),
		)
	}
}

// DeleteRole 删除项目成员角色缓存。
//   - 注意：删除缓存失败会记录日志并返回错误。
func (c *ProjectMemberCache) DeleteRole(ctx context.Context, projectID, userID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 || userID == 0 {
		return nil
	}

	key := c.kb.ProjectMemberRoleKey(projectID, userID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete project member role cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// DeleteAll 删除项目成员模块相关缓存。
//   - 注意：当前项目成员模块底层只维护 role 缓存。
//   - 注意：删除缓存失败会记录日志并返回错误。
func (c *ProjectMemberCache) DeleteAll(ctx context.Context, projectID, userID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 || userID == 0 {
		return nil
	}

	roleKey := c.kb.ProjectMemberRoleKey(projectID, userID)
	keys := []string{roleKey}

	if err := c.store.Del(ctx, keys...); err != nil {
		zap.L().Warn("delete project member cache failed",
			zap.Uint64("project_id", projectID),
			zap.Uint64("user_id", userID),
			zap.Strings("cache_keys", keys),
			zap.Int("key_number", len(keys)),
			zap.Error(err),
		)
		return err
	}

	return nil
}
