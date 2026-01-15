package db

import (
	"context"
	"fmt"
	"log"
	"o2stock-crawler/internal/model"
	"strconv"
	"time"
)

/*
球员历史价格表
```sql
CREATE TABLE `p_p_history` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `player_id` int unsigned NOT NULL DEFAULT '0' COMMENT '球员 id',
  `at_date` date NOT NULL COMMENT '价格对应的日期',
  `at_date_hour` char(12) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '202601010000' COMMENT '价格对应的日期小时分钟，格式为：年月日时分（例 202601022305）',
  `at_year` char(4) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '2026' COMMENT '价格对应的年份',
  `at_month` char(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '01' COMMENT '价格对应的月份',
  `at_day` char(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '01' COMMENT '价格对应的日期',
  `at_hour` char(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '00' COMMENT '价格对应的小时',
  `at_minute` char(2) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '00' COMMENT '价格对应的分钟',
  `price_standard` int unsigned NOT NULL DEFAULT '0' COMMENT '基础卡片单卡价格',
  `price_current_sale` int NOT NULL DEFAULT '-1' COMMENT '当前成交价，-1 代表无人出售',
  `price_lower` int unsigned NOT NULL DEFAULT '0' COMMENT '出售最低价',
  `price_upper` int unsigned NOT NULL DEFAULT '0' COMMENT '出售最高价',
  `c_time` datetime NOT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_dh_pid` (`at_date_hour`,`player_id`),
  KEY `idx_pid` (`player_id`),
  KEY `idx_date` (`at_year`,`at_month`,`at_day`,`at_hour`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='球员历史价格';
```
*/

// SQL 查询字段常量
const (
	selectPriceHistoryFields = `player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, at_minute, price_standard, price_current_sale, price_lower, price_upper`
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
// period: 1-过去24小时（所有数据），2-过去3天（每小时1条），3-过去7天（每2小时1条）
func (q *PlayerHistoryQuery) GetPlayerHistory(ctx context.Context, database *DB, period uint8) ([]*model.PriceHistoryRow, error) {
	startTime := calculateHistoryStartTime(period)

	query := fmt.Sprintf(`
SELECT %s
FROM p_p_history
WHERE player_id = ?
AND at_date_hour >= ?
ORDER BY %s
LIMIT ?`, selectPriceHistoryFields, q.orderBy.GetOrderByClause())

	rows, err := database.QueryContext(ctx, query, q.playerID, FormatDateTimeHour(startTime), q.limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query player history: %w", err)
	}

	defer rows.Close()

	result := make([]*model.PriceHistoryRow, 0, q.limit)
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := scanPriceHistoryRow(rows, &r); err != nil {
			return nil, fmt.Errorf("failed to scan price history row: %w", err)
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating price history rows: %w", err)
	}

	// log.Println("result", len(result))

	// 倒序排列（因为查询是按降序，需要转为升序返回）
	reversePriceHistoryRows(result)

	// 根据 period 过滤数据
	filteredResult := filterHistoryByPeriod(result, period)

	return filteredResult, nil
}

// calculateHistoryStartTime 根据 period 计算历史价格查询的开始时间
func calculateHistoryStartTime(period uint8) time.Time {
	now := time.Now()
	switch period {
	case Period1Day:
		return now.AddDate(0, 0, -1)
	case Period3Days:
		return now.AddDate(0, 0, -3)
	case Period1Week:
		return now.AddDate(0, 0, -7)
	default:
		return now.AddDate(0, 0, -1) // 默认为1天
	}
}

// filterHistoryByPeriod 根据 period 过滤历史价格数据
func filterHistoryByPeriod(result []*model.PriceHistoryRow, period uint8) []*model.PriceHistoryRow {
	switch period {
	case Period1Day:
		// 过去24小时：返回所有数据
		return result
	case Period3Days:
		return filterHistoryByHourInterval(result)
	case Period1Week:
		return filterHistoryByHourInterval(result)
	default:
		return result
	}
}

// filterHistoryByHourInterval 按小时间隔过滤历史数据
func filterHistoryByHourInterval(result []*model.PriceHistoryRow) []*model.PriceHistoryRow {
	if len(result) == 0 {
		return result
	}

	filtered := make([]*model.PriceHistoryRow, 0, len(result))
	stDh := 0
	for _, r := range result {
		atDateHour := r.AtDateHourStr[0:10]
		log.Println("atDateHour", atDateHour)
		atDateHourInt, err := strconv.Atoi(atDateHour)
		if err != nil {
			continue
		}
		if atDateHourInt > stDh {
			filtered = append(filtered, r)
			stDh = atDateHourInt
		}
	}

	return filtered
}

// scanPriceHistoryRow 扫描价格历史行数据
func scanPriceHistoryRow(rows interface {
	Scan(dest ...any) error
}, r *model.PriceHistoryRow) error {
	return rows.Scan(
		&r.PlayerId,
		&r.AtDate,
		&r.AtDateHourStr,
		&r.AtYear,
		&r.AtMonth,
		&r.AtDay,
		&r.AtHour,
		&r.AtMinute,
		&r.PriceStandard,
		&r.PriceCurrentSale,
		&r.PriceLower,
		&r.PriceUpper,
	)
}

// reversePriceHistoryRows 反转价格历史行切片
func reversePriceHistoryRows(slice []*model.PriceHistoryRow) {
	for i, j := 0, len(slice)-1; i < j; i, j = i+1, j-1 {
		slice[i], slice[j] = slice[j], slice[i]
	}
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
func (q *MultiPlayersHistoryQuery) GetMultiPlayersHistory(ctx context.Context, database *DB) (map[uint32][]*model.PriceHistoryRow, error) {
	if len(q.playerIDs) == 0 {
		return make(map[uint32][]*model.PriceHistoryRow), nil
	}

	// 构建 IN 查询
	placeholders, args := buildINClause(convertUint32ToAny(q.playerIDs))
	orderByClause := q.orderBy.GetOrderByClause()

	query := fmt.Sprintf(`
SELECT %s
FROM (
  SELECT %s, ROW_NUMBER() OVER (PARTITION BY player_id ORDER BY %s) AS rn
  FROM p_p_history
  WHERE player_id IN (%s)
) t
WHERE rn <= %d
ORDER BY %s`, selectPriceHistoryFields, selectPriceHistoryFields, orderByClause, placeholders, q.limit, NewOrderByAsc("at_date_hour").GetOrderByClause())

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query multi players history: %w", err)
	}
	defer rows.Close()

	result := make(map[uint32][]*model.PriceHistoryRow)
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := scanPriceHistoryRow(rows, &r); err != nil {
			return nil, fmt.Errorf("failed to scan price history row: %w", err)
		}
		playerID := uint32(r.PlayerId)
		result[playerID] = append(result[playerID], &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating price history rows: %w", err)
	}

	// 限制每个球员的记录数（虽然 SQL 已经限制，但这里作为双重保险）
	for pid := range result {
		if len(result[pid]) > q.limit {
			result[pid] = result[pid][:q.limit]
		}
	}

	return result, nil
}

// convertUint32ToAny 将 []uint32 转换为 []any
func convertUint32ToAny(values []uint32) []any {
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}

// PriceHistoryMapQuery 获取指定时间点的价格快照
type PriceHistoryMapQuery struct {
	QueryBase
}

// NewPriceHistoryMapQuery 创建一个 PriceHistoryMapQuery
func NewPriceHistoryMapQuery() *PriceHistoryMapQuery {
	return &PriceHistoryMapQuery{
		QueryBase: QueryBase{},
	}
}

// GetPriceHistoryMap 获取指定时间点之后最早的价格快照
// 用于计算价格变动，返回每个球员在指定时间点之后的第一条价格记录
func (q *PriceHistoryMapQuery) GetPriceHistoryMap(ctx context.Context, database *DB, beforeTime time.Time) (model.PriceHistoryMap, error) {
	const query = `SELECT 
    p1.player_id,
    p1.at_date_hour,
    p1.price_standard,
    p1.price_current_sale,
    p1.price_lower,
    p1.price_upper,
    p1.at_date,
    p1.at_year,
    p1.at_month,
    p1.at_day,
    p1.at_hour,
    p1.at_minute
FROM p_p_history p1
INNER JOIN (
    SELECT 
        player_id,
        MIN(at_date_hour) as min_hour
    FROM p_p_history
    WHERE at_date_hour >= ?
    GROUP BY player_id
) p2 ON p1.player_id = p2.player_id AND p1.at_date_hour = p2.min_hour
ORDER BY p1.player_id`

	rows, err := database.QueryContext(ctx, query, FormatDateTimeHour(beforeTime))
	if err != nil {
		return nil, fmt.Errorf("failed to query price history map: %w", err)
	}
	defer rows.Close()

	priceHistoryMap := make(map[uint]*model.PriceHistoryRow)
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := rows.Scan(
			&r.PlayerId,
			&r.AtDateHourStr,
			&r.PriceStandard,
			&r.PriceCurrentSale,
			&r.PriceLower,
			&r.PriceUpper,
			&r.AtDate,
			&r.AtYear,
			&r.AtMonth,
			&r.AtDay,
			&r.AtHour,
			&r.AtMinute,
		); err != nil {
			return nil, fmt.Errorf("failed to scan price history map row: %w", err)
		}
		priceHistoryMap[r.PlayerId] = &r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating price history map rows: %w", err)
	}

	return priceHistoryMap, nil
}
