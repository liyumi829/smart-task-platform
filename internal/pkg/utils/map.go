// internal/pkg/utils/map.go
// Package utils
// map 使用的工具
package utils

// 把一个 map 的 value 批量转换成另一种类型，key 保持不变，返回新 map。
func MapToOtherMap[C comparable, T, V any](m map[C]T, fn func(val T) V) map[C]V {
	res := make(map[C]V, len(m))
	for key, val := range m {
		v := fn(val)
		res[key] = v
	}
	return res
}
