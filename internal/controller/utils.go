package controller

import (
	"strconv"
	"time"
)

// parseIntDefault 解析字符串为整数，失败时返回默认值
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// formatTimeOrEmpty 格式化时间为字符串，如果为 nil 则返回空字符串
func formatTimeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// splitCommaSeparated 按逗号分割字符串
func splitCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	start := 0
	for i, char := range s {
		if char == ',' {
			if start < i {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}
