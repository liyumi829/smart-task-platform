// internal/service/cachesvc/cache_version.go
// Package cachesvc
// 维护一个版本迭代器

package cachesvc

import (
	"sync"
)

// ListKind 列表类型。
type ListKind string

const (
	ListKindUser        ListKind = "user"
	ListKindUserProject ListKind = "user_project"
	ListKindTask        ListKind = "task"
	ListKindTaskComment ListKind = "task_comment"
)

// ListVersionRegistry 列表版本注册器。
// 说明：
//  1. 每种列表独立维护版本号，互不影响。
//  2. 版本号只控制缓存命名空间，不影响业务数据库结构。
//  3. 当列表结构或组装字段变更时，只需 bump 对应列表版本即可。
type ListVersionRegistry struct {
	mu       sync.RWMutex
	versions map[ListKind]uint64
}

// NewListVersionRegistry 创建版本注册器。
// 默认所有列表版本从 v1 开始。
func NewListVersionRegistry() *ListVersionRegistry {
	return &ListVersionRegistry{
		versions: map[ListKind]uint64{
			ListKindUser:        1,
			ListKindUserProject: 1,
			ListKindTask:        1,
			ListKindTaskComment: 1,
		},
	}
}

// Get 获取指定列表当前版本。
func (r *ListVersionRegistry) Get(kind ListKind) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r == nil || r.versions == nil {
		return 1
	}

	version, ok := r.versions[kind]
	if !ok || version == 0 {
		return 1
	}

	return version
}

// Set 直接设置指定列表版本。
func (r *ListVersionRegistry) Set(kind ListKind, version uint64) {
	if r == nil || version == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.versions == nil {
		r.versions = make(map[ListKind]uint64)
	}

	r.versions[kind] = version
}

// Bump 指定列表版本 +1。
// 说明：
// - 适用于该列表缓存结构变更后的迭代。
// - 新版本会自然隔离旧缓存，无需批量删除旧 key。
func (r *ListVersionRegistry) Bump(kind ListKind) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.versions == nil {
		r.versions = make(map[ListKind]uint64)
	}

	current := r.versions[kind]
	if current == 0 {
		current = 1
	}

	current++
	r.versions[kind] = current
	return current
}

// CacheKeyPart 统一缓存 key 片段接口。
// 说明：
//   - 每个 list 的查询条件只要能稳定输出 key 片段即可。
//   - 不要求 query 和 cache 层强耦合。
type CacheKeyPart interface {
	CacheKeyParts() []string
}
