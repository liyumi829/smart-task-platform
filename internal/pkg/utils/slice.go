// internal/pkg/utils/slice.go
// 关于切片的工具
package utils

// RemoveTarget 从切片中删除第一个匹配的目标值（泛型，支持所有可比较类型）
func RemoveTarget[T comparable](slice []T, target T) []T {
	for i, v := range slice {
		if v == target {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
