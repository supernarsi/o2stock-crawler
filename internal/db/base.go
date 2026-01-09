package db

import "strings"

type OrderByDirection string

const (
	OrderAsc  OrderByDirection = "ASC"
	OrderDesc OrderByDirection = "DESC"
)

type orderByDirection struct {
	orderBy        string
	orderDirection OrderByDirection
}

type DbBase struct{}

type QueryBase struct {
	DbBase
	orderBy []orderByDirection
	limit   int
	offset  int
}

// GetOrderByClause 构建 ORDER BY 子句
func (q *QueryBase) GetOrderByClause() string {
	// 将 QueryBase 中的 orderBy 构造成 order by at_date asc, at_hour asc, at_minute asc 这样的格式
	orderByClause := make([]string, 0, len(q.orderBy))
	for _, orderBy := range q.orderBy {
		orderByClause = append(orderByClause, orderBy.orderBy+" "+string(orderBy.orderDirection))
	}
	return strings.Join(orderByClause, ", ")
}
