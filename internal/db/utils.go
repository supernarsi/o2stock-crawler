package db

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// SQL 构建辅助函数
// ============================================================================

// buildINClause 构建 SQL IN 子句的占位符和参数
// 返回占位符字符串（如 "?,?,?"）和参数列表
func buildINClause(values []any) (string, []any) {
	if len(values) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ","), values
}

// buildINClauseWithPrefix 构建带前缀的 SQL IN 子句（用于 WHERE 条件）
// 例如：buildINClauseWithPrefix("player_id", []uint{1,2,3}) 返回 "AND player_id IN (?,?,?)"
func buildINClauseWithPrefix(column string, values []any, args *[]any) string {
	if len(values) == 0 {
		return ""
	}
	placeholders := make([]string, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		*args = append(*args, v)
	}
	return fmt.Sprintf("AND %s IN (%s)", column, strings.Join(placeholders, ","))
}

// convertUintToAny 将 []uint 转换为 []any
func convertUintToAny(values []uint) []any {
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}

// ============================================================================
// 时间处理辅助函数
// ============================================================================

// FormatDateTimeHour 格式化时间为数据库日期时间格式（200601021504）
func FormatDateTimeHour(t time.Time) string {
	return t.Format("200601021504")
}

// ============================================================================
// 数据提取辅助函数
// ============================================================================

// extractIDs 从任意切片中提取 ID（需要类型断言）
func extractIDs[T any](items []T, getID func(T) uint) []uint {
	ids := make([]uint, len(items))
	for i, item := range items {
		ids[i] = getID(item)
	}
	return ids
}
