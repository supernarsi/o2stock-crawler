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
type OrderBy []orderByDirection

type QueryBase struct {
	DbBase
	orderBy OrderBy
	limit   int
	offset  int
}

// GetOrderByClause 构建 ORDER BY 子句
func (q OrderBy) GetOrderByClause() string {
	// 将 QueryBase 中的 orderBy 构造成 order by at_date asc, at_hour asc, at_minute asc 这样的格式
	orderByClause := make([]string, 0, len(q))
	for _, orderBy := range q {
		orderByClause = append(orderByClause, orderBy.orderBy+" "+string(orderBy.orderDirection))
	}
	return strings.Join(orderByClause, ", ")
}

func NewOrderBy(orderBy string, direction OrderByDirection) OrderBy {
	return OrderBy{{orderBy: orderBy, orderDirection: direction}}
}

func NewOrderBys(orderBys ...orderByDirection) OrderBy {
	return OrderBy(orderBys)
}

func NewOrderByAsc(orderBy string) OrderBy {
	return OrderBy{{orderBy: orderBy, orderDirection: OrderAsc}}
}

func NewOrderByDesc(orderBy string) OrderBy {
	return OrderBy{{orderBy: orderBy, orderDirection: OrderDesc}}
}
