// internal/pkg/utils/ptr.go
// 指针工具

package utils

import "time"

// SafeStringValue 安全获取 *string 的值
//   - nil  → 返回空字符串 ""
//   - 非 nil → 返回解引用后的字符串
func SafeStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// StringPtr 返回字符串指针
func StringPtr(v string) *string {
	return &v
}

// Int64Ptr 返回Int64指针
func Int64Ptr(v int64) *int64 {
	return &v
}

// TimePtr 返回时间指针
func TimePtr(v time.Time) *time.Time {
	return &v
}

// SafeValue 安全获取指针的值，指针为 nil 时返回对应类型零值
func SafeValue[T any](val *T) T {
	if val == nil {
		var zero T
		return zero
	}
	return *val
}

// SafePtrClone 安全克隆指针的值，如果nil，则返回nil
func SafePtrClone[T any](val *T) *T {
	if val == nil {
		return nil
	}
	v := *val
	return &v
}
