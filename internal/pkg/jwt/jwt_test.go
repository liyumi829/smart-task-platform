// 测试代码
package jwt

import (
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// newTestManager 创建测试用 JWT 管理器
func newTestManager() *Manager {
	return NewManager(
		"test-secret-key",
		"test-issuer",
		60*time.Minute,
		7*24*time.Hour,
	)
}

// TestNewManager 测试创建 JWT 管理器
func TestNewManager(t *testing.T) {
	t.Parallel()

	secret := "unit-test-secret"
	issuer := "unit-test-issuer"
	accessExpire := 60 * time.Minute
	refreshExpire := 7 * 24 * time.Hour

	manager := NewManager(secret, issuer, accessExpire, refreshExpire)

	t.Logf("manager = %+v", manager)

	if manager == nil {
		t.Fatal("expected manager not nil")
	}
	if string(manager.secret) != secret {
		t.Fatalf("expected secret %q, got %q", secret, string(manager.secret))
	}
	if manager.issuer != issuer {
		t.Fatalf("expected issuer %q, got %q", issuer, manager.issuer)
	}
	if manager.expireAccess != accessExpire {
		t.Fatalf("expected expireAccess %v, got %v", accessExpire, manager.expireAccess)
	}
	if manager.expireRefresh != refreshExpire {
		t.Fatalf("expected expireRefresh %v, got %v", refreshExpire, manager.expireRefresh)
	}
}

// TestManagerGenerateAccessToken 测试生成访问令牌
func TestManagerGenerateAccessToken(t *testing.T) {
	t.Parallel()

	manager := newTestManager()
	userID := uint64(1001)
	username := "zhangsan"

	tokenString, expiresIn, err := manager.generateAccessToken(userID, username)
	if err != nil {
		t.Fatalf("generateAccessToken returned error: %v", err)
	}

	t.Logf("access token = %s", tokenString)
	t.Logf("access expiresIn = %d", expiresIn)

	if tokenString == "" {
		t.Fatal("expected access token not empty")
	}
	if expiresIn != int64(manager.expireAccess.Seconds()) {
		t.Fatalf("expected expiresIn %d, got %d", int64(manager.expireAccess.Seconds()), expiresIn)
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}

	t.Logf("access claims = %+v", claims)

	if claims == nil {
		t.Fatal("expected claims not nil")
	}
	if claims.UserID != userID {
		t.Fatalf("expected userID %d, got %d", userID, claims.UserID)
	}
	if claims.Username != username {
		t.Fatalf("expected username %q, got %q", username, claims.Username)
	}
	if claims.Issuer != manager.issuer {
		t.Fatalf("expected issuer %q, got %q", manager.issuer, claims.Issuer)
	}
	if claims.Subject != username {
		t.Fatalf("expected subject %q, got %q", username, claims.Subject)
	}
	if claims.IssuedAt == nil {
		t.Fatal("expected IssuedAt not nil")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt not nil")
	}
	if claims.NotBefore == nil {
		t.Fatal("expected NotBefore not nil")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt.Time) {
		t.Fatalf("expected ExpiresAt > IssuedAt, got ExpiresAt=%v IssuedAt=%v", claims.ExpiresAt.Time, claims.IssuedAt.Time)
	}
	if claims.NotBefore.Time.After(claims.ExpiresAt.Time) {
		t.Fatalf("expected NotBefore <= ExpiresAt, got NotBefore=%v ExpiresAt=%v", claims.NotBefore.Time, claims.ExpiresAt.Time)
	}
}

// TestManagerGenerateRefreshToken 测试生成刷新令牌
func TestManagerGenerateRefreshToken(t *testing.T) {
	t.Parallel()

	manager := newTestManager()
	userID := uint64(2002)
	username := "lisi"

	tokenString, expiresIn, err := manager.generateRefreshToken(userID, username)
	if err != nil {
		t.Fatalf("generateRefreshToken returned error: %v", err)
	}

	t.Logf("refresh token = %s", tokenString)
	t.Logf("refresh expiresIn = %d", expiresIn)

	if tokenString == "" {
		t.Fatal("expected refresh token not empty")
	}
	if expiresIn != int64(manager.expireRefresh.Seconds()) {
		t.Fatalf("expected expiresIn %d, got %d", int64(manager.expireRefresh.Seconds()), expiresIn)
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}

	t.Logf("refresh claims = %+v", claims)

	if claims == nil {
		t.Fatal("expected claims not nil")
	}
	if claims.UserID != userID {
		t.Fatalf("expected userID %d, got %d", userID, claims.UserID)
	}
	if claims.Username != username {
		t.Fatalf("expected username %q, got %q", username, claims.Username)
	}
	if claims.Issuer != manager.issuer {
		t.Fatalf("expected issuer %q, got %q", manager.issuer, claims.Issuer)
	}
	if claims.Subject != username {
		t.Fatalf("expected subject %q, got %q", username, claims.Subject)
	}
	if claims.IssuedAt == nil {
		t.Fatal("expected IssuedAt not nil")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt not nil")
	}
	if claims.NotBefore == nil {
		t.Fatal("expected NotBefore not nil")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt.Time) {
		t.Fatalf("expected ExpiresAt > IssuedAt, got ExpiresAt=%v IssuedAt=%v", claims.ExpiresAt.Time, claims.IssuedAt.Time)
	}
	if claims.NotBefore.Time.After(claims.ExpiresAt.Time) {
		t.Fatalf("expected NotBefore <= ExpiresAt, got NotBefore=%v ExpiresAt=%v", claims.NotBefore.Time, claims.ExpiresAt.Time)
	}
}

// TestManagerGenerateToken 测试同时生成访问令牌和刷新令牌
func TestManagerGenerateToken(t *testing.T) {
	t.Parallel()

	manager := newTestManager()
	userID := uint64(3003)
	username := "wangwu"

	accessToken, refreshToken, expiresIn, err := manager.GenerateToken(userID, username)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	t.Logf("access token = %s", accessToken)
	t.Logf("refresh token = %s", refreshToken)
	t.Logf("expiresIn = %d", expiresIn)

	if accessToken == "" {
		t.Fatal("expected access token not empty")
	}
	if refreshToken == "" {
		t.Fatal("expected refresh token not empty")
	}
	if expiresIn != int64(manager.expireAccess.Seconds()) {
		t.Fatalf("expected expiresIn %d, got %d", int64(manager.expireAccess.Seconds()), expiresIn)
	}

	accessClaims, err := manager.ParseToken(accessToken)
	if err != nil {
		t.Fatalf("ParseToken access returned error: %v", err)
	}
	refreshClaims, err := manager.ParseToken(refreshToken)
	if err != nil {
		t.Fatalf("ParseToken refresh returned error: %v", err)
	}

	t.Logf("access claims = %+v", accessClaims)
	t.Logf("refresh claims = %+v", refreshClaims)

	// 断言 access token claims
	if accessClaims.UserID != userID {
		t.Fatalf("expected access userID %d, got %d", userID, accessClaims.UserID)
	}
	if accessClaims.Username != username {
		t.Fatalf("expected access username %q, got %q", username, accessClaims.Username)
	}
	if accessClaims.Issuer != manager.issuer {
		t.Fatalf("expected access issuer %q, got %q", manager.issuer, accessClaims.Issuer)
	}
	if accessClaims.Subject != username {
		t.Fatalf("expected access subject %q, got %q", username, accessClaims.Subject)
	}
	if accessClaims.IssuedAt == nil || accessClaims.ExpiresAt == nil || accessClaims.NotBefore == nil {
		t.Fatal("expected access token registered claims not nil")
	}

	// 断言 refresh token claims
	if refreshClaims.UserID != userID {
		t.Fatalf("expected refresh userID %d, got %d", userID, refreshClaims.UserID)
	}
	if refreshClaims.Username != username {
		t.Fatalf("expected refresh username %q, got %q", username, refreshClaims.Username)
	}
	if refreshClaims.Issuer != manager.issuer {
		t.Fatalf("expected refresh issuer %q, got %q", manager.issuer, refreshClaims.Issuer)
	}
	if refreshClaims.Subject != username {
		t.Fatalf("expected refresh subject %q, got %q", username, refreshClaims.Subject)
	}
	if refreshClaims.IssuedAt == nil || refreshClaims.ExpiresAt == nil || refreshClaims.NotBefore == nil {
		t.Fatal("expected refresh token registered claims not nil")
	}

	// 刷新令牌应比访问令牌过期更晚
	if !refreshClaims.ExpiresAt.After(accessClaims.ExpiresAt.Time) {
		t.Fatalf(
			"expected refresh token expire later than access token, access=%v refresh=%v",
			accessClaims.ExpiresAt.Time,
			refreshClaims.ExpiresAt.Time,
		)
	}
}

// TestManagerParseToken_Success 测试成功解析令牌
func TestManagerParseToken_Success(t *testing.T) {
	t.Parallel()

	manager := newTestManager()
	userID := uint64(4004)
	username := "zhaoliu"

	tokenString, _, err := manager.generateAccessToken(userID, username)
	if err != nil {
		t.Fatalf("generateAccessToken returned error: %v", err)
	}

	claims, err := manager.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}

	t.Logf("parsed claims = %+v", claims)

	if claims == nil {
		t.Fatal("expected claims not nil")
	}
	if claims.UserID != userID {
		t.Fatalf("expected userID %d, got %d", userID, claims.UserID)
	}
	if claims.Username != username {
		t.Fatalf("expected username %q, got %q", username, claims.Username)
	}
	if claims.Issuer != manager.issuer {
		t.Fatalf("expected issuer %q, got %q", manager.issuer, claims.Issuer)
	}
	if claims.Subject != username {
		t.Fatalf("expected subject %q, got %q", username, claims.Subject)
	}
	if claims.IssuedAt == nil {
		t.Fatal("expected IssuedAt not nil")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt not nil")
	}
	if claims.NotBefore == nil {
		t.Fatal("expected NotBefore not nil")
	}
}

// TestManagerParseToken_Expired 测试解析过期令牌
func TestManagerParseToken_Expired(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"expired-secret",
		"expired-issuer",
		-1*time.Minute, // 设置为已过期
		7*24*time.Hour,
	)

	tokenString, _, err := manager.generateAccessToken(5005, "expired-user")
	if err != nil {
		t.Fatalf("generateAccessToken returned error: %v", err)
	}

	t.Logf("expired token = %s", tokenString)

	claims, err := manager.ParseToken(tokenString)
	t.Logf("expired parse claims = %+v", claims)
	t.Logf("expired parse err = %v", err)

	if err == nil {
		t.Fatal("expected expired token error, got nil")
	}
	if !errors.Is(err, ExpiredTokenError) {
		t.Fatalf("expected error %v, got %v", ExpiredTokenError, err)
	}
	if claims != nil {
		t.Fatalf("expected claims nil, got %+v", claims)
	}
}

// TestManagerParseToken_InvalidToken 测试解析非法令牌
func TestManagerParseToken_InvalidToken(t *testing.T) {
	t.Parallel()

	manager := newTestManager()
	invalidToken := "this.is.not.a.valid.jwt"

	claims, err := manager.ParseToken(invalidToken)
	t.Logf("invalid parse claims = %+v", claims)
	t.Logf("invalid parse err = %v", err)

	if err == nil {
		t.Fatal("expected invalid token error, got nil")
	}
	if !errors.Is(err, InvalidTokenError) {
		t.Fatalf("expected error %v, got %v", InvalidTokenError, err)
	}
	if claims != nil {
		t.Fatalf("expected claims nil, got %+v", claims)
	}
}

// TestManagerParseToken_InvalidSigningMethod 测试解析非法签名方法令牌
func TestManagerParseToken_InvalidSigningMethod(t *testing.T) {
	t.Parallel()

	manager := newTestManager()

	claims := Claims{
		UserID:   6006,
		Username: "invalid-sign-method-user",
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    manager.issuer,
			Subject:   "invalid-sign-method-user",
			IssuedAt:  jwtv5.NewNumericDate(time.Now()),
			ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(10 * time.Minute)),
			NotBefore: jwtv5.NewNumericDate(time.Now()),
		},
	}

	// 使用 none 签名算法构造非法签名方法令牌
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwtv5.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString returned error: %v", err)
	}

	t.Logf("invalid signing method token = %s", tokenString)

	parsedClaims, parseErr := manager.ParseToken(tokenString)
	t.Logf("invalid signing parse claims = %+v", parsedClaims)
	t.Logf("invalid signing parse err = %v", parseErr)

	if parseErr == nil {
		t.Fatal("expected invalid signing method error, got nil")
	}
	if !errors.Is(parseErr, InvalidSigningMethodError) {
		t.Fatalf("expected error %v, got %v", InvalidSigningMethodError, parseErr)
	}
	if parsedClaims != nil {
		t.Fatalf("expected claims nil, got %+v", parsedClaims)
	}
}
