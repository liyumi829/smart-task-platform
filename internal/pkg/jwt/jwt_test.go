package jwt

import (
	"errors"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSecret             = "unit-test-secret"
	testOtherSecret        = "unit-test-other-secret"
	testIssuer             = "smart-task-platform"
	testUserID      uint64 = 1001
	testUsername           = "zhangsan"
	testSessionID          = "sess_001"
	testAccessJTI          = "access_jti_001"
	testRefreshJTI         = "refresh_jti_001"
)

func newTestManager() *Manager {
	return NewManager(
		testSecret,
		testIssuer,
		time.Hour,
		24*time.Hour,
	)
}

func newExpiredTestManager() *Manager {
	return NewManager(
		testSecret,
		testIssuer,
		-time.Second,
		-time.Second,
	)
}

func assertCommonClaims(
	t *testing.T,
	claims *Claims,
	wantUserID uint64,
	wantUsername string,
	wantSessionID string,
	wantJTI string,
	wantTokenType string,
	wantIssuer string,
) {
	t.Helper()

	require.NotNil(t, claims)

	assert.Equal(t, wantUserID, claims.UserID)
	assert.Equal(t, wantUsername, claims.Username)
	assert.Equal(t, wantSessionID, claims.SessionID)
	assert.Equal(t, wantJTI, claims.ID)
	assert.Equal(t, wantTokenType, claims.TokenType)

	assert.Equal(t, wantIssuer, claims.Issuer)
	assert.Equal(t, wantUsername, claims.Subject)
	assert.Equal(t, wantJTI, claims.ID)

	require.NotNil(t, claims.IssuedAt)
	require.NotNil(t, claims.ExpiresAt)
	require.NotNil(t, claims.NotBefore)

	assert.False(t, claims.ExpiresAt.Time.Before(claims.IssuedAt.Time))
	assert.False(t, claims.NotBefore.Time.After(time.Now().Add(time.Second)))
}

func TestManager_NewManager(t *testing.T) {
	manager := NewManager(testSecret, testIssuer, time.Hour, 24*time.Hour)

	require.NotNil(t, manager)
	assert.Equal(t, []byte(testSecret), manager.secret)
	assert.Equal(t, testIssuer, manager.issuer)
	assert.Equal(t, time.Hour, manager.expireAccess)
	assert.Equal(t, 24*time.Hour, manager.expireRefresh)

	t.Logf("[NewManager] issuer=%s access_ttl=%s refresh_ttl=%s",
		manager.issuer,
		manager.expireAccess,
		manager.expireRefresh,
	)
}

func TestManager_AccessTTL_RefreshTTL(t *testing.T) {
	manager := newTestManager()

	assert.Equal(t, time.Hour, manager.AccessTTL())
	assert.Equal(t, 24*time.Hour, manager.RefreshTTL())

	t.Logf("[TTL] access_ttl=%s refresh_ttl=%s", manager.AccessTTL(), manager.RefreshTTL())
}

func TestManager_GenerateAccessToken(t *testing.T) {
	manager := newTestManager()

	tokenString, expiresIn, err := manager.GenerateAccessToken(
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
	)

	t.Logf("[GenerateAccessToken] token=%s expires_in=%d err=%v", tokenString, expiresIn, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)
	assert.Equal(t, int64(3600), expiresIn)

	claims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseAccessToken] claims=%+v err=%v", claims, err)

	require.NoError(t, err)

	assertCommonClaims(
		t,
		claims,
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
		"access",
		testIssuer,
	)

	assert.WithinDuration(t, time.Now().Add(time.Hour), claims.ExpiresAt.Time, 3*time.Second)
}

func TestManager_GenerateRefreshToken(t *testing.T) {
	manager := newTestManager()

	tokenString, expiresIn, err := manager.GenerateRefreshToken(
		testUserID,
		testUsername,
		testSessionID,
		testRefreshJTI,
	)

	t.Logf("[GenerateRefreshToken] token=%s expires_in=%d err=%v", tokenString, expiresIn, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)
	assert.Equal(t, int64(86400), expiresIn)

	claims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseRefreshToken] claims=%+v err=%v", claims, err)

	require.NoError(t, err)

	assertCommonClaims(
		t,
		claims,
		testUserID,
		testUsername,
		testSessionID,
		testRefreshJTI,
		"refresh",
		testIssuer,
	)

	assert.WithinDuration(t, time.Now().Add(24*time.Hour), claims.ExpiresAt.Time, 3*time.Second)
}

func TestManager_GenerateTokenPair(t *testing.T) {
	manager := newTestManager()

	accessToken, refreshToken, expiresIn, err := manager.GenerateTokenPair(
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
		testRefreshJTI,
	)

	t.Logf("[GenerateTokenPair] access_token=%s refresh_token=%s expires_in=%d err=%v",
		accessToken,
		refreshToken,
		expiresIn,
		err,
	)

	require.NoError(t, err)
	require.NotEmpty(t, accessToken)
	require.NotEmpty(t, refreshToken)
	assert.NotEqual(t, accessToken, refreshToken)
	assert.Equal(t, int64(3600), expiresIn)

	accessClaims, err := manager.ParseToken(accessToken)
	t.Logf("[ParseTokenPairAccess] claims=%+v err=%v", accessClaims, err)
	require.NoError(t, err)

	refreshClaims, err := manager.ParseToken(refreshToken)
	t.Logf("[ParseTokenPairRefresh] claims=%+v err=%v", refreshClaims, err)
	require.NoError(t, err)

	assertCommonClaims(
		t,
		accessClaims,
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
		"access",
		testIssuer,
	)

	assertCommonClaims(
		t,
		refreshClaims,
		testUserID,
		testUsername,
		testSessionID,
		testRefreshJTI,
		"refresh",
		testIssuer,
	)

	assert.NotEqual(t, accessClaims.ID, refreshClaims.ID)
	assert.NotEqual(t, accessClaims.ID, refreshClaims.ID)
}

func TestManager_ParseToken_InvalidTokenString(t *testing.T) {
	manager := newTestManager()

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "空字符串",
			token: "",
		},
		{
			name:  "随机字符串",
			token: "invalid_token",
		},
		{
			name:  "只有两段",
			token: "header.payload",
		},
		{
			name:  "三段但内容非法",
			token: "a.b.c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := manager.ParseToken(tt.token)

			t.Logf("[ParseToken InvalidTokenString] name=%s token=%q claims=%+v err=%v",
				tt.name,
				tt.token,
				claims,
				err,
			)

			require.Error(t, err)
			assert.Nil(t, claims)
			assert.ErrorIs(t, err, InvalidTokenError)
		})
	}
}

func TestManager_ParseToken_ExpiredToken(t *testing.T) {
	manager := newExpiredTestManager()

	tokenString, expiresIn, err := manager.GenerateAccessToken(
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
	)

	t.Logf("[GenerateExpiredAccessToken] token=%s expires_in=%d err=%v", tokenString, expiresIn, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)
	assert.Less(t, expiresIn, int64(0))

	claims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseExpiredAccessToken] claims=%+v err=%v", claims, err)

	require.Error(t, err)
	assert.Nil(t, claims)
	assert.ErrorIs(t, err, ExpiredTokenError)
}

func TestManager_ParseToken_WrongSecret(t *testing.T) {
	manager := newTestManager()
	otherManager := NewManager(testOtherSecret, testIssuer, time.Hour, 24*time.Hour)

	tokenString, expiresIn, err := manager.GenerateAccessToken(
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
	)

	t.Logf("[GenerateTokenForWrongSecret] token=%s expires_in=%d err=%v",
		tokenString,
		expiresIn,
		err,
	)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	claims, err := otherManager.ParseToken(tokenString)

	t.Logf("[ParseTokenWrongSecret] claims=%+v err=%v", claims, err)

	require.Error(t, err)
	assert.Nil(t, claims)
	assert.ErrorIs(t, err, InvalidTokenError)
}

func TestManager_ParseToken_InvalidSigningMethod_None(t *testing.T) {
	manager := newTestManager()

	claims := Claims{
		UserID:    testUserID,
		Username:  testUsername,
		SessionID: testSessionID,
		TokenType: "access",
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   testUsername,
			IssuedAt:  jwtv5.NewNumericDate(time.Now()),
			ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwtv5.NewNumericDate(time.Now()),
			ID:        testAccessJTI,
		},
	}

	tokenString, err := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, claims).
		SignedString(jwtv5.UnsafeAllowNoneSignatureType)

	t.Logf("[GenerateNoneAlgToken] token=%s err=%v", tokenString, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	parsedClaims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseNoneAlgToken] claims=%+v err=%v", parsedClaims, err)

	require.Error(t, err)
	assert.Nil(t, parsedClaims)
	assert.ErrorIs(t, err, InvalidSigningMethodError)
}

func TestManager_ParseToken_InvalidSigningMethod_RSA(t *testing.T) {
	manager := newTestManager()

	claims := Claims{
		UserID:    testUserID,
		Username:  testUsername,
		SessionID: testSessionID,
		TokenType: "access",
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   testUsername,
			IssuedAt:  jwtv5.NewNumericDate(time.Now()),
			ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwtv5.NewNumericDate(time.Now()),
			ID:        testAccessJTI,
		},
	}

	// 直接手工构造一个 alg=RS256 的 token 字符串。
	// ParseToken 在 Keyfunc 阶段会先检查签名方法，发现不是 HMAC 后返回 InvalidSigningMethodError。
	tokenString, err := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims).
		SignedString([]byte(testSecret))

	t.Logf("[GenerateRS256Token] token=%s err=%v", tokenString, err)

	require.Error(t, err)
	assert.Empty(t, tokenString)

	// 这里使用一个伪造的 RS256 token，重点测试 ParseToken 对非 HMAC alg 的拒绝逻辑。
	tokenString = strings.Join([]string{
		"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9",
		"eyJ1c2VyX2lkIjoxMDAxLCJ1c2VybmFtZSI6InpoYW5nc2FuIiwic2Vzc2lvbl9pZCI6InNlc3NfMDAxIiwianRpIjoiYWNjZXNzX2p0aV8wMDEiLCJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiaXNzIjoic21hcnQtdGFzay1wbGF0Zm9ybSIsInN1YiI6InpoYW5nc2FuIiwiZXhwIjo0MTAyNDQ0ODAwLCJuYmYiOjE3MDAwMDAwMDAsImlhdCI6MTcwMDAwMDAwMCwianRpIjoiYWNjZXNzX2p0aV8wMDEifQ",
		"fake-signature",
	}, ".")

	parsedClaims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseRS256Token] claims=%+v err=%v", parsedClaims, err)

	require.Error(t, err)
	assert.Nil(t, parsedClaims)
	assert.ErrorIs(t, err, InvalidSigningMethodError)
}

func TestManager_ParseToken_TamperedToken(t *testing.T) {
	manager := newTestManager()

	tokenString, expiresIn, err := manager.GenerateAccessToken(
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
	)

	t.Logf("[GenerateTokenForTamper] token=%s expires_in=%d err=%v",
		tokenString,
		expiresIn,
		err,
	)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	parts := strings.Split(tokenString, ".")
	require.Len(t, parts, 3)

	// 修改 payload，但不重新签名，签名校验应该失败。
	parts[1] = "eyJ1c2VyX2lkIjo5OTk5fQ"
	tamperedToken := strings.Join(parts, ".")

	claims, err := manager.ParseToken(tamperedToken)

	t.Logf("[ParseTamperedToken] token=%s claims=%+v err=%v", tamperedToken, claims, err)

	require.Error(t, err)
	assert.Nil(t, claims)
	assert.ErrorIs(t, err, InvalidTokenError)
}

func TestManager_ParseToken_NotBeforeInFuture(t *testing.T) {
	manager := newTestManager()

	now := time.Now()
	claims := Claims{
		UserID:    testUserID,
		Username:  testUsername,
		SessionID: testSessionID,
		TokenType: "access",
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   testUsername,
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwtv5.NewNumericDate(now.Add(time.Hour)),
			ID:        testAccessJTI,
		},
	}

	tokenString, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).
		SignedString([]byte(testSecret))

	t.Logf("[GenerateFutureNBFToken] token=%s err=%v", tokenString, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	parsedClaims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseFutureNBFToken] claims=%+v err=%v", parsedClaims, err)

	require.Error(t, err)
	assert.Nil(t, parsedClaims)

	// 当前 Manager 只单独转换过期错误，nbf 未生效这类错误统一转换为 InvalidTokenError。
	assert.ErrorIs(t, err, InvalidTokenError)
}

func TestManager_ParseToken_IssuerNotValidated(t *testing.T) {
	manager := newTestManager()

	now := time.Now()
	claims := Claims{
		UserID:    testUserID,
		Username:  testUsername,
		SessionID: testSessionID,
		TokenType: "access",
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    "other-issuer",
			Subject:   testUsername,
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwtv5.NewNumericDate(now),
			ID:        testAccessJTI,
		},
	}

	tokenString, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).
		SignedString([]byte(testSecret))

	t.Logf("[GenerateOtherIssuerToken] token=%s err=%v", tokenString, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	parsedClaims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseOtherIssuerToken] claims=%+v err=%v", parsedClaims, err)

	// 注意：
	// 当前 ParseToken 没有校验 issuer，只校验签名、过期时间、nbf 等 RegisteredClaims 默认规则。
	// 所以 other-issuer 仍然可以解析成功。
	require.NoError(t, err)
	require.NotNil(t, parsedClaims)
	assert.Equal(t, "other-issuer", parsedClaims.Issuer)
}

func TestManager_ParseToken_TokenTypeNotValidated(t *testing.T) {
	manager := newTestManager()

	now := time.Now()
	claims := Claims{
		UserID:    testUserID,
		Username:  testUsername,
		SessionID: testSessionID,
		TokenType: "custom",
		RegisteredClaims: jwtv5.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   testUsername,
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwtv5.NewNumericDate(now),
			ID:        testAccessJTI,
		},
	}

	tokenString, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).
		SignedString([]byte(testSecret))

	t.Logf("[GenerateCustomTokenTypeToken] token=%s err=%v", tokenString, err)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	parsedClaims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseCustomTokenTypeToken] claims=%+v err=%v", parsedClaims, err)

	// 注意：
	// 当前 ParseToken 不校验 token_type，只负责解析和签名校验。
	// access / refresh 类型判断应由业务层完成，或者后续新增 ParseAccessToken / ParseRefreshToken。
	require.NoError(t, err)
	require.NotNil(t, parsedClaims)
	assert.Equal(t, "custom", parsedClaims.TokenType)
}

func TestManager_ParseToken_JTIAndRegisteredIDConsistency(t *testing.T) {
	manager := newTestManager()

	tokenString, expiresIn, err := manager.GenerateAccessToken(
		testUserID,
		testUsername,
		testSessionID,
		testAccessJTI,
	)

	t.Logf("[GenerateTokenForJTIConsistency] token=%s expires_in=%d err=%v",
		tokenString,
		expiresIn,
		err,
	)

	require.NoError(t, err)

	claims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseTokenForJTIConsistency] claims_jti=%s registered_id=%s err=%v",
		claims.ID,
		claims.ID,
		err,
	)

	require.NoError(t, err)
	require.NotNil(t, claims)

	assert.Equal(t, claims.ID, claims.ID)
	assert.Equal(t, testAccessJTI, claims.ID)
}

func TestManager_ParseToken_EmptyBusinessFieldsStillValidJWT(t *testing.T) {
	manager := newTestManager()

	tokenString, expiresIn, err := manager.GenerateAccessToken(
		0,
		"",
		"",
		"",
	)

	t.Logf("[GenerateEmptyBusinessFieldsToken] token=%s expires_in=%d err=%v",
		tokenString,
		expiresIn,
		err,
	)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)

	claims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseEmptyBusinessFieldsToken] claims=%+v err=%v", claims, err)

	// 注意：
	// 当前 Manager 不校验 user_id、username、session_id、jti 是否为空。
	// 如果业务要求这些字段必填，应在 GenerateAccessToken/GenerateRefreshToken 或业务层补充参数校验。
	require.NoError(t, err)
	require.NotNil(t, claims)

	assert.Equal(t, uint64(0), claims.UserID)
	assert.Empty(t, claims.Username)
	assert.Empty(t, claims.SessionID)
	assert.Empty(t, claims.ID)
	assert.Equal(t, "access", claims.TokenType)
}

func TestManager_ParseToken_ExpiredRefreshToken(t *testing.T) {
	manager := newExpiredTestManager()

	tokenString, expiresIn, err := manager.GenerateRefreshToken(
		testUserID,
		testUsername,
		testSessionID,
		testRefreshJTI,
	)

	t.Logf("[GenerateExpiredRefreshToken] token=%s expires_in=%d err=%v",
		tokenString,
		expiresIn,
		err,
	)

	require.NoError(t, err)
	require.NotEmpty(t, tokenString)
	assert.Less(t, expiresIn, int64(0))

	claims, err := manager.ParseToken(tokenString)

	t.Logf("[ParseExpiredRefreshToken] claims=%+v err=%v", claims, err)

	require.Error(t, err)
	assert.Nil(t, claims)
	assert.ErrorIs(t, err, ExpiredTokenError)
}

func TestManager_ParseToken_ErrorIdentity(t *testing.T) {
	manager := newTestManager()

	claims, err := manager.ParseToken("invalid_token")

	t.Logf("[ParseTokenErrorIdentity] claims=%+v err=%v", claims, err)

	require.Error(t, err)
	assert.Nil(t, claims)

	assert.True(t, errors.Is(err, InvalidTokenError))
	assert.False(t, errors.Is(err, ExpiredTokenError))
	assert.False(t, errors.Is(err, InvalidSigningMethodError))
}
