package db

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/model"
	"strings"
)

// PlayerHistoryQuery 获取某个球员的历史价格
type PlayerHistoryQuery struct {
	QueryBase
	playerID uint32
}

// NewPlayerHistoryQuery 创建一个 PlayerHistoryQuery
func NewPlayerHistoryQuery(playerID uint32, limit int) *PlayerHistoryQuery {
	return &PlayerHistoryQuery{
		QueryBase: QueryBase{
			orderBy: NewOrderByDesc("at_date_hour"),
			limit:   limit,
		},
		playerID: playerID,
	}
}

// GetPlayerHistory 返回某个球员的历史价格，按时间升序。
func (s *PlayerHistoryQuery) GetPlayerHistory(ctx context.Context, database *DB) ([]*model.PriceHistoryRow, error) {
	q := fmt.Sprintf(`
SELECT player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, at_minute, price_standard, price_current_sale, price_lower, price_upper
FROM p_p_history
WHERE player_id = ?
ORDER BY %s
LIMIT ?`, s.orderBy.GetOrderByClause())

	rows, err := database.QueryContext(ctx, q, s.playerID, s.limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.PriceHistoryRow
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := rows.Scan(&r.PlayerId, &r.AtDate, &r.AtDateHourStr, &r.AtYear, &r.AtMonth, &r.AtDay, &r.AtHour, &r.AtMinute, &r.PriceStandard, &r.PriceCurrentSale, &r.PriceLower, &r.PriceUpper); err != nil {
			return nil, err
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 倒序排列 result
	func(slice []*model.PriceHistoryRow) {
		for i, j := 0, len(slice)-1; i < j; i, j = i+1, j-1 {
			slice[i], slice[j] = slice[j], slice[i]
		}
	}(result)
	return result, nil
}

// MultiPlayersHistoryQuery 批量获取多个球员的历史价格
type MultiPlayersHistoryQuery struct {
	QueryBase
	playerIDs []uint32
}

// NewMultiPlayersHistoryQuery 创建一个 MultiPlayersHistoryQuery
func NewMultiPlayersHistoryQuery(playerIDs []uint32, limit int) *MultiPlayersHistoryQuery {
	return &MultiPlayersHistoryQuery{
		QueryBase: QueryBase{
			orderBy: NewOrderByDesc("at_date_hour"),
			limit:   limit,
		},
		playerIDs: playerIDs,
	}
}

// GetMultiPlayersHistory 批量获取多个球员的历史价格
func (s *MultiPlayersHistoryQuery) GetMultiPlayersHistory(ctx context.Context, database *DB) (map[uint32][]*model.PriceHistoryRow, error) {
	if len(s.playerIDs) == 0 {
		return make(map[uint32][]*model.PriceHistoryRow), nil
	}

	// 构建 IN 查询的占位符
	var placeholders strings.Builder
	args := make([]any, 0, len(s.playerIDs))
	for i, pid := range s.playerIDs {
		if i > 0 {
			placeholders.WriteString(",")
		}
		placeholders.WriteString("?")
		args = append(args, pid)
	}
	orderByClause := s.orderBy.GetOrderByClause()
	q := fmt.Sprintf(`
SELECT player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, at_minute, price_standard, price_current_sale, price_lower, price_upper
FROM (
  SELECT player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, at_minute, price_standard, price_current_sale, price_lower, price_upper, ROW_NUMBER() OVER (
      PARTITION BY player_id ORDER BY %s) AS rn
    FROM p_p_history
    WHERE player_id IN (%s)
  ) t
  WHERE rn <= %d
  ORDER BY %s
`, orderByClause, placeholders.String(), s.limit, NewOrderByAsc("at_date_hour").GetOrderByClause())

	// fmt.Println("查询语句：", q)

	rows, err := database.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}

	result := make(map[uint32][]*model.PriceHistoryRow)
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := rows.Scan(&r.PlayerId, &r.AtDate, &r.AtDateHourStr, &r.AtYear, &r.AtMonth, &r.AtDay, &r.AtHour, &r.AtMinute, &r.PriceStandard, &r.PriceCurrentSale, &r.PriceLower, &r.PriceUpper); err != nil {
			return nil, err
		}
		playerID := uint32(r.PlayerId)
		result[playerID] = append(result[playerID], &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 限制每个球员的记录数
	for pid := range result {
		if len(result[pid]) > s.limit {
			result[pid] = result[pid][:s.limit]
		}
	}

	return result, nil
}
