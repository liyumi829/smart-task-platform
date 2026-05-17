// Package utils 生成一个 uuid
package utils

import (
	"github.com/google/uuid"
)

func Uuid() string {
	return uuid.NewString()
}

// NewSessionID 生成新的 session_id
func NewSessionID() string {
	return "sess_" + Uuid()
}

// NewAccessJTI 生成 access token jti
func NewAccessJTI() string {
	return "ajti_" + Uuid()
}

// NewRefreshJTI 生成 refresh token jti
func NewRefreshJTI() string {
	return "rjti_" + Uuid()
}
