package service

import (
	"o2stock-crawler/internal/db"
	"testing"
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
