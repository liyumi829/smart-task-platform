// Package utils 生成一个 uuid
package utils

import "github.com/google/uuid"

func Uuid() string {
	return uuid.NewString()
}
