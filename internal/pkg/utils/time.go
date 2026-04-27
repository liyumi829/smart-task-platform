// Package utils 提供了与时间相关的实用函数。
package utils

import "time"

// 前端 ISO8601 → time.Time ISO8601格式解析函数
// ISO8601 格式示例：2024-06-01T12:00:00Z
func ISO2Time(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// Time2ISO 将 time.Time 转为 前端需要的 ISO8601 字符串
// 示例：time.Time → 2024-06-01T12:00:00Z
func Time2ISO(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	// 强制输出 UTC + RFC3339（和前端格式完全对齐）
	return t.UTC().Format(time.RFC3339)
}
