package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"strconv"
	"sync"
	"time"
)

// NonceCache prevents replay attacks by storing recent nonces
type NonceCache struct {
	cache sync.Map
}

func (c *NonceCache) Add(nonce string) {
	c.cache.Store(nonce, time.Now())
}

func (c *NonceCache) Exists(nonce string) bool {
	_, ok := c.cache.Load(nonce)
	return ok
}

// Cleanup removes expired nonces
func (c *NonceCache) Cleanup(expiration time.Duration) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.cache.Range(func(key, value any) bool {
			if t, ok := value.(time.Time); ok {
				if now.Sub(t) > expiration {
					c.cache.Delete(key)
				}
			}
			return true
		})
	}
}

var (
	nonceCache = &NonceCache{}
	once       sync.Once
)

func init() {
	go nonceCache.Cleanup(10 * time.Minute) // Cleanup nonces older than 10 minutes (timestamp check is +/- 5 min)
}

// SignatureMiddleware creates a new signature verification middleware
func SignatureMiddleware(cfg *db.Config) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !cfg.EnableSignature {
				next(w, r)
				return
			}

			// Debug bypass
			if r.Header.Get("x-debug") == "42" {
				log.Println("[DEBUG] Signature verification skipped due to xdebug header")
				next(w, r)
				return
			}

			// 1. Check headers
			sig := r.Header.Get("x-signature")
			tsStr := r.Header.Get("x-timestamp")
			nonce := r.Header.Get("x-nonce")

			if sig == "" || tsStr == "" || nonce == "" {
				log.Println("[Signature] Missing headers")
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, api.Error(http.StatusUnauthorized, "Missing signature headers"))
				return
			}

			// 2. Validate timestamp
			ts, err := strconv.ParseInt(tsStr, 10, 64)
			if err != nil {
				log.Printf("[Signature] Invalid timestamp: %s", tsStr)
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, api.Error(http.StatusUnauthorized, "Invalid timestamp"))
				return
			}
			now := time.Now().Unix()
			if now-ts > 300 || ts-now > 300 { // +/- 5 minutes
				log.Printf("[Signature] Timestamp expired: %d (now: %d)", ts, now)
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, api.Error(http.StatusUnauthorized, "Timestamp expired"))
				return
			}

			// 3. Validate nonce
			if len(nonce) < 16 {
				log.Printf("[Signature] Nonce too short: %s", nonce)
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, api.Error(http.StatusUnauthorized, "Nonce too short"))
				return
			}
			if nonceCache.Exists(nonce) {
				log.Printf("[Signature] Nonce reused: %s", nonce)
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, api.Error(http.StatusUnauthorized, "Nonce reused"))
				return
			}
			nonceCache.Add(nonce)

			// 4. Read body
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, err = io.ReadAll(r.Body)
				if err != nil {
					log.Printf("[Signature] Failed to read body: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					writeJSON(w, api.Error(http.StatusInternalServerError, "Failed to read body"))
					return
				}
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body
			}

			// 5. Calculate digest
			// Content: Method + Path + Timestamp + Nonce + Body Digest
			bodyHash := sha256.Sum256(bodyBytes)
			bodyDigest := hex.EncodeToString(bodyHash[:])

			raw := fmt.Sprintf("%s%s%s%s%s", r.Method, r.URL.RequestURI(), tsStr, nonce, bodyDigest)

			// 6. Calculate HMAC
			mac := hmac.New(sha256.New, []byte(cfg.SignatureSecret))
			mac.Write([]byte(raw))
			expectedSig := hex.EncodeToString(mac.Sum(nil))

			if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
				log.Printf("[Signature] Verification failed. Raw: %s, Expected: %s, Got: %s", raw, expectedSig, sig)
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, api.Error(http.StatusUnauthorized, "Invalid signature"))
				return
			}

			next(w, r)
		}
	}
}
