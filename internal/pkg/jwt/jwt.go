// Package jwt 提供 JWT 相关的功能，包括生成和验证 JWT Token。
package jwt

import (
	"errors"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

var (
	ExpiredTokenError         = errors.New("token expired")          // 过期的 Token 错误消息
	InvalidTokenError         = errors.New("invalid token")          // 无效的 Token 错误消息
	InvalidSigningMethodError = errors.New("invalid signing method") // 无效的签名方法错误消息
)

// Claims 自定义 JWT Claims
type Claims struct {
	// 用户ID
	UserID uint64 `json:"user_id"`

	// 用户名
	Username string `json:"username"`

	// 内嵌标准注册声明
	jwtv5.RegisteredClaims
}

// Manager JWT 管理器
type Manager struct {
	secret        []byte        // 密钥
	issuer        string        // 签发者
	expireAccess  time.Duration // 访问令牌过期时间
	expireRefresh time.Duration // 刷新令牌过期时间
}

// NewManager 创建 JWT 管理器
func NewManager(secret, issuer string, expireAccess time.Duration, expireRefresh time.Duration) *Manager {
	return &Manager{
		secret:        []byte(secret),
		issuer:        issuer,
		expireAccess:  expireAccess,
		expireRefresh: expireRefresh,
	}
}

// generateAccessToken 生成访问 Token
func (m *Manager) generateAccessToken(userID uint64, username string) (string, int64, error) {
	now := time.Now()                // 获取现在的时间
	expAt := now.Add(m.expireAccess) // 过期时间点

	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(expAt),
			NotBefore: jwtv5.NewNumericDate(now),
		},
	} // 构造 Claims 对象，设置用户 ID、用户名和标准注册声明

	// 创建新的 JWT Token，使用 HS256 签名方法和自定义 Claims
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret) // 获取数据签名字符串
	if err != nil {
		return "", 0, err
	}

	return signed, int64(m.expireAccess.Seconds()), nil
}

// generateRefreshToken 生成刷新 Token
func (m *Manager) generateRefreshToken(userID uint64, username string) (string, int64, error) {
	now := time.Now()                 // 获取现在的时间
	expAt := now.Add(m.expireRefresh) // 过期时间点

	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(expAt),
			NotBefore: jwtv5.NewNumericDate(now),
		},
	} // 构造 Claims 对象，设置用户 ID、用户名和标准注册声明

	// 创建新的 JWT Token，使用 HS256 签名方法和自定义 Claims
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret) // 获取数据签名字符串
	if err != nil {
		return "", 0, err
	}

	return signed, int64(m.expireRefresh.Seconds()), nil
}

// GenerateToken 同时生成访问 Token 和刷新 Token
func (m *Manager) GenerateToken(userID uint64, username string) (string, string, int64, error) {
	accessToken, expiresIn, err := m.generateAccessToken(userID, username) // 生成访问 Token
	if err != nil {
		return "", "", 0, err
	}
	refreshToken, _, err := m.generateRefreshToken(userID, username) // 生成刷新 Token
	if err != nil {
		return "", "", 0, err
	}
	return accessToken, refreshToken, expiresIn, nil
}

// ParseToken 解析 Token
func (m *Manager) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwtv5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwtv5.Token) (any, error) {
			// 验证签名方法是否为 HMAC，防止攻击者使用不同的签名方法绕过验证
			if _, ok := token.Method.(*jwtv5.SigningMethodHMAC); !ok {
				return nil, InvalidSigningMethodError
			}
			return m.secret, nil // 返回密钥用于验证签名
		})
	if err != nil {
		// 解析失败区别对待过期错误和其他错误，提供更具体的错误信息
		if errors.Is(err, jwtv5.ErrTokenExpired) {
			return nil, ExpiredTokenError
		}
		// 其他错误
		return nil, InvalidTokenError
	}
	// 将解析后的 Claims 转换为自定义的 Claims 结构体，并验证 Token 是否有效
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, InvalidTokenError
	}

	return claims, nil
}
