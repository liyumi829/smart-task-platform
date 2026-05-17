// internal/service/cachesvc/cacheobj/common_cache_object.go
// Package cacheobj
// 共用的缓存对象

package cacheobj

type ListPage struct {
	List     []uint64 `json:"list"`
	Total    *int64   `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	HasMore  bool     `json:"has_more"`
}
