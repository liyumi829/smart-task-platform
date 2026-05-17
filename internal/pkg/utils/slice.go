// internal/pkg/utils/slice.go
// 关于切片的工具
package utils

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
)

// RemoveTarget 从切片中删除第一个匹配的目标值（泛型，支持所有可比较类型）
func RemoveTarget[T comparable](slice []T, target T) []T {
	for i, v := range slice {
		if v == target {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// Deduplicate 对切片进行去重，保留元素第一次出现的顺序，支持所有可比较类型
func Deduplicate[T comparable](slice []T) []T {
	seen := make(map[T]struct{}, len(slice)) // 空结构体不占用内存
	res := make([]T, 0, len(slice))          // 存储最终结果，预分配容量提升性能
	for _, val := range slice {
		// 如果map中不存在该元素，说明是第一次出现
		if _, ok := seen[val]; !ok {
			seen[val] = struct{}{} // 标记为已存在
			res = append(res, val) // 加入结果切片
		}
	}

	return res
}

// DeduplicateAny 通用切片去重
//   - T：任意类型（不需要 comparable）
//   - equal：自定义相等判断函数，返回 true 表示两个元素相等
func DeduplicateAny[T any](slice []T, equal func(a, b T) bool) []T {
	// 结果切片，预分配容量提升性能
	res := make([]T, 0, len(slice))

	// 遍历原切片
	for _, val := range slice {
		// 检查当前元素是否已经在结果里存在
		exists := false
		for _, item := range res {
			if equal(item, val) {
				exists = true
				break
			}
		}
		// 不存在则加入结果
		if !exists {
			res = append(res, val)
		}
	}

	return res
}

// Partition 把切片按照 fn 规则分为两个部分
//   - left 是判断为 true
//   - right 是判断为 false
func Partition[T any](slice []T, fn func(T) bool) ([]T, []T) {
	left := make([]T, 0, len(slice))
	right := make([]T, 0, len(slice))
	for _, v := range slice {
		if ok := fn(v); ok {
			left = append(left, v)
		} else {
			right = append(right, v)
		}
	}
	return left, right
}

// SliceToMap 将切片转成 map，key 由自定义函数 fn 生成
// T：map 的 key 类型（必须可比较）
// V：切片元素 & map value 类型
func SliceToMap[T comparable, V any](slice []V, fn func(v V) T) map[T]V {
	res := make(map[T]V, len(slice)) // 预分配容量，性能最优
	for _, val := range slice {
		key := fn(val)
		// 只在不存在时设置，保留第一个
		if _, exist := res[key]; !exist {
			res[key] = val
		}
	}
	return res
}

// SliceToUniqueCacheKey 生成唯一、有序、短长度的缓存 Key（生产推荐）
func SliceToUniqueCacheKey(slice any) string {
	buf := bytes.Buffer{}

	switch v := slice.(type) {
	case []string:
		// 排序，保证顺序不同但内容相同生成同一个key
		sort.Strings(v)
		for i, s := range v {
			if i > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(s)
		}
	case []int:
		sort.Ints(v)
		for i, num := range v {
			if i > 0 {
				buf.WriteString(",")
			}
			fmt.Fprint(&buf, num)
		}
	case []int64:
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		for i, num := range v {
			if i > 0 {
				buf.WriteString(",")
			}
			fmt.Fprint(&buf, num)
		}
	default:
		return ""
	}

	// MD5 压缩，让key更短（适合Redis等缓存）
	hash := md5.Sum(buf.Bytes())
	return hex.EncodeToString(hash[:])
}
