package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client 请求客户端信息，从 Header 提取，供 controller 从 ctx 获取
type Client struct {
	OS              int        // X-Client-OS：1.iOS；2.安卓；3.鸿蒙；0.未知
	IP              []byte     // 客户端 IP，varbinary(16)，IPv4 映射为 16 字节
	ReqTime         time.Time  // 服务端请求接收时间
	ClientTimestamp *time.Time // 客户端请求时间戳，来自 X-Timestamp（unix 秒），未提供或解析失败为 nil
}

type contextKey struct{}

var clientKey = &contextKey{}

// ClientMiddleware 从请求 Header 提取客户端信息，注入 ctx
func ClientMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := Client{
			OS:              parseIntHeader(r.Header.Get("X-Client-OS"), 0),
			IP:              parseClientIP(r),
			ReqTime:         time.Now(),
			ClientTimestamp: parseClientTimestamp(r.Header.Get("X-Timestamp")),
		}
		ctx := context.WithValue(r.Context(), clientKey, &client)
		next(w, r.WithContext(ctx))
	}
}

// GetClientFromContext 从 Context 获取 Client，不存在时返回 nil, false
func GetClientFromContext(ctx context.Context) (*Client, bool) {
	c, ok := ctx.Value(clientKey).(*Client)
	return c, ok
}

// MustGetClient 从 Context 获取 Client，不存在时返回零值（便于 controller 安全使用）
func MustGetClient(ctx context.Context) *Client {
	c, _ := GetClientFromContext(ctx)
	return c
}

func parseClientTimestamp(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil || sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0)
	return &t
}

func parseIntHeader(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}

func parseClientIP(r *http.Request) []byte {
	raw := r.Header.Get("X-Forwarded-For")
	if raw != "" {
		if idx := strings.Index(raw, ","); idx > 0 {
			raw = strings.TrimSpace(raw[:idx])
		} else {
			raw = strings.TrimSpace(raw)
		}
	}
	if raw == "" {
		raw = r.Header.Get("X-Real-IP")
	}
	if raw == "" {
		raw, _, _ = net.SplitHostPort(r.RemoteAddr)
		if raw == "" {
			raw = r.RemoteAddr
		}
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return nil
	}
	return ip.To16()
}
