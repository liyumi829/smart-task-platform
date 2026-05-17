// internal/pkg/codec/error.go
// Package codec
// 定义解析的错误

package codec

import "errors"

var (
	ErrJsonUnpxpectedTralingData = errors.New("json: unexpected trailing data")
)
