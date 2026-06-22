// internal/pkg/utils/sleep.go
// package utils
// 提供休眠功能的工具
package utils

import (
	"context"
	"time"
)

// SleepWithContext 支持 context 取消的 sleep
func SleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
