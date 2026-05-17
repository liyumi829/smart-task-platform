// internal/service/cachesvc/user_cache.go
// Package cachesvc
// 实现用户模块的缓存服务操作
package cachesvc

import (
	"context"
	cacheStore "smart-task-platform/internal/cache"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/codec"
	"smart-task-platform/internal/pkg/utils"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"

	"go.uber.org/zap"
)

// UserCache 用户缓存
type UserCache struct {
	store cacheStore.Store // 操作句柄 Set/Del/Get/MGet
	kb    *keyBuilder      // key建造者
	conv  *Converter       // 类型转化器
}

// NewUserCache 创建用户缓存实例
func NewUserCache(store cacheStore.Store, kb *keyBuilder, conv *Converter) *UserCache {
	return &UserCache{
		store: store,
		kb:    kb,
		conv:  conv,
	}
}

// ExistsKey 获取用户存在性Key
func (c *UserCache) ExistsKey(projectID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.UserExistsKey(projectID)
}

// BriefInfoKey 获取用户简要信息 Key 方法
func (c *UserCache) BriefInfoKey(projectID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.UserBriefInfoKey(projectID)
}

func (c *UserCache) ListKey(query *cacheObj.UserListQuery) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.UserListKey(query)
}

func (c *UserCache) ProjectIDsKey(userID uint64) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.UserProjectIDs(userID)
}

func (c *UserCache) ProjectListKey(query *cacheObj.UserProjectListQuery) string {
	if c == nil || c.kb == nil {
		return ""
	}

	return c.kb.UserProjectListKey(query)
}

// GetExists 从缓存读取用户是否存在。
//   - 返回值：exists 表示是否存在，hit 表示缓存是否有效命中。
//   - 注意：缓存失败只记录日志，不返回错误。
func (c *UserCache) GetExists(ctx context.Context, userID uint64) (exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return false, false
	}
	key := c.kb.UserExistsKey(userID)
	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get user exists cache failed",
			zap.Uint64("user_id", userID),
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
			zap.L().Warn("delete invalid user exists cache failed",
				zap.Uint64("user_id", userID),
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}
		return false, false
	}
}

// SetExists 将用户是否存在信息写入缓存中
func (c *UserCache) SetExists(ctx context.Context, userID uint64, exists bool) {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return
	}

	key := c.kb.UserExistsKey(userID)

	cacheValue := CacheBoolFalse
	cacheTTL := UserExistsNullTTL

	if exists {
		cacheValue = CacheBoolTrue
		cacheTTL = UserExistsTTL
	}

	if err := c.store.Set(ctx, key, cacheValue, cacheTTL); err != nil {
		zap.L().Warn("set user exists cache failed",
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Bool("exists", exists),
			zap.Error(err),
		)
	}
}

// DeleteExists 删除用户存在性缓存。
func (c *UserCache) DeleteExists(ctx context.Context, userID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return nil
	}

	key := c.kb.UserExistsKey(userID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete user exists cache failed",
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// GetBriefInfo 从缓存读取用户简要信息。
//   - 返回值：info 表示用户简要信息，exists 表示用户是否存在，hit 表示缓存是否有效命中。
//   - 注意：缓存失败只记录日志，不返回错误。
func (c *UserCache) GetBriefInfo(ctx context.Context, userID uint64) (info *cacheObj.UserBriefInfo, exists bool, hit bool) {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return nil, false, false
	}

	key := c.kb.UserBriefInfoKey(userID)
	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get user brief info cache failed",
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false, false
	}
	if !ok {
		return nil, false, false
	}

	// 命中空值缓存，表示用户不存在。
	if value == CacheNullValue {
		return nil, false, true
	}

	var briefInfo cacheObj.UserBriefInfo
	if err = codec.UnmarshalString(value, &briefInfo); err == nil && briefInfo.ID != 0 {
		return &briefInfo, true, true
	}

	// 缓存内容异常，删除脏缓存后继续回源 DB。
	if delErr := c.store.Del(ctx, key); delErr != nil {
		zap.L().Warn("delete invalid user brief info cache failed",
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(delErr),
		)
	}

	return nil, false, false
}

// BatchGetBriefInfo 批量从缓存获取用户简要信息。
// 返回值：
//   - briefInfoMap：命中的用户简要信息，key 为 userID
//   - missingUserIDs：缓存未命中的用户 ID
func (c *UserCache) BatchGetBriefInfo(ctx context.Context, userIDs []uint64) (map[uint64]*cacheObj.UserBriefInfo, []uint64, error) {
	briefInfoMap := make(map[uint64]*cacheObj.UserBriefInfo, len(userIDs))
	missingUserIDs := make([]uint64, 0, len(userIDs))

	if len(userIDs) == 0 {
		return briefInfoMap, missingUserIDs, nil
	}

	// 去重并过滤非法 ID，避免查询 user:brief:0 这类无效缓存。
	uniqueIDs := make([]uint64, 0, len(userIDs))
	for _, userID := range utils.Deduplicate(userIDs) {
		if userID == 0 {
			continue
		}
		uniqueIDs = append(uniqueIDs, userID)
	}

	if len(uniqueIDs) == 0 {
		return briefInfoMap, missingUserIDs, nil
	}

	keys := make([]string, 0, len(uniqueIDs))
	idToKey := make(map[uint64]string, len(uniqueIDs))

	for _, userID := range uniqueIDs {
		key := c.kb.UserBriefInfoKey(userID)
		keys = append(keys, key)
		idToKey[userID] = key
	}

	cacheMap, err := c.store.MGet(ctx, keys...)
	if err != nil {
		// MGet 异常时，直接当作全部 miss 处理，避免缓存故障影响主流程。
		zap.L().Warn("mget user brief cache failed",
			zap.Int("keys_len", len(keys)),
			zap.Error(err),
		)
		return briefInfoMap, uniqueIDs, nil
	}

	for _, userID := range uniqueIDs {
		key := idToKey[userID]

		value, ok := cacheMap[key]
		if !ok || value == "" {
			missingUserIDs = append(missingUserIDs, userID)
			continue
		}

		var userCache cacheObj.UserBriefInfo
		if err := codec.UnmarshalString(value, &userCache); err != nil {
			// 脏缓存：反序列化失败，删除后按 miss 处理。
			zap.L().Warn("unmarshal user brief cache failed",
				zap.Uint64("user_id", userID),
				zap.String("cache_key", key),
				zap.Error(err),
			)

			if delErr := c.store.Del(ctx, key); delErr != nil {
				zap.L().Warn("delete dirty user brief cache failed",
					zap.Uint64("user_id", userID),
					zap.String("cache_key", key),
					zap.Error(delErr),
				)
			}

			missingUserIDs = append(missingUserIDs, userID)
			continue
		}

		if userCache.ID != userID {
			// 脏缓存：key 中的 userID 和 value 中的 userID 不一致。
			zap.L().Warn("user brief cache id mismatch",
				zap.Uint64("request_user_id", userID),
				zap.Uint64("cache_user_id", userCache.ID),
				zap.String("cache_key", key),
			)

			if delErr := c.store.Del(ctx, key); delErr != nil {
				zap.L().Warn("delete dirty user brief cache failed",
					zap.Uint64("request_user_id", userID),
					zap.Uint64("cache_user_id", userCache.ID),
					zap.String("cache_key", key),
					zap.Error(delErr),
				)
			}

			missingUserIDs = append(missingUserIDs, userID)
			continue
		}

		briefInfoMap[userID] = utils.SafeGetPtr(userCache)
	}

	return briefInfoMap, missingUserIDs, nil
}

// SetBriefInfo 设置 User 简要信息缓存
func (c *UserCache) SetBriefInfo(ctx context.Context, user *model.User) {
	if c == nil || c.store == nil || c.kb == nil || c.conv == nil || user == nil || user.ID == 0 {
		return
	}

	key := c.kb.UserBriefInfoKey(user.ID)

	userCache := c.conv.UserBriefInfo(user)

	raw, err := codec.MarshalString(userCache)
	if err != nil {
		zap.L().Warn("marshal user brief info cache failed",
			zap.Uint64("user_id", user.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return
	}

	if err := c.store.Set(ctx, key, raw, UserBriefInfoTTL); err != nil {
		zap.L().Warn("set user brief info cache failed",
			zap.Uint64("user_id", user.ID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// MSetBriefInfo 批量设置 User 简要信息缓存。
//   - 注意：缓存写入失败只记录日志，不影响业务逻辑。
//   - 注意：所有缓存使用相同 TTL。
func (c *UserCache) MSetBriefInfo(ctx context.Context, users []*model.User) {
	if c == nil || c.store == nil || c.kb == nil || c.conv == nil || len(users) == 0 {
		return
	}

	data := make(map[string]string, len(users))

	for _, user := range users {
		if user == nil || user.ID == 0 {
			continue
		}

		key := c.kb.UserBriefInfoKey(user.ID)

		info := c.conv.UserBriefInfo(user)
		if info == nil {
			zap.L().Warn("convert user brief info cache failed",
				zap.Uint64("user_id", user.ID),
				zap.String("cache_key", key),
			)
			continue
		}

		raw, err := codec.MarshalString(info)
		if err != nil {
			zap.L().Warn("marshal user brief info cache failed",
				zap.Uint64("user_id", user.ID),
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

	if err := c.store.MSet(ctx, data, UserBriefInfoTTL); err != nil {
		zap.L().Warn("multi set user brief info cache failed",
			zap.Int("user_number", len(users)),
			zap.Int("key_number", len(data)),
			zap.Duration("ttl", UserBriefInfoTTL),
			zap.Error(err),
		)
	}
}

// SetBriefInfoNull 给用户简要信息设置空值缓存
func (c *UserCache) SetBriefInfoNull(ctx context.Context, userID uint64) {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return
	}

	key := c.kb.UserBriefInfoKey(userID)

	if err := c.store.Set(ctx, key, CacheNullValue, UserBriefInfoNullTTL); err != nil {
		zap.L().Warn("set user brief info null cache failed",
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// DeleteBriefInfo 删除用户简要信息缓存。
func (c *UserCache) DeleteBriefInfo(ctx context.Context, userID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return nil
	}

	key := c.kb.UserBriefInfoKey(userID)

	if err := c.store.Del(ctx, key); err != nil {
		zap.L().Warn("delete user brief info cache failed",
			zap.Uint64("user_id", userID),
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// GetListPage 从缓存读取用户列表页。
func (c *UserCache) GetListPage(ctx context.Context, key string) (*cacheObj.UserListPage, bool) {
	if c == nil || c.store == nil {
		return nil, false
	}

	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get user list cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false
	}
	if !ok {
		return nil, false
	}

	var page cacheObj.UserListPage
	if err := codec.UnmarshalString(value, &page); err != nil {
		// 脏缓存直接删除。
		if delErr := c.store.Del(ctx, key); delErr != nil {
			zap.L().Warn("delete invalid user list cache failed",
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}

		zap.L().Warn("unmarshal user list cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false
	}

	return &page, true
}

// SetListPage 写入任务列表缓存。
func (c *UserCache) SetListPage(ctx context.Context, key string, page *cacheObj.UserListPage) {
	if c.store == nil || page == nil {
		return
	}

	cacheValue, marshalErr := codec.MarshalString(page) // 解析加入的数据
	if marshalErr != nil {
		zap.L().Warn("marshal user list cache failed",
			zap.Int("user_len", len(page.List)),
			zap.String("cache_key", key),
			zap.Error(marshalErr),
		)
	} else if setErr := c.store.Set(ctx, key, cacheValue, UserListTTL); setErr != nil {
		zap.L().Warn("set project user list cache failed",
			zap.String("cache_key", key),
			zap.Error(setErr),
		)
	}
}

// BumpListPage 迭代任务表项缓存版本号
func (c *UserCache) BumpListPage(ctx context.Context) error {
	if c == nil || c.kb == nil {
		return nil
	}

	c.kb.BumpUserVersion() // 内部迭代版本号
	return nil
}

// GetProjectIDs 从缓存读取用户下的项目列表
func (c *UserCache) GetProjectIDs(ctx context.Context, userID uint64) (*cacheObj.UserProjectIDs, bool) {
	if c == nil || c.store == nil {
		return nil, false
	}

	key := c.kb.UserProjectIDs(userID)
	value, ok, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get user projectIDs list cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
		return nil, false
	}
	if !ok {
		return nil, false
	}

	var ids cacheObj.UserProjectIDs
	if err := codec.UnmarshalString(value, &ids); err != nil {
		// 脏缓存直接删除。
		if delErr := c.store.Del(ctx, key); delErr != nil {
			zap.L().Warn("delete invalid user projectIDs list cache failed",
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}

		zap.L().Warn("unmarshal user projectIDs list cache failed",
			zap.String("cache_key", key),
			zap.Error(err))

		return nil, false
	}

	return &ids, true
}

// SetProjectIDs 写入项目缓存列表
func (c *UserCache) SetProjectIDs(ctx context.Context, userID uint64, ids *cacheObj.UserProjectIDs) {
	if c == nil || c.store == nil || ids == nil {
		return
	}

	if ids.ProjectIDs == nil {
		ids.ProjectIDs = []uint64{}
	}

	key := c.kb.UserProjectIDs(userID)
	cacheValue, marshalErr := codec.MarshalString(ids) // 解析加入的数据
	if marshalErr != nil {
		zap.L().Warn("marshal user project id list cache failed",
			zap.Int("projectIDs list_len", len(ids.ProjectIDs)),
			zap.String("cache_key", key),
			zap.Error(marshalErr),
		)
	} else if setErr := c.store.Set(ctx, key, cacheValue, UserProjectIDsTTL); setErr != nil {
		zap.L().Warn("set user project id list cache failed",
			zap.String("cache_key", key),
			zap.Error(setErr),
		)
	}
}

// GetProjectListPage 获取用户的查询项目页
func (c *UserCache) GetProjectListPage(ctx context.Context, key string) (*cacheObj.UserProjectListPage, bool) {
	if c == nil || c.store == nil {
		return nil, false
	}

	// 获取缓存
	raw, hit, err := c.store.Get(ctx, key)
	if err != nil {
		zap.L().Warn("get user project list page cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
	if !hit {
		return nil, false
	}

	// 命中了，解析
	var page cacheObj.UserProjectListPage
	if err := codec.UnmarshalString(raw, &page); err != nil {
		zap.L().Warn("unmarshal user project list page cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)

		// 脏缓存直接删除
		if delErr := c.store.Del(ctx, key); delErr != nil {
			zap.L().Warn("delete invalid user project list page cache failed",
				zap.String("cache_key", key),
				zap.Error(delErr),
			)
		}
		return nil, false
	}

	return &page, true
}

// SetProjectListPage 写入用户的查询项目页
func (c *UserCache) SetProjectListPage(ctx context.Context, key string, page *cacheObj.UserProjectListPage) {
	if c == nil || c.store == nil {
		return
	}

	// 先获取 value
	cacheValue, marshalErr := codec.MarshalString(page)
	if marshalErr != nil {
		zap.L().Warn("marshal user project list page cache failed",
			zap.String("cache_key", key),
			zap.Error(marshalErr),
		)
		return
	}

	if err := c.store.Set(ctx, key, cacheValue, UserProjectListTTL); err != nil {
		zap.L().Warn("set user project list page cache failed",
			zap.String("cache_key", key),
			zap.Error(err),
		)
	}
}

// BumpUserProjectList 迭代任务表项缓存版本号
func (c *UserCache) BumpUserProjectList(ctx context.Context) error {
	if c == nil || c.kb == nil {
		return nil
	}

	c.kb.BumpUserProjectListVersion() // 内部迭代版本号
	return nil
}

// DeleteAll 删除用户相关所有缓存。
func (c *UserCache) DeleteAll(ctx context.Context, userID uint64) error {
	if c == nil || c.store == nil || c.kb == nil || userID == 0 {
		return nil
	}

	existsKey := c.kb.UserExistsKey(userID)
	briefInfoKey := c.kb.UserBriefInfoKey(userID)
	c.kb.BumpUserVersion() // 迭代版本号

	if err := c.store.Del(ctx, existsKey, briefInfoKey); err != nil {
		zap.L().Warn("delete user all cache failed",
			zap.Uint64("user_id", userID),
			zap.Strings("cache_keys", []string{existsKey, briefInfoKey}),
			zap.Error(err),
		)
		return err
	}

	return nil
}
