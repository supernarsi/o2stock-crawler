package service

import (
	"o2stock-crawler/internal/db"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestToken(t *testing.T) {
	cfg := &db.Config{
		JWTSecret: "test-secret",
	}
	svc := &AuthService{
		dbConfig: cfg,
	}

	userID := uint(123)
	token, err := svc.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned empty token")
	}
	t.Logf("Token: %s", token)

	parsedID, err := svc.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if parsedID != userID {
		t.Errorf("expected user id %d, got %d", userID, parsedID)
	}

	// Test invalid signature
	invalidToken := token + "invalid"
	_, err = svc.VerifyToken(invalidToken)
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestRefreshToken(t *testing.T) {
	cfg := &db.Config{
		JWTSecret: "test-secret",
	}
	svc := &AuthService{
		dbConfig: cfg,
	}

	userID := uint(123)
	token, err := svc.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// 1. 测试未过期的 Token 刷新
	newToken, newUserID, err := svc.RefreshToken(token)
	if err != nil {
		t.Fatalf("RefreshToken failed for valid token: %v", err)
	}
	if newUserID != userID {
		t.Errorf("expected user id %d, got %d", userID, newUserID)
	}
	// Note: tokens might be identical if generated in the same second due to IssuedAt/ExpiresAt precision

	// Verify the new token
	if _, err := svc.VerifyToken(newToken); err != nil {
		t.Fatalf("VerifyToken failed for new token: %v", err)
	}

	// 2. 测试已过期但在宽限期内的 Token 刷新
	expiredClaims := UserClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // 已过期 1 小时
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-8 * 24 * time.Hour)),
			Issuer:    "o2stock-api",
		},
	}
	expiredTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredToken, _ := expiredTokenObj.SignedString([]byte(cfg.JWTSecret))

	// VerifyToken 应该失败
	_, err = svc.VerifyToken(expiredToken)
	if err == nil {
		t.Error("expected VerifyToken to fail for expired token")
	}

	// RefreshToken 应该成功 (因为在宽限期内)
	refreshedToken, refreshedUserID, err := svc.RefreshToken(expiredToken)
	if err != nil {
		t.Fatalf("RefreshToken failed for expired (but in grace period) token: %v", err)
	}
	if refreshedUserID != userID {
		t.Errorf("expected user id %d, got %d", userID, refreshedUserID)
	}
	if _, err := svc.VerifyToken(refreshedToken); err != nil {
		t.Fatalf("VerifyToken failed for refreshed token: %v", err)
	}
}

func TestIsTokenNearExpiry(t *testing.T) {
	cfg := &db.Config{
		JWTSecret: "test-secret",
	}
	svc := &AuthService{
		dbConfig: cfg,
	}

	userID := uint(123)
	token, err := svc.GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// 正常 Token 不应该接近过期 (7天有效期，2天阈值)
	if svc.IsTokenNearExpiry(token) {
		t.Error("token should not be near expiry")
	}

	// 测试快过期的 Token
	nearExpiryClaims := UserClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * 24 * time.Hour)), // 剩 1 天
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-6 * 24 * time.Hour)),
			Issuer:    "o2stock-api",
		},
	}
	nearExpiryTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, nearExpiryClaims)
	nearExpiryToken, _ := nearExpiryTokenObj.SignedString([]byte(cfg.JWTSecret))

	if !svc.IsTokenNearExpiry(nearExpiryToken) {
		t.Error("token should be near expiry (1 day left < 2 days threshold)")
	}
}
