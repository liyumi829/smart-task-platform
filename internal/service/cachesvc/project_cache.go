// internal/service/cachesvc/project_cache.go
// Package cachesvc
// 实现项目模块的缓存服务操作
package cachesvc

import (
	"context"

	cacheStore "smart-task-platform/internal/cache"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/codec"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/service/cachesvc/cacheobj"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"

	"go.uber.org/zap"
)

// ProjectCache 项目缓存
type ProjectCache struct {
	store cacheStore.Store // 操作句柄 Set/Del/Get/MGet
	kb    *keyBuilder      // key建造者
	conv  *Converter       // 类型转化器
}

// NewProjectCache 创建项目缓存实例
func NewProjectCache(store cacheStore.Store, kb *keyBuilder, conv *Converter) *ProjectCache {
	return &ProjectCache{
		store: store,
		kb:    kb,
		conv:  conv,
	}
}

func (c *ProjectCache) ExistsKey(projectID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.ProjectExistsKey(projectID)
}

func (c *ProjectCache) BriefInfoKey(projectID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.ProjectBriefInfoKey(projectID)
}

// GetExists 从缓存读取项目是否存在。
//   - 返回值：exists 表示项目是否存在，hit 表示缓存是否有效命中。
//   - 注意：缓存失败只记录日志，不返回错误。
func (c *ProjectCache) GetExists(ctx context.Context, projectID uint64) (exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return false, false
	}

	key := c.kb.ProjectExistsKey(projectID)

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get project exists cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return false, false
	}

	if !ok {
		return false, false
	}

	switch value {
	case CacheBoolTrue:
		return true, true

	case CacheBoolFalse:
		return false, true

	default:
		// 缓存值异常，删除脏缓存后继续回源 DB。
		if delErr := c.store.Del(ctx, key); delErr != nil {
			zap.L().Warn("delete invalid project exists cache failed",
				zap.Uint64("project_id", projectID),
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}

		return false, false
	}
}

// SetExists 写入项目是否存在缓存。
//   - 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *ProjectCache) SetExists(ctx context.Context, projectID uint64, exists bool) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return
	}

	key := c.kb.ProjectExistsKey(projectID)

	cacheValue := CacheBoolFalse
	cacheTTL := ProjectExistsNullTTL

	if exists {
		cacheValue = CacheBoolTrue
		cacheTTL = ProjectExistsTTL
	}

	if err := c.store.Set(ctx, key, cacheValue, cacheTTL); err != nil {
		zap.L().Warn("set project exists cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Bool("exists", exists),
			zap.Error(err),
		)
	}
}

// DeleteExists 删除项目存在性缓存。
//   - 注意：删除缓存失败会记录日志并返回错误。
func (c *ProjectCache) DeleteExists(ctx context.Context, projectID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return nil
	}

	key := c.kb.ProjectExistsKey(projectID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete project exists cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// GetBriefInfo 从缓存读取项目简要信息。
//   - 返回值：info 表示项目简要信息，exists 表示项目是否存在，hit 表示缓存是否有效命中。
//   - 注意：缓存失败只记录日志，不返回错误。
func (c *ProjectCache) GetBriefInfo(ctx context.Context, projectID uint64) (info *cacheobj.ProjectBriefInfo, exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return nil, false, false
	}

	key := c.kb.ProjectBriefInfoKey(projectID)

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get project brief info cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false, false
	}

	if !ok {
		return nil, false, false
	}

	// 命中空值缓存，表示项目不存在。
	if value == CacheNullValue {
		return nil, false, true
	}

	var briefInfo cacheobj.ProjectBriefInfo
	if err := codec.UnmarshalString(value, &briefInfo); err == nil && briefInfo.ID != 0 {
		return &briefInfo, true, true
	}

	// 缓存内容异常，删除脏缓存后继续回源 DB。
	if delErr := c.store.Del(ctx, key); delErr != nil {
		zap.L().Warn("delete invalid project brief info cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Error(delErr),
		)
	}

	return nil, false, false
}

// BatchGetBriefInfo 批量从缓存获取项目简要信息。
// 返回值：
//   - briefInfoMap：命中的项目简要信息，key 为 projectID
//   - missingProjectIDs：缓存未命中的项目 ID
func (s *ProjectCache) BatchGetBriefInfo(ctx context.Context, projectIDs []uint64) (map[uint64]*cacheObj.ProjectBriefInfo, []uint64, error) {
	briefInfoMap := make(map[uint64]*cacheObj.ProjectBriefInfo, len(projectIDs))
	missingProjectIDs := make([]uint64, 0, len(projectIDs))

	if len(projectIDs) == 0 {
		return briefInfoMap, missingProjectIDs, nil
	}

	// 去重并过滤非法 ID，避免查询 project:brief:0 这类无效缓存。
	uniqueIDs := make([]uint64, 0, len(projectIDs))
	for _, projectID := range utils.Deduplicate(projectIDs) {
		if projectID == 0 {
			continue
		}
		uniqueIDs = append(uniqueIDs, projectID)
	}

	if len(uniqueIDs) == 0 {
		return briefInfoMap, missingProjectIDs, nil
	}

	keys := make([]string, 0, len(uniqueIDs))
	idToKey := make(map[uint64]string, len(uniqueIDs))

	for _, projectID := range uniqueIDs {
		key := s.kb.ProjectBriefInfoKey(projectID)
		keys = append(keys, key)
		idToKey[projectID] = key
	}

	cacheMap, err := s.store.MGet(ctx, keys...)
	if err != nil {
		// MGet 异常时，直接当作全部 miss 处理，避免缓存故障影响主流程。
		zap.L().Warn("mget project brief cache failed",
			zap.Int("keys_len", len(keys)),
			zap.Error(err),
		)
		return briefInfoMap, uniqueIDs, nil
	}

	for _, projectID := range uniqueIDs {
		key := idToKey[projectID]

		value, ok := cacheMap[key]
		if !ok || value == "" {
			missingProjectIDs = append(missingProjectIDs, projectID)
			continue
		}

		var projectCache cacheObj.ProjectBriefInfo
		if err := codec.UnmarshalString(value, &projectCache); err != nil {
			// 脏缓存：反序列化失败，删除后按 miss 处理。
			zap.L().Warn("unmarshal project brief cache failed",
				zap.Uint64("project_id", projectID),
				zap.String("cache_key", key),
				zap.Error(err),
			)

			if delErr := s.store.Del(ctx, key); delErr != nil {
				zap.L().Warn("delete dirty project brief cache failed",
					zap.Uint64("project_id", projectID),
					zap.String("cache_key", key),
					zap.Error(delErr),
				)
			}

			missingProjectIDs = append(missingProjectIDs, projectID)
			continue
		}

		if projectCache.ID != projectID {
			// 脏缓存：key 中的 projectID 和 value 中的 projectID 不一致。
			zap.L().Warn("project brief cache id mismatch",
				zap.Uint64("request_project_id", projectID),
				zap.Uint64("cache_project_id", projectCache.ID),
				zap.String("cache_key", key),
			)

			if delErr := s.store.Del(ctx, key); delErr != nil {
				zap.L().Warn("delete dirty project brief cache failed",
					zap.Uint64("request_project_id", projectID),
					zap.Uint64("cache_project_id", projectCache.ID),
					zap.String("cache_key", key),
					zap.Error(delErr),
				)
			}

			missingProjectIDs = append(missingProjectIDs, projectID)
			continue
		}

		briefInfoMap[projectID] = utils.SafeGetPtr(projectCache)
	}

	return briefInfoMap, missingProjectIDs, nil
}

// SetBriefInfoNull 写入项目简要信息空值缓存。
//   - 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *ProjectCache) SetBriefInfoNull(ctx context.Context, projectID uint64) {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return
	}

	key := c.kb.ProjectBriefInfoKey(projectID)

	if err := c.store.Set(ctx, key, CacheNullValue, ProjectBriefInfoNullTTL); err != nil {
		zap.L().Warn("set project brief info null cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// SetBriefInfo 写入项目简要信息缓存。
//   - 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *ProjectCache) SetBriefInfo(ctx context.Context, project *model.Project) {
	if c == nil || c.store == nil || c.kb == nil || c.conv == nil || project == nil || project.ID == 0 {
		return
	}

	key := c.kb.ProjectBriefInfoKey(project.ID)

	info := c.conv.ProjectBriefInfo(project)
	if info == nil || info.ID == 0 {
		zap.L().Warn("convert project brief info cache failed",
			zap.Uint64("project_id", project.ID),
			zap.String("cache_key", key),
		)
		return
	}

	raw, err := codec.MarshalString(info)
	if err != nil {
		zap.L().Warn("marshal project brief info cache failed",
			zap.Uint64("project_id", project.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return
	}

	if err := c.store.Set(ctx, key, raw, ProjectBriefInfoTTL); err != nil {
		zap.L().Warn("set project brief info cache failed",
			zap.Uint64("project_id", project.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// MSetBriefInfo 批量设置 Project 简要信息缓存。
//   - 注意：缓存写入失败只记录日志，不影响业务逻辑。
//   - 注意：所有缓存使用相同 TTL。
func (c *ProjectCache) MSetBriefInfo(ctx context.Context, projects []*model.Project) {
	if c == nil || c.store == nil || c.kb == nil || c.conv == nil || len(projects) == 0 {
		return
	}

	data := make(map[string]string, len(projects))

	for _, project := range projects {
		if project == nil || project.ID == 0 {
			continue
		}

		key := c.kb.ProjectBriefInfoKey(project.ID)

		info := c.conv.ProjectBriefInfo(project)
		if info == nil {
			zap.L().Warn("convert project brief info cache failed",
				zap.Uint64("project_id", project.ID),
				zap.String("cache_key", key),
			)
			continue
		}

		raw, err := codec.MarshalString(info)
		if err != nil {
			zap.L().Warn("marshal project brief info cache failed",
				zap.Uint64("project_id", project.ID),
				zap.String("cache_key", key),
				zap.Error(err),
			)
			continue
		}

		data[key] = raw
	}

	if len(data) == 0 {
		return
	}

	if err := c.store.MSet(ctx, data, ProjectBriefInfoTTL); err != nil {
		zap.L().Warn("multi set project brief info cache failed",
			zap.Int("project_number", len(projects)),
			zap.Int("key_number", len(data)),
			zap.Duration("ttl", ProjectBriefInfoTTL),
			zap.Error(err),
		)
	}
}

// DeleteBriefInfo 删除项目简要信息缓存。
//   - 注意：删除缓存失败会记录日志并返回错误。
func (c *ProjectCache) DeleteBriefInfo(ctx context.Context, projectID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return nil
	}

	key := c.kb.ProjectBriefInfoKey(projectID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete project brief info cache failed",
			zap.Uint64("project_id", projectID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// DeleteAll 删除项目相关所有缓存。
//   - 注意：删除缓存失败会记录日志并返回错误。
func (c *ProjectCache) DeleteAll(ctx context.Context, projectID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || projectID == 0 {
		return nil
	}

	existsKey := c.kb.ProjectExistsKey(projectID)
	briefInfoKey := c.kb.ProjectBriefInfoKey(projectID)

	if err := c.store.Del(ctx, existsKey, briefInfoKey); err != nil {
		zap.L().Warn("delete project all cache failed",
			zap.Uint64("project_id", projectID),
			zap.Strings("cache_keys", []string{existsKey, briefInfoKey}),
			zap.Error(err),
		)
		return err
	}

	return nil
}
