// internal/service/cachesvc/task_cache.go
// Package cachesvc
// 实现任务模块的缓存服务操作。
package cachesvc

import (
	"context"
	"strings"

	cacheStore "smart-task-platform/internal/cache"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/codec"
	"smart-task-platform/internal/pkg/utils"
	"smart-task-platform/internal/service/cachesvc/cacheobj"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"

	"go.uber.org/zap"
)

// TaskCache 任务缓存
type TaskCache struct {
	store cacheStore.Store // 操作句柄 Set/Del/Get/MGet
	kb    *keyBuilder      // key建造者
	conv  *Converter
}

// NewTaskCache 创建任务缓存实例
func NewTaskCache(store cacheStore.Store, kb *keyBuilder, conv *Converter) *TaskCache {
	return &TaskCache{
		store: store,
		kb:    kb,
		conv:  conv,
	}
}

func (c *TaskCache) PermissionInfoKey(taskID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.TaskPermissionInfoKey(taskID)
}

func (c *TaskCache) DetailInfoKey(taskID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}
	return c.kb.TaskDetailInfoKey(taskID)
}

func (c *TaskCache) ListItemKey(taskID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.TaskListItemKey(taskID)
}

func (c *TaskCache) ListKey(query *cacheObj.TaskListQuery) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.TaskListKey(query)
}

// GetPermissionInfo 从缓存读取任务权限信息。
// 返回值：info 表示任务权限信息，exists 表示任务是否存在，hit 表示缓存是否有效命中。
// 注意：缓存失败只记录日志，不返回错误。
func (c *TaskCache) GetPermissionInfo(ctx context.Context, taskID uint64) (info *cacheobj.TaskPermissionInfo, exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil, false, false
	}

	key := c.kb.TaskPermissionInfoKey(taskID)

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get task permission info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false, false
	}
	if !ok {
		return nil, false, false
	}

	// 命中空值缓存，表示任务不存在。
	if value == CacheNullValue {
		return nil, false, true
	}

	valid := func(info *cacheobj.TaskPermissionInfo) bool {
		if info == nil {
			return false
		}

		return info.ID != 0 &&
			info.ProjectID != 0 &&
			info.CreatorID != 0 &&
			strings.TrimSpace(info.Status) != ""
	}

	var infoCache cacheobj.TaskPermissionInfo
	if err = codec.UnmarshalString(value, &infoCache); err == nil && valid(&infoCache) {
		return &infoCache, true, true
	}

	if err != nil {
		zap.L().Warn("unmarshal task permission info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}

	// 缓存内容异常，删除脏缓存后继续回源 DB。
	if delErr := c.store.Del(ctx, key); delErr != nil {
		zap.L().Warn("delete invalid task permission info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(delErr),
		)
	}

	return nil, false, false
}

// SetPermissionInfoNull 写入任务权限信息空值缓存。
// 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *TaskCache) SetPermissionInfoNull(ctx context.Context, taskID uint64) {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return
	}

	key := c.kb.TaskPermissionInfoKey(taskID)

	if err := c.store.Set(ctx, key, CacheNullValue, TaskPermissionInfoNullTTL); err != nil {
		zap.L().Warn("set task permission info null cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// SetPermissionInfo 写入任务权限信息缓存。
// 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *TaskCache) SetPermissionInfo(ctx context.Context, task *model.Task) {
	if c == nil || c.store == nil || c.kb == nil || c.conv == nil || task == nil || task.ID == 0 {
		return
	}

	key := c.kb.TaskPermissionInfoKey(task.ID)

	info := c.conv.TaskPermissionInfo(task)
	if info == nil {
		zap.L().Warn("convert task permission info cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
		)
		return
	}

	raw, err := codec.MarshalString(info)
	if err != nil {
		zap.L().Warn("marshal task permission info cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return
	}

	if err := c.store.Set(ctx, key, raw, TaskPermissionInfoTTL); err != nil {
		zap.L().Warn("set task permission info cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// DeletePermissionInfo 删除任务权限信息缓存。
// 注意：删除缓存失败会记录日志并返回错误。
func (c *TaskCache) DeletePermissionInfo(ctx context.Context, taskID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil
	}

	key := c.kb.TaskPermissionInfoKey(taskID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete task permission info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// GetDetailInfo 从缓存读取任务详情信息。
// 返回值：info 表示任务详情信息，exists 表示任务是否存在，hit 表示缓存是否有效命中。
// 注意：缓存失败只记录日志，不返回错误。
func (c *TaskCache) GetDetailInfo(ctx context.Context, taskID uint64) (info *cacheobj.TaskDetailInfo, exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil, false, false
	}

	key := c.kb.TaskDetailInfoKey(taskID)

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get task detail info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false, false
	}
	if !ok {
		return nil, false, false
	}

	// 命中空值缓存，表示任务不存在。
	if value == CacheNullValue {
		return nil, false, true
	}

	valid := func(info *cacheobj.TaskDetailInfo) bool {
		if info == nil {
			return false
		}

		return info.ID == taskID &&
			info.ProjectID != 0 &&
			info.CreatorID != 0 &&
			strings.TrimSpace(info.Status) != ""
	}

	var infoCache cacheobj.TaskDetailInfo
	if err = codec.UnmarshalString(value, &infoCache); err == nil && valid(&infoCache) {
		return &infoCache, true, true
	}

	if err != nil {
		zap.L().Warn("unmarshal task detail info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}

	// 缓存内容异常，删除脏缓存后继续回源 DB。
	if delErr := c.store.Del(ctx, key); delErr != nil {
		zap.L().Warn("delete invalid task detail info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(delErr),
		)
	}

	return nil, false, false
}

// SetDetailInfoNull 写入任务详情信息空值缓存。
// 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *TaskCache) SetDetailInfoNull(ctx context.Context, taskID uint64) {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return
	}

	key := c.kb.TaskDetailInfoKey(taskID)

	if err := c.store.Set(ctx, key, CacheNullValue, TaskDetailInfoNullTTL); err != nil {
		zap.L().Warn("set task detail info null cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// SetDetailInfo 写入任务详情信息缓存。
// 注意：写缓存失败只记录日志，不影响正常业务逻辑。
func (c *TaskCache) SetDetailInfo(ctx context.Context, task *model.Task) {
	if c == nil || c.store == nil || c.kb == nil || c.conv == nil || task == nil || task.ID == 0 {
		return
	}

	key := c.kb.TaskDetailInfoKey(task.ID)

	info := c.conv.TaskDetailInfo(task)
	if info == nil {
		zap.L().Warn("convert task detail info cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
		)
		return
	}

	raw, err := codec.MarshalString(info)
	if err != nil {
		zap.L().Warn("marshal task detail info cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return
	}

	if err := c.store.Set(ctx, key, raw, TaskDetailInfoTTL); err != nil {
		zap.L().Warn("set task detail info cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// DeleteDetailInfo 删除任务详情信息缓存。
// 注意：删除缓存失败会记录日志并返回错误。
func (c *TaskCache) DeleteDetailInfo(ctx context.Context, taskID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil
	}

	key := c.kb.TaskDetailInfoKey(taskID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete task detail info cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// GetListItem 从缓存读取任务列表 item。
// 返回值：item / exists / hit
func (c *TaskCache) GetListItem(ctx context.Context, taskID uint64) (item *cacheobj.TaskListItem, exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil, false, false
	}

	key := c.kb.TaskListItemKey(taskID)

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get task list item cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false, false
	}
	if !ok {
		return nil, false, false
	}

	// 命中空值缓存。
	if value == CacheNullValue {
		return nil, false, true
	}

	var itemCache cacheobj.TaskListItem
	if err := codec.UnmarshalString(value, &itemCache); err != nil {
		// 缓存值异常，删除脏缓存后继续回源。
		if delErr := c.store.Del(ctx, key); delErr != nil {
			zap.L().Warn("delete invalid task list item cache failed",
				zap.Uint64("task_id", taskID),
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}

		zap.L().Warn("unmarshal task list item cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false, false
	}

	return &itemCache, true, true
}

// BatchGetListItem 批量从缓存获取任务表项信息。
// 返回值：
//   - map：缓存命中的任务Task Map
//   - slice：缓存未命中的任务ID
func (c *TaskCache) BatchGetListItem(ctx context.Context, taskIDs []uint64) (map[uint64]*cacheObj.TaskListItem, []uint64, error) {
	taskItemMap := make(map[uint64]*cacheObj.TaskListItem, len(taskIDs))
	missingTaskIDs := make([]uint64, 0)

	if len(taskIDs) == 0 {
		return taskItemMap, missingTaskIDs, nil
	}

	// 去重，避免重复查缓存和重复回源
	uniqueIDs := utils.Deduplicate(taskIDs)
	keys := make([]string, 0, len(taskIDs))
	for _, taskID := range uniqueIDs {
		keys = append(keys, c.kb.TaskListItemKey(taskID))
	}

	cacheMap, err := c.store.MGet(ctx, keys...)
	if err != nil {
		// MGet 异常时，直接当作全部 miss 处理，避免缓存故障影响主流程
		zap.L().Warn("mget task item cache failed",
			zap.Int("keys_len", len(keys)),
			zap.Error(err),
		)
		return taskItemMap, uniqueIDs, nil
	}

	for _, taskID := range uniqueIDs {
		key := c.kb.TaskListItemKey(taskID)
		value, ok := cacheMap[key]
		if !ok || value == "" {
			missingTaskIDs = append(missingTaskIDs, taskID)
			continue
		}

		var taskCache cacheObj.TaskListItem
		if err := codec.UnmarshalString(value, &taskCache); err != nil {
			// 脏缓存：删除后按 miss 处理
			zap.L().Warn("unmarshal task item cache failed",
				zap.Uint64("task_id", taskID),
				zap.Error(err),
			)
			if delErr := c.store.Del(ctx, key); delErr != nil {
				zap.L().Warn("delete dirty task item cache failed",
					zap.Uint64("task_id", taskID),
					zap.Error(delErr),
				)
			}
			missingTaskIDs = append(missingTaskIDs, taskID)
			continue
		}

		// 校验字段是否正确
		if taskCache.ID != taskID { // 脏数据 需要删除
			if delErr := c.store.Del(ctx, key); delErr != nil {
				zap.L().Warn("delete dirty task item cache failed",
					zap.Uint64("task_id", taskID),
					zap.String("cache_key", key),
					zap.Error(delErr),
				)
			}
			missingTaskIDs = append(missingTaskIDs, taskID)
			continue
		}

		taskItemMap[taskID] = utils.SafeGetPtr(taskCache)
	}

	return taskItemMap, missingTaskIDs, nil
}

// SetListItemNull 写入任务列表 item 空值缓存。
func (c *TaskCache) SetListItemNull(ctx context.Context, taskID uint64) {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return
	}

	key := c.kb.TaskListItemKey(taskID)

	if err := c.store.Set(ctx, key, CacheNullValue, TaskListItemNullTTL); err != nil {
		zap.L().Warn("set task list item null cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// SetListItem 写入任务列表 item 缓存。
func (c *TaskCache) SetListItem(ctx context.Context, task *model.Task) {
	if c == nil || c.store == nil || task == nil || task.ID == 0 {
		return
	}

	key := c.kb.TaskListItemKey(task.ID)
	taskItem := c.conv.TaskListItem(task)
	raw, err := codec.MarshalString(taskItem)
	if err != nil {
		zap.L().Warn("marshal task list item cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return
	}

	if err := c.store.Set(ctx, key, raw, TaskListItemTTL); err != nil {
		zap.L().Warn("set task list item cache failed",
			zap.Uint64("task_id", task.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// MSetListItem 批量写入任务列表 item 缓存。
//   - 注意：缓存写入失败只记录日志，不影响业务逻辑。
//   - 注意：所有缓存使用相同 TTL。
func (c *TaskCache) MSetListItem(ctx context.Context, tasks []*model.Task) {
	if c == nil || c.store == nil || c.kb == nil || len(tasks) == 0 {
		return
	}

	data := make(map[string]string, len(tasks))

	for _, task := range tasks {
		if task == nil || task.ID == 0 {
			continue
		}

		key := c.kb.TaskListItemKey(task.ID)
		taskItem := c.conv.TaskListItem(task)
		raw, err := codec.MarshalString(taskItem)
		if err != nil {
			zap.L().Warn("marshal task list item cache failed",
				zap.Uint64("task_id", task.ID),
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

	if err := c.store.MSet(ctx, data, TaskListItemTTL); err != nil {
		zap.L().Warn("multi set task list item cache failed",
			zap.Int("task_number", len(tasks)),
			zap.Int("key_number", len(data)),
			zap.Duration("ttl", TaskListItemTTL),
			zap.Error(err),
		)
	}
}

// DeleteListItem 删除任务列表 item 缓存。
func (c *TaskCache) DeleteListItem(ctx context.Context, taskID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil
	}

	key := c.kb.TaskListItemKey(taskID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete task list item cache failed",
			zap.Uint64("task_id", taskID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// GetListPage 从缓存读取项目任务列表页。
func (c *TaskCache) GetListPage(ctx context.Context, key string) (*cacheobj.TaskListPage, bool) {
	if c == nil || c.store == nil {
		return nil, false
	}

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get task list cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false
	}
	if !ok {
		return nil, false
	}

	var page cacheobj.TaskListPage
	if err := codec.UnmarshalString(value, &page); err != nil {
		// 脏缓存直接删除。
		if delErr := c.store.Del(ctx, key); delErr != nil {
			zap.L().Warn("delete invalid task list cache failed",
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}

		zap.L().Warn("unmarshal task list cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false
	}

	return &page, true
}

// SetListPage 写入任务列表缓存。
func (s *TaskCache) SetListPage(ctx context.Context, key string, page *cacheobj.TaskListPage) {
	if s.store == nil || page == nil {
		return
	}

	cacheValue, marshalErr := codec.MarshalString(page) // 解析加入的数据
	if marshalErr != nil {
		zap.L().Warn("marshal task list cache failed",
			zap.Int("task_len", len(page.List)),
			zap.String("cache_key", key),
			zap.Error(marshalErr),
		)
	} else if setErr := s.store.Set(ctx, key, cacheValue, TaskListTTL); setErr != nil {
		zap.L().Warn("set project task list cache failed",
			zap.String("cache_key", key),
			zap.Error(setErr),
		)
	}
}

// BumpListPage 迭代任务表项缓存版本号
func (c *TaskCache) BumpListPage(ctx context.Context) error {
	if c == nil || c.kb == nil {
		return nil
	}

	c.kb.BumpTaskVersion() // 内部迭代版本号
	return nil
}

// DeleteAll 删除任务模块相关所有缓存。
func (c *TaskCache) DeleteAll(ctx context.Context, taskID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || taskID == 0 {
		return nil
	}

	permissionKey := c.kb.TaskPermissionInfoKey(taskID)
	DetailKey := c.kb.TaskDetailInfoKey(taskID)
	listItemKey := c.kb.TaskListItemKey(taskID)
	c.kb.BumpTaskVersion()

	if err := c.store.Del(ctx, permissionKey, DetailKey, listItemKey); err != nil {
		zap.L().Warn("delete task all cache failed",
			zap.Uint64("task_id", taskID),
			zap.Strings("cache_keys", []string{permissionKey, listItemKey}),
			zap.Error(err),
		)
		return err
	}

	return nil
}
