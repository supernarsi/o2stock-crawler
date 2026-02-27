package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"o2stock-crawler/internal/db"
	"strconv"
	"testing"
	"time"
)

func TestSignatureMiddleware(t *testing.T) {
	secret := "test-secret"
	cfg := &db.Config{
		EnableSignature: true,
		SignatureSecret: secret,
	}

	handler := SignatureMiddleware(cfg)(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	t.Run("Valid Signature", func(t *testing.T) {
		nonce := "1234567890123456"
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		body := []byte(`{"foo":"bar"}`)

		req := httptest.NewRequest("POST", "/test?q=1", bytes.NewBuffer(body))

		bodyHash := sha256.Sum256(body)
		bodyDigest := hex.EncodeToString(bodyHash[:])
		raw := fmt.Sprintf("%s%s%s%s%s", "POST", "/test?q=1", ts, nonce, bodyDigest)

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(raw))
		sig := hex.EncodeToString(mac.Sum(nil))

		req.Header.Set("x-signature", sig)
		req.Header.Set("x-timestamp", ts)
		req.Header.Set("x-nonce", nonce)

		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}
	})

	t.Run("Invalid Signature", func(t *testing.T) {
		nonce := "1234567890123456"
		ts := strconv.FormatInt(time.Now().Unix(), 10)

		req := httptest.NewRequest("GET", "/test", nil)

		req.Header.Set("x-signature", "invalid")
		req.Header.Set("x-timestamp", ts)
		req.Header.Set("x-nonce", nonce)

		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Expired Timestamp", func(t *testing.T) {
		nonce := "1234567890123457"
		ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("x-signature", "dummy")
		req.Header.Set("x-timestamp", ts)
		req.Header.Set("x-nonce", nonce)

		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Debug Bypass", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("x-debug", "42")

		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}
	})

	t.Run("Nonce Replay", func(t *testing.T) {
		nonce := "1234567890123458"
		ts := strconv.FormatInt(time.Now().Unix(), 10)

		// 1st Request (Valid)
		req1 := httptest.NewRequest("GET", "/test", nil)

		bodyHash := sha256.Sum256(nil)
		bodyDigest := hex.EncodeToString(bodyHash[:])
		raw := fmt.Sprintf("%s%s%s%s%s", "GET", "/test", ts, nonce, bodyDigest)

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(raw))
		sig := hex.EncodeToString(mac.Sum(nil))

		req1.Header.Set("x-signature", sig)
		req1.Header.Set("x-timestamp", ts)
		req1.Header.Set("x-nonce", nonce)

		w1 := httptest.NewRecorder()
		handler(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("First request failed: %d", w1.Code)
		}

		// 2nd Request (Replay)
		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.Header.Set("x-signature", sig)
		req2.Header.Set("x-timestamp", ts)
		req2.Header.Set("x-nonce", nonce)

		w2 := httptest.NewRecorder()
		handler(w2, req2)
		if w2.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized for replay, got %d", w2.Code)
		}
	})
}
