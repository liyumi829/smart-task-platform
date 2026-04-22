// Package bootstrap
// JWT 的配置信息
package bootstrap

import (
	jwtpkg "smart-task-platform/internal/pkg/jwt"
	"time"
)

type JWTConfig struct {
	SecretKey       string `yaml:"secret_key"`        // JWT 密钥
	Issuer          string `yaml:"issuer"`            // JWT 签发者
	AccessTokenTTL  int64  `yaml:"access_token_ttl"`  // 访问令牌过期时间（秒）
	RefreshTokenTTL int64  `yaml:"refresh_token_ttl"` // 刷新令牌过期时间（秒）
}

// setDefault 设置 JWT 配置的默认值
func (c *JWTConfig) setDefault() {
	if c.SecretKey == "" {
		c.SecretKey = "your_secret_key" // 默认密钥，生产环境请务必修改
	}
	if c.Issuer == "" {
		c.Issuer = "smart-task-platform" // 默认签发者
	}
	if c.AccessTokenTTL <= 0 {
		c.AccessTokenTTL = 3600 // 默认访问令牌过期时间 1 小时
	}
	if c.RefreshTokenTTL <= 0 {
		c.RefreshTokenTTL = 7 * 24 * 3600 // 默认刷新令牌过期时间 7 天
	}
}

func InitJWT(c *JWTConfig) *jwtpkg.Manager {
	c.setDefault() // 合并默认值
	return jwtpkg.NewManager(
		c.SecretKey,
		c.Issuer,
		time.Duration(c.AccessTokenTTL)*time.Second,
		time.Duration(c.RefreshTokenTTL)*time.Second,
	)
}
