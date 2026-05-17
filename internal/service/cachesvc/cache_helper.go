// internal/service/cachesvc/cache_helper.go
// Package cachesvc
// 实现工具辅助函数
package cachesvc

import (
	"context"
	"errors"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service/cachesvc/cacheobj"
	cacheObj "smart-task-platform/internal/service/cachesvc/cacheobj"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

var (
	errInvalidSingleflightResult = errors.New("invalid singleflight result")
	errInvalidListBundle         = errors.New("invalid list bundle")
	errNilLoadInsideSF           = errors.New("nil loadInsideSF")
)

// sfLogMeta helper 日志元信息
type sfLogMeta struct {
	Message string
	Fields  []zap.Field
}

//=============
// 单飞泛型
//=============

// doCacheSF 通用缓存单飞模板
//
//  1. fastGet 查缓存
//
//  2. loadInsideSF  如果外层的缓存 miss ，执行内部的单飞逻辑 如在查缓存+数据库
//
// 适用场景：
//
//   - exists cache
//
//   - brief info cache
//
//   - list item cache
//
//   - detail info cache
func doCacheSF[T any](
	sf *singleflight.Group,
	key string,
	fastGet func() (T, bool),
	loadInsideSF func() (T, error),
	logMeta sfLogMeta,
) (T, error) {
	var zero T

	if fastGet != nil {
		if val, hit := fastGet(); hit {
			return val, nil
		}
	}

	if loadInsideSF == nil {
		return zero, errNilLoadInsideSF
	}

	if sf == nil {
		return loadInsideSF()
	}

	value, err, _ := sf.Do(key, func() (any, error) {
		return loadInsideSF()
	})
	if err != nil {
		return zero, err
	}

	result, ok := value.(T)
	if !ok {
		msg := logMeta.Message
		if msg == "" {
			msg = "invalid singleflight result"
		}
		zap.L().Warn(msg, logMeta.Fields...)
		return zero, errInvalidSingleflightResult
	}

	return result, nil
}

//===========
// 加载对象 singleflight 模板
//===========

// objectSFBundle singleflight 中使用的打包结构。
//   - Cache：缓存结构。
//   - Item：DB 查询得到的业务对象。
//   - Exists：业务对象是否存在。
//   - FromCache：是否来自缓存。
//
// 说明：
// 和普通对象不一样的是：这里多了一个 FromCache。
//
// 普通 cache 对象通常在模板内部直接转换成 model，调用方无需关心来源。
// 但这个通用方法会把 singleflight 中的数据带到外部再转换，
// 这样可以支持更通用的场景，例如：缓存命中后还需要预加载关联对象。
type objectSFBundle[T any, C any] struct {
	Cache     C
	Item      T
	Exists    bool // 业务对象是否存在
	FromCache bool // 是否来自缓存
}

// objectSFConfig 用配置结构承载 object singleflight 所需的行为，避免函数参数过长。
type objectSFConfig[T any, C any] struct {
	SF  *singleflight.Group
	Key string

	// GetCache 读取缓存，返回 cache, exists, hit。
	//   - hit：表示缓存是否命中。
	//   - exists：表示业务对象是否存在。
	//
	// 注意：
	//   - hit=false：缓存不存在，需要继续回源。
	//   - hit=true && exists=false：命中空值缓存，表示业务对象不存在。
	//   - hit=true && exists=true：命中正缓存，需要继续校验缓存内容是否有效。
	GetCache func(ctx context.Context) (C, bool, bool)

	// IsCacheValid 判断正缓存对象是否有效。
	//
	// 如果命中正缓存，但是 IsCacheValid 返回 false，
	// 不再把它当成“业务对象不存在”，而是认为这是“脏缓存 / 无效缓存”。
	// 模板会：
	//   1. 记录 warn 日志；
	//   2. 调用 DelCache 删除脏缓存；
	//   3. 将本次读取视为 cache miss；
	//   4. 继续进入 singleflight / DB 回源流程。
	IsCacheValid func(C) bool

	// DelCache 删除当前对象缓存。
	// 注意：
	//   - DelCache 一般只删除当前 key 对应的对象缓存。
	//   - DelCache 失败建议在具体 cache 层内部记录日志，不建议影响主业务流程。
	//   - 如果不配置 DelCache，模板只会记录日志并把无效缓存视为 miss。
	DelCache func(ctx context.Context)

	// LoadDB DB 回源。
	LoadDB func(ctx context.Context) (T, error)

	// IsItemValid 判断 DB 返回对象是否有效。
	//
	// 注意：
	// DB 返回无效对象，通常视为业务对象不存在，会写入空值缓存。
	IsItemValid func(T) bool

	// SetCache 写正缓存。
	SetCache func(ctx context.Context, item T)

	// SetNull 写空值缓存。
	SetNull func(ctx context.Context)

	// AfterLoadDB DB 回源成功后的额外处理。
	// 例如：顺手写 project/user brief 缓存。
	AfterLoadDB func(ctx context.Context, item T)

	// BuildFromCache 缓存命中时，根据缓存对象构造最终 model。
	//
	// 注意：
	// 如果存在正缓存命中的可能，通常必须配置该函数。
	BuildFromCache func(ctx context.Context, cache C) (T, error)

	// NotFoundErr DB 查询不存在时的错误。
	// 例如 repository.ErrTaskNotFound。
	NotFoundErr error

	// LogMeta 日志元信息。
	LogMeta sfLogMeta
}

// loadObjectByBundleWithSF 加载 model 对象。
//
// 返回值：
//
//   - T：业务对象。
//
//   - bool：业务对象是否存在。
//
//   - error：执行错误。
//
//     正缓存无效时，不再当作业务不存在，而是：
//
//     记录日志 -> 删除脏缓存 -> 视为 cache miss -> 继续 DB 回源。
func loadObjectByBundleWithSF[T any, C any](ctx context.Context, cfg *objectSFConfig[T, C]) (T, bool, error) {
	var zeroT T
	var zeroC C

	if cfg == nil || cfg.Key == "" {
		return zeroT, false, nil
	}

	// logInvalidCache 记录无效正缓存日志。
	// 这里不返回错误，因为缓存脏数据不应该直接影响主业务流程。
	logInvalidCache := func(stage string) {
		fields := make([]zap.Field, 0, len(cfg.LogMeta.Fields)+2)
		fields = append(fields,
			zap.String("cache_key", cfg.Key),
			zap.String("stage", stage),
		)
		fields = append(fields, cfg.LogMeta.Fields...)

		zap.L().Warn("invalid object cache, delete dirty cache and treat as cache miss", fields...)
	}

	// deleteDirtyCache 删除脏缓存。
	//
	// 注意：
	// DelCache 不返回 error，删除失败建议由具体 cache 层内部记录日志。
	deleteDirtyCache := func(ctx context.Context) {
		if cfg.DelCache == nil {
			return
		}

		cfg.DelCache(ctx)
	}

	// loadFromCache 从缓存中读取并打包结果。
	//
	// 返回值：
	//   - bundle：缓存命中时的包装结果。
	//   - bool：是否真正命中“可用缓存”。
	//
	// 注意：
	// 如果命中的是无效正缓存，本函数会删除脏缓存，并返回 nil, false，
	// 让外层继续按照 cache miss 处理。
	loadFromCache := func(ctx context.Context, stage string) (*objectSFBundle[T, C], bool) {
		if cfg.GetCache == nil {
			return nil, false
		}

		cache, exists, hit := cfg.GetCache(ctx)
		if !hit {
			return nil, false
		}

		// 命中空值缓存。
		if !exists {
			return &objectSFBundle[T, C]{
				Cache:     zeroC,
				Item:      zeroT,
				Exists:    false,
				FromCache: true,
			}, true
		}

		// 命中正缓存，但缓存对象无效。
		//
		// 重要：
		// 这里不能返回 Exists=false。
		// 因为“缓存对象无效”不等于“业务对象不存在”。
		if cfg.IsCacheValid != nil && !cfg.IsCacheValid(cache) {
			logInvalidCache(stage)
			deleteDirtyCache(ctx)
			return nil, false
		}

		// 命中有效正缓存。
		return &objectSFBundle[T, C]{
			Cache:     cache,
			Item:      zeroT,
			Exists:    true,
			FromCache: true,
		}, true
	}

	// loadBundle 是 singleflight 内部执行的完整加载逻辑：
	// 二次查缓存 -> DB 回源 -> 写缓存。
	loadBundle := func() (*objectSFBundle[T, C], error) {
		// singleflight 内二次查缓存。
		if bundle, hit := loadFromCache(ctx, "singleflight_second_get"); hit {
			return bundle, nil
		}

		if cfg.LoadDB == nil {
			return &objectSFBundle[T, C]{
				Cache:     zeroC,
				Item:      zeroT,
				Exists:    false,
				FromCache: false,
			}, nil
		}

		// DB 回源。
		item, err := cfg.LoadDB(ctx)
		if err != nil {
			// DB 明确返回不存在，写入空值缓存。
			if cfg.NotFoundErr != nil && errors.Is(err, cfg.NotFoundErr) {
				if cfg.SetNull != nil {
					cfg.SetNull(ctx)
				}

				return &objectSFBundle[T, C]{
					Cache:     zeroC,
					Item:      zeroT,
					Exists:    false,
					FromCache: false,
				}, nil
			}

			// 其它 DB 错误直接返回。
			return nil, err
		}

		// DB 返回无效对象，也视为业务对象不存在。
		// 例如：返回 nil 指针、ID 为 0、role 为空字符串等。
		if cfg.IsItemValid != nil && !cfg.IsItemValid(item) {
			if cfg.SetNull != nil {
				cfg.SetNull(ctx)
			}

			return &objectSFBundle[T, C]{
				Cache:     zeroC,
				Item:      zeroT,
				Exists:    false,
				FromCache: false,
			}, nil
		}

		// 写正缓存。
		if cfg.SetCache != nil {
			cfg.SetCache(ctx, item)
		}

		// DB 回源成功后的额外缓存处理。
		if cfg.AfterLoadDB != nil {
			cfg.AfterLoadDB(ctx, item)
		}

		return &objectSFBundle[T, C]{
			Cache:     zeroC,
			Item:      item,
			Exists:    true,
			FromCache: false,
		}, nil
	}

	// 第一次查缓存。
	if bundle, hit := loadFromCache(ctx, "fast_get"); hit {
		return returnObjectByBundle(ctx, bundle, cfg.BuildFromCache, cfg.IsItemValid, cfg.LogMeta)
	}

	// 正式执行 singleflight。
	bundle, err := doCacheSF(
		cfg.SF,
		cfg.Key,
		nil,
		loadBundle,
		cfg.LogMeta,
	)
	if err != nil {
		return zeroT, false, err
	}

	return returnObjectByBundle(ctx, bundle, cfg.BuildFromCache, cfg.IsItemValid, cfg.LogMeta)
}

// returnObjectByBundle 根据 objectSFBundle 构造最终返回值。
//
// 修改点说明：
//  1. 增加 logMeta 参数，用于在缓存命中但 BuildFromCache 未配置时打印日志。
//  2. 缓存命中正缓存时，如果 BuildFromCache 为空，会记录 warn，方便排查配置遗漏。
//  3. 其它主流程不变。
func returnObjectByBundle[T any, C any](
	ctx context.Context,
	bundle *objectSFBundle[T, C],
	buildFromCache func(ctx context.Context, cache C) (T, error),
	isItemValid func(T) bool,
	logMeta sfLogMeta,
) (T, bool, error) {
	var zero T

	if bundle == nil || !bundle.Exists {
		return zero, false, nil
	}

	// 缓存 miss，DB 已经全量查询，直接返回 DB 对象。
	if !bundle.FromCache {
		if isItemValid != nil && !isItemValid(bundle.Item) {
			return zero, false, nil
		}

		return bundle.Item, true, nil
	}

	// 缓存 hit，需要根据缓存对象构造最终 model。
	if buildFromCache == nil {
		fields := make([]zap.Field, 0, len(logMeta.Fields))
		fields = append(fields, logMeta.Fields...)

		zap.L().Warn("cache hit but BuildFromCache is nil", fields...)
		return zero, false, nil
	}

	item, err := buildFromCache(ctx, bundle.Cache)
	if err != nil {
		return zero, false, err
	}

	if isItemValid != nil && !isItemValid(item) {
		return zero, false, nil
	}

	return item, true, nil
}

//===========
// 列表单飞模板
//============

// listSFBundle
// 说明：
//  1. Page 是缓存对象，只保存 taskID 列表
//  2. Items 只在缓存 miss 且回源成功时有值
//  3. FromCache 表示 page 是否来自缓存
type listSFBundle[T any] struct {
	Page      *cacheObj.ListPage
	Items     []T
	FromCache bool
}

// loadAndReturnListPage 通用列表页加载并返回最终结果
//   - page cache hit：根据 page.List 批量恢复 items
//   - page cache miss：DB 返回完整 items，本次请求直接复用
//   - page cache 只保存 ID 列表
func loadAndReturnListPage[T, Q any](
	ctx context.Context,
	sf *singleflight.Group,
	key string,
	query Q,
	getPage func(ctx context.Context, key string) (*cacheobj.ListPage, bool),
	setPage func(ctx context.Context, key string, page *cacheobj.ListPage),
	searchWithQuery func(ctx context.Context, query Q) (*repository.SearchResult[T], error),
	buildPage func(page, pageSize int, result *repository.SearchResult[T], fn func(val T) uint64) *cacheobj.ListPage,
	afterSearch func(ctx context.Context, items []T),
	buildFromPage func(ctx context.Context, ids []uint64) ([]T, error),
	page, pageSize int,
	fn func(val T) uint64,
	logMeta sfLogMeta,
) ([]T, *int64, bool, error) {
	bundle, err := loadListPageBundle(
		ctx,
		sf,
		key,
		query,
		getPage,
		setPage,
		searchWithQuery,
		buildPage,
		afterSearch,
		page,
		pageSize,
		fn,
		logMeta,
	)
	if err != nil {
		return nil, nil, false, err
	}

	return returnListByBundle(ctx, bundle, buildFromPage)
}

// returnListByBundle 根据打包的结果，返回查询的结果
// 注意:
//   - 外部需要先对 bundle 进行差错处理
func returnListByBundle[T any](
	ctx context.Context,
	bundle *listSFBundle[T],
	buildFromPage func(ctx context.Context, ids []uint64) ([]T, error),
) ([]T, *int64, bool, error) {
	if bundle == nil || bundle.Page == nil {
		return nil, nil, false, nil
	}

	if !bundle.FromCache {
		return bundle.Items, bundle.Page.Total, bundle.Page.HasMore, nil
	}

	tasks, err := buildFromPage(ctx, bundle.Page.List)
	if err != nil {
		return nil, nil, false, err
	}

	return tasks, bundle.Page.Total, bundle.Page.HasMore, nil
}

// loadListPageBundle 通用列表页缓存模板
//  1. getPage 获取页缓存
//  2. setPage 设置页缓存
//  3. searchWithQuery 数据库的查询方法（包装一层查询参数）
//  4. buildPage 利用数据库的查询结构构造缓存页
//  5. setItem 缓存多表项
//
// 说明：
//   - page cache 只保存 ID 列表
//   - miss 时 DB 返回完整 items，本次请求直接复用
//   - hit 时调用方根据 Page.List 批量恢复 model
func loadListPageBundle[T, Q any](
	ctx context.Context,
	sf *singleflight.Group,
	key string,
	query Q,
	getPage func(ctx context.Context, key string) (*cacheobj.ListPage, bool),
	setPage func(ctx context.Context, key string, page *cacheobj.ListPage),
	searchWithQuery func(ctx context.Context, query Q) (*repository.SearchResult[T], error),
	buildPage func(page, pageSize int, result *repository.SearchResult[T], fn func(val T) uint64) *cacheobj.ListPage,
	afterSearch func(ctx context.Context, items []T),
	page, pageSize int,
	fn func(val T) uint64,
	logMeta sfLogMeta,
) (*listSFBundle[T], error) {
	if key == "" {
		return nil, nil
	}

	if page, hit := getPage(ctx, key); hit {
		return &listSFBundle[T]{
			Page:      page,
			FromCache: true,
		}, nil
	}

	result, err := doCacheSF(
		sf,
		key,
		nil, // 这里传 nil，是因为外层已经读取一次读取缓存了
		func() (listSFBundle[T], error) {
			// 单飞模式：二次查缓存
			if page, hit := getPage(ctx, key); hit {
				return listSFBundle[T]{
					Page:      page,
					FromCache: true,
				}, nil
			}

			// 回源查询
			// 在 Search 场景下会进行预加载
			// 这些搜索结果直接复用
			searchResult, err := searchWithQuery(ctx, query)
			if err != nil {
				return listSFBundle[T]{}, err
			}

			pageCache := buildPage(page, pageSize, searchResult, fn)
			if pageCache != nil && setPage != nil {
				setPage(ctx, key, pageCache)
			}

			if searchResult != nil && len(searchResult.List) > 0 && afterSearch != nil {
				afterSearch(ctx, searchResult.List)
			}

			var items []T
			if searchResult != nil {
				items = searchResult.List
			}

			return listSFBundle[T]{
				Page:      pageCache,
				Items:     items,
				FromCache: false,
			}, nil
		},
		logMeta,
	)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

//============
// 存在性单飞模板
//=============

type existsSFResult struct {
	Exists bool
}

// loadExistsWithSF 存在性单飞模板
// 注意：
//   - key 可能是需要外部传入的
//   - getCache/setCache 没有传入key，是因为差异化 有些存在性判断可能需要多个ID
func loadExistsWithSF(
	ctx context.Context,
	sf *singleflight.Group,
	key string,
	getCache func(ctx context.Context) (bool, bool),
	setCache func(ctx context.Context, exists bool),
	loadDB func(ctx context.Context) (bool, error),
	notFoundErr error,
	logMeta sfLogMeta,
) (bool, error) {
	if sf == nil {
		return false, nil
	}

	// 第一次读缓存
	if exists, hit := getCache(ctx); hit {
		return exists, nil
	}

	value, err, _ := sf.Do(key, func() (any, error) {
		// singleflight 内二次读缓存
		if exists, hit := getCache(ctx); hit {
			return existsSFResult{Exists: exists}, nil
		}

		// 二次缓存仍未命中，查询 DB
		exists, err := loadDB(ctx)
		if err != nil {
			if notFoundErr != nil && errors.Is(err, notFoundErr) {
				setCache(ctx, false)
				return existsSFResult{Exists: false}, nil
			}
			return nil, err
		}

		// 用户是否存在写入缓存
		setCache(ctx, exists)
		return existsSFResult{Exists: exists}, nil
	})
	if err != nil {
		return false, err
	}

	result, ok := value.(existsSFResult)
	if !ok {
		zap.L().Warn(logMeta.Message, logMeta.Fields...)
		return false, nil
	}

	return result.Exists, nil
}
