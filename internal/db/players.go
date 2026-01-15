package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"o2stock-crawler/internal/model"
)

/*
球员表
```sql
CREATE TABLE `players` (

	`id` int unsigned NOT NULL AUTO_INCREMENT,
	`player_id` int unsigned NOT NULL COMMENT '球员 id',
	`p_name_show` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_vi_0900_ai_ci NOT NULL COMMENT '球员展示名称',
	`p_name_en` varchar(255) COLLATE utf8mb4_vi_0900_ai_ci NOT NULL COMMENT '球员英文名称',
	`team_abbr` varchar(255) COLLATE utf8mb4_vi_0900_ai_ci NOT NULL COMMENT '球队名称',
	`version` int unsigned NOT NULL DEFAULT '0' COMMENT '球员版本',
	`card_type` int unsigned NOT NULL DEFAULT '0' COMMENT '卡类型',
	`player_img` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_vi_0900_ai_ci NOT NULL COMMENT '球员头像',
	`price_standard` int unsigned NOT NULL DEFAULT '0' COMMENT '单卡价格-基准',
	`price_current_lowest` int unsigned NOT NULL DEFAULT '0' COMMENT '市场最低售价',
	`price_sale_lower` int unsigned NOT NULL DEFAULT '0' COMMENT '售价-低',
	`price_sale_upper` int unsigned NOT NULL DEFAULT '0' COMMENT '售价-高',
	PRIMARY KEY (`id`),
	UNIQUE KEY `idx_pid` (`player_id`)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_vi_0900_ai_ci COMMENT='球员价格表';

```
*/

const (
	// OrderByPriceStandard 按标准价格排序
	OrderByPriceStandard = "price_standard"
	// OrderByPriceChange 按涨跌幅排序
	OrderByPriceChange = "price_change"
	// OrderByPlayerID 按球员ID排序（默认）
	OrderByPlayerID = "player_id"
)

const (
	// Period1Day 24小时数据涨跌幅
	Period1Day uint8 = 1
	// Period3Days 3天数据涨跌幅
	Period3Days uint8 = 2
	// Period1Week 1周数据涨跌幅
	Period1Week uint8 = 3
)

// SQL 查询模板
const (
	// selectPlayersFields 球员表查询字段
	selectPlayersFields = `player_id, p_name_show, p_name_en, team_abbr, version, card_type, player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper`

	// queryPriceRatioBase SQL 基础查询（价格变动）
	queryPriceRatioBase = `WITH recent_data AS (
  SELECT
    player_id, at_date_hour, price_standard, price_current_sale,
    ROW_NUMBER() OVER ( PARTITION BY player_id ORDER BY at_date_hour DESC) AS rn_desc,
    ROW_NUMBER() OVER ( PARTITION BY player_id ORDER BY at_date_hour ASC) AS rn_asc
  FROM p_p_history
  WHERE at_date_hour >= ?
  %s
)
SELECT
  cur.player_id,
  old.price_standard AS price_old,
  cur.price_standard AS price_now,
  (CAST(cur.price_standard AS SIGNED) - CAST(old.price_standard AS SIGNED)) / old.price_standard AS price_ratio
FROM recent_data cur
JOIN recent_data old ON cur.player_id = old.player_id
WHERE cur.rn_desc = 1
  AND old.rn_asc = 1
  AND old.price_standard > 0
  %s
ORDER BY price_ratio %s`
)

// PlayerFilter 封装查询条件
type PlayerFilter struct {
	Page      int
	Limit     int
	OrderBy   string
	OrderAsc  bool
	Period    uint8
	SoldOut   bool
	PlayerIDs []uint
}

// GetOffset 计算偏移量
func (f *PlayerFilter) GetOffset() int {
	if f.Page < 1 {
		return 0
	}
	return (f.Page - 1) * f.Limit
}

// GetOrderDirection 获取排序方向
func (f *PlayerFilter) GetOrderDirection() string {
	if f.OrderAsc {
		return string(OrderAsc)
	}
	return string(OrderDesc)
}

// GetStartTime 根据周期计算开始时间
func (f *PlayerFilter) GetStartTime() time.Time {
	now := time.Now()
	switch f.Period {
	case Period1Week:
		return now.AddDate(0, 0, -7)
	case Period3Days:
		return now.AddDate(0, 0, -3)
	default: // Period1Day
		return now.AddDate(0, 0, -1)
	}
}

// PlayersQuery 获取球员列表
type PlayersQuery struct {
	filter PlayerFilter
}

// NewPlayersQuery 创建一个 PlayersQuery
func NewPlayersQuery(page, limit int, orderBy string, orderAsc bool) *PlayersQuery {
	// 限制排序字段，只允许 price_change 和 price_standard
	if orderBy != OrderByPriceChange && orderBy != OrderByPriceStandard {
		orderBy = OrderByPlayerID
	}
	return &PlayersQuery{
		filter: PlayerFilter{
			Page:     page,
			Limit:    limit,
			OrderBy:  orderBy,
			OrderAsc: orderAsc,
		},
	}
}

// ListPlayers 返回简单的球员列表，支持按价格或涨跌幅排序，可分页。
func (s *PlayersQuery) ListPlayers(ctx context.Context, database *DB, period uint8, soldOut bool) ([]*model.PlayerWithPriceChange, error) {
	// 更新 Filter 中的条件
	s.filter.Period = period
	s.filter.SoldOut = soldOut

	switch s.filter.OrderBy {
	case OrderByPriceStandard:
		// 如果按价格排序，直接查询 players 表，按 price_standard 排序，再查 p_p_history 表计算涨跌幅
		return s.queryPlayersOrderByPrice(ctx, database)
	case OrderByPriceChange:
		// 如果按涨跌幅排序，先使用窗口函数从 p_p_history 表中获取按涨跌幅排序后的球员 id，再使用 in 查询从 players 表中获取球员信息
		return s.queryPlayersOrderByPriceRatio(ctx, database)
	default:
		// 默认按涨跌幅排序
		return s.queryPlayersOrderByPriceRatio(ctx, database)
	}
}

// ListPlayersWithOwned 返回球员列表，并可选地包含用户的拥有信息
func (s *PlayersQuery) ListPlayersWithOwned(ctx context.Context, database *DB, period uint8, userID *uint, soldOut bool) ([]*model.PlayerWithPriceChange, map[uint][]*model.OwnInfo, error) {
	players, err := s.ListPlayers(ctx, database, period, soldOut)
	if err != nil {
		return nil, nil, err
	}

	var ownedMap map[uint][]*model.OwnInfo
	if userID != nil && len(players) > 0 {
		playerIDs := extractPlayerIDsFromPlayersWithPriceChange(players)
		ownedQuery := NewUserPlayerOwnQuery(*userID)
		ownedMap, err = ownedQuery.GetOwnedInfoByPlayerIDs(ctx, database, playerIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get owned info: %w", err)
		}
	}

	return players, ownedMap, nil
}

// GetPlayerInfo 获取单个球员信息
func (s *PlayersQuery) GetPlayerInfo(ctx context.Context, database *DB, playerID uint) (*model.Players, error) {
	query := fmt.Sprintf(`SELECT %s FROM players WHERE player_id = ?`, selectPlayersFields)
	player, err := queryPlayers(ctx, database, query, playerID)
	if err != nil || len(player) == 0 {
		return nil, fmt.Errorf("failed to get player info: %w", err)
	}
	return player[0], nil
}

// queryPlayersOrderByPrice 按价格排序查询球员价格
func (s *PlayersQuery) queryPlayersOrderByPrice(ctx context.Context, database *DB) ([]*model.PlayerWithPriceChange, error) {
	orderDir := s.filter.GetOrderDirection()
	filterClause := ""
	if s.filter.SoldOut {
		filterClause = "WHERE price_current_lowest = 0"
	}
	q := fmt.Sprintf(`SELECT %s
FROM players 
%s
ORDER BY price_standard %s 
LIMIT ? OFFSET ?`, selectPlayersFields, filterClause, orderDir)

	players, err := s.queryPlayers(ctx, database, q, s.filter.Limit, s.filter.GetOffset())
	if err != nil {
		return nil, fmt.Errorf("failed to query players by price: %w", err)
	}

	// 如果没有查询到球员，直接返回空结果
	if len(players) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 提取球员ID列表
	playerIds := extractPlayerIDs(players)

	// 查询涨跌幅数据（注意：这里不需要排序和分页，因为已经按价格排序了）
	// 创建一个新的 Filter 用于查询比率，不带分页
	ratioFilter := s.filter
	ratioFilter.PlayerIDs = playerIds
	ratioFilter.Limit = 0 // 不分页

	priceRatio, err := s.queryPlayersRatio(ctx, database, ratioFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to query price ratio: %w", err)
	}

	// 组装成 []*model.PlayerWithPriceChange 数据（以球员价格为基准）
	return s.mergePlayersPriceChangeByPriceStandard(players, priceRatio), nil
}

// queryPlayersOrderByPriceRatio 按涨跌幅排序查询球员价格变动
func (s *PlayersQuery) queryPlayersOrderByPriceRatio(ctx context.Context, database *DB) ([]*model.PlayerWithPriceChange, error) {
	// 查询涨跌幅数据（已按涨跌幅排序和分页）
	priceRatio, err := s.queryPlayersRatio(ctx, database, s.filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query price ratio: %w", err)
	}

	// 如果没有查询到涨跌幅数据，直接返回空结果
	if len(priceRatio) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 提取球员ID列表
	playerIds := extractPlayerIDsFromPriceChange(priceRatio)

	// 查询球员数据
	players, err := s.GetPlayersByIDs(ctx, database, playerIds, s.filter.SoldOut)
	if err != nil {
		return nil, fmt.Errorf("failed to get players by IDs: %w", err)
	}

	// 组装成 []*model.PlayerWithPriceChange 数据（以涨跌幅为基准）
	return s.mergePlayersPriceChangeByPriceRatio(players, priceRatio), nil
}

// queryPlayersRatio 查询球员价格变动（涨跌幅）
func (s *PlayersQuery) queryPlayersRatio(ctx context.Context, database *DB, filter PlayerFilter) ([]*model.PlayerPriceChange, error) {
	orderDir := filter.GetOrderDirection()
	atDateHour := FormatDateTimeHour(filter.GetStartTime())

	// 构建查询参数
	args := []any{atDateHour}
	playerFilterClause := buildPlayerIDFilter(filter.PlayerIDs, &args)
	soldOutFilterClause := ""
	if filter.SoldOut {
		soldOutFilterClause = " AND cur.price_current_sale = -1"
	}
	// 构建 SQL 查询
	q := fmt.Sprintf(queryPriceRatioBase, playerFilterClause, soldOutFilterClause, orderDir)
	if filter.Limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.GetOffset())
	}

	// log.Println("查询语句：", q, "参数：", args)

	rows, err := database.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query price ratio: %w", err)
	}
	defer rows.Close()

	result := make([]*model.PlayerPriceChange, 0)
	for rows.Next() {
		var r model.PlayerPriceChange
		if err := rows.Scan(
			&r.PlayerID,
			&r.PriceOld,
			&r.PriceNow,
			&r.ChangeRatio,
		); err != nil {
			return nil, fmt.Errorf("failed to scan price change row: %w", err)
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating price change rows: %w", err)
	}
	return result, nil
}

// GetPlayersByIDs 根据球员 ID 列表获取球员信息
func (s *PlayersQuery) GetPlayersByIDs(ctx context.Context, database *DB, playerIDs []uint, soldOut bool) ([]*model.Players, error) {
	if len(playerIDs) == 0 {
		return []*model.Players{}, nil
	}

	// 构建 IN 查询
	placeholders := make([]string, len(playerIDs))
	args := make([]any, len(playerIDs))
	for i, pid := range playerIDs {
		placeholders[i] = "?"
		args[i] = pid
	}

	filter := ""
	if soldOut {
		filter = " AND price_current_lowest = 0"
	}
	q := fmt.Sprintf(`SELECT %s FROM players WHERE player_id IN (%s)%s`, selectPlayersFields, strings.Join(placeholders, ","), filter)

	return queryPlayers(ctx, database, q, args...)
}

// GetPlayersWithPriceChangeByIDs 根据球员ID列表获取包含价格变动的球员信息
func (s *PlayersQuery) GetPlayersWithPriceChangeByIDs(ctx context.Context, database *DB, playerIDs []uint, period uint8) ([]*model.PlayerWithPriceChange, error) {
	if len(playerIDs) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 设置过滤条件
	s.filter.PlayerIDs = playerIDs
	s.filter.Period = period
	s.filter.Limit = 0 // 不限制数量

	// 复用按涨跌幅查询的逻辑（因为它已经支持了 PlayerIDs 过滤和数据组装）
	return s.queryPlayersOrderByPriceRatio(ctx, database)
}

// ============================================================================
// 数据合并函数
// ============================================================================

// mergePlayersPriceChangeByPriceRatio 合并球员信息和价格变动（以涨跌幅为基准）
// 保持 priceChange 的顺序，只包含在 players 中存在的球员
func (s *PlayersQuery) mergePlayersPriceChangeByPriceRatio(players []*model.Players, priceChange []*model.PlayerPriceChange) []*model.PlayerWithPriceChange {
	playersMap := buildPlayersMap(players)
	return mergeByPriceChangeOrder(priceChange, playersMap)
}

// mergePlayersPriceChangeByPriceStandard 合并球员信息和价格变动（以球员价格为基准）
// 保持 players 的顺序，只包含有价格变动数据的球员
func (s *PlayersQuery) mergePlayersPriceChangeByPriceStandard(players []*model.Players, priceChange []*model.PlayerPriceChange) []*model.PlayerWithPriceChange {
	priceRatioMap := buildPriceChangeMap(priceChange)
	return mergeByPlayersOrder(players, priceRatioMap)
}

// buildPlayersMap 构建球员ID到球员信息的映射
func buildPlayersMap(players []*model.Players) map[uint]*model.Players {
	playersMap := make(map[uint]*model.Players, len(players))
	for _, p := range players {
		playersMap[p.PlayerID] = p
	}
	return playersMap
}

// buildPriceChangeMap 构建球员ID到价格变动的映射
func buildPriceChangeMap(priceChange []*model.PlayerPriceChange) map[uint]*model.PlayerPriceChange {
	priceRatioMap := make(map[uint]*model.PlayerPriceChange, len(priceChange))
	for _, pc := range priceChange {
		priceRatioMap[pc.PlayerID] = pc
	}
	return priceRatioMap
}

// mergeByPriceChangeOrder 按价格变动顺序合并（以 priceChange 为基准）
func mergeByPriceChangeOrder(priceChange []*model.PlayerPriceChange, playersMap map[uint]*model.Players) []*model.PlayerWithPriceChange {
	res := make([]*model.PlayerWithPriceChange, 0, len(priceChange))
	for _, pRatio := range priceChange {
		playerInfo, ok := playersMap[pRatio.PlayerID]
		if !ok || playerInfo == nil {
			continue
		}
		res = append(res, &model.PlayerWithPriceChange{
			Players:     *playerInfo,
			PriceChange: pRatio.ChangeRatio,
		})
	}
	return res
}

// mergeByPlayersOrder 按球员顺序合并（以 players 为基准）
func mergeByPlayersOrder(players []*model.Players, priceRatioMap map[uint]*model.PlayerPriceChange) []*model.PlayerWithPriceChange {
	res := make([]*model.PlayerWithPriceChange, 0, len(players))
	for _, player := range players {
		changRatio := 0.0
		priceRatio, ok := priceRatioMap[player.PlayerID]
		if ok && priceRatio != nil {
			changRatio = priceRatio.ChangeRatio
		}
		res = append(res, &model.PlayerWithPriceChange{
			Players:     *player,
			PriceChange: changRatio,
		})
	}
	return res
}

// extractPlayerIDsFromPlayersWithPriceChange 从 PlayerWithPriceChange 列表中提取ID
func extractPlayerIDsFromPlayersWithPriceChange(players []*model.PlayerWithPriceChange) []uint {
	ids := make([]uint, len(players))
	for i, p := range players {
		ids[i] = p.PlayerID
	}
	return ids
}

// ============================================================================
// 辅助函数：查询相关
// ============================================================================

// queryPlayers 执行球员查询并返回结果
func queryPlayers(ctx context.Context, database *DB, query string, args ...any) ([]*model.Players, error) {
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query players: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Players, 0)
	for rows.Next() {
		var r model.Players
		if err := scanPlayerRow(rows, &r); err != nil {
			return nil, fmt.Errorf("failed to scan player row: %w", err)
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating player rows: %w", err)
	}
	return result, nil
}

// queryPlayers 执行球员查询（PlayersQuery 的方法版本）
func (s *PlayersQuery) queryPlayers(ctx context.Context, database *DB, query string, args ...any) ([]*model.Players, error) {
	return queryPlayers(ctx, database, query, args...)
}

// scanPlayerRow 扫描球员行数据
func scanPlayerRow(rows interface {
	Scan(dest ...any) error
}, r *model.Players) error {
	return rows.Scan(
		&r.PlayerID,
		&r.ShowName,
		&r.EnName,
		&r.TeamAbbr,
		&r.Version,
		&r.CardType,
		&r.PlayerImg,
		&r.PriceStandard,
		&r.PriceCurrentLower,
		&r.PriceSaleLower,
		&r.PriceSaleUpper,
	)
}

// ============================================================================
// 辅助函数：数据处理
// ============================================================================

// extractPlayerIDs 从球员列表中提取ID
func extractPlayerIDs(players []*model.Players) []uint {
	ids := make([]uint, len(players))
	for i, p := range players {
		ids[i] = p.PlayerID
	}
	return ids
}

// extractPlayerIDsFromPriceChange 从价格变动列表中提取球员ID
func extractPlayerIDsFromPriceChange(priceChanges []*model.PlayerPriceChange) []uint {
	ids := make([]uint, len(priceChanges))
	for i, pc := range priceChanges {
		ids[i] = pc.PlayerID
	}
	return ids
}

// buildPlayerIDFilter 构建球员ID过滤条件
func buildPlayerIDFilter(playerIDs []uint, args *[]any) string {
	if len(playerIDs) == 0 {
		return ""
	}
	return buildINClauseWithPrefix("player_id", convertUintToAny(playerIDs), args)
}
