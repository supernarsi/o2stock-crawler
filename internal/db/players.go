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
	// OrderByPowerPer5 按近5场战力值排序
	OrderByPowerPer5 = "power_per5"
	// OrderByPowerPer10 按近10场战力值排序
	OrderByPowerPer10 = "power_per10"
	// OrderByOverAll 按球员能力值排序
	OrderByOverAll = "over_all"
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
	selectPlayersFields = `player_id, nba_player_id, p_name_show, p_name_en, team_abbr, version, card_type, player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper, over_all, power_per5, power_per10, price_change_1d, price_change_7d`
)

// PlayerFilter 封装查询条件
type PlayerFilter struct {
	Page       int
	Limit      int
	OrderBy    string
	OrderAsc   bool
	Period     uint8
	SoldOut    bool
	PlayerName string
	PlayerIDs  []uint
	MinPrice   uint
	MaxPrice   uint
	ExFree     bool
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
func NewPlayersQuery(filter PlayerFilter) *PlayersQuery {
	// 限制排序字段
	validOrderBy := map[string]bool{
		OrderByPriceChange:   true,
		OrderByPriceStandard: true,
		OrderByPowerPer5:     true,
		OrderByPowerPer10:    true,
		OrderByOverAll:       true,
	}
	if !validOrderBy[filter.OrderBy] {
		filter.OrderBy = OrderByPlayerID
	}
	return &PlayersQuery{
		filter: filter,
	}
}

// GetAllPlayers 获取所有球员基础信息（无价格过滤）
func (s *PlayersQuery) GetAllPlayers(ctx context.Context, database *DB) ([]*model.Players, error) {
	q := fmt.Sprintf(`SELECT %s FROM players`, selectPlayersFields)
	return queryPlayers(ctx, database, q)
}

// ListPlayers 返回简单的球员列表，支持按价格或涨跌幅排序，可分页。
func (s *PlayersQuery) ListPlayers(ctx context.Context, database *DB) ([]*model.PlayerWithPriceChange, error) {
	if s.filter.PlayerName != "" {
		// 如果搜索名字，直接指定按价格倒序排序
		s.filter.OrderAsc = false
		s.filter.OrderBy = OrderByPriceStandard
	}

	switch s.filter.OrderBy {
	case OrderByPriceStandard:
		// 如果按价格排序，直接查询 players 表，按 price_standard 排序
		return s.queryPlayersOrderByPrice(ctx, database)
	case OrderByPriceChange:
		// 如果按涨跌幅排序，直接查询 players 表
		return s.queryPlayersOrderByPriceRatioNew(ctx, database)
	case OrderByPowerPer5, OrderByPowerPer10, OrderByOverAll:
		// 按战力值或能力值排序
		return s.queryPlayersOrderByPower(ctx, database)
	default:
		// 默认按涨跌幅排序
		return s.queryPlayersOrderByPriceRatioNew(ctx, database)
	}
}

// ListPlayersWithOwned 返回球员列表，并可选地包含用户的拥有信息
func (s *PlayersQuery) ListPlayersWithOwned(ctx context.Context, database *DB, userID *uint) ([]*model.PlayerWithPriceChange, map[uint][]*model.OwnInfo, error) {
	players, err := s.ListPlayers(ctx, database)
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
	args := []any{}
	filterClause := ""
	if s.filter.SoldOut {
		filterClause = " AND price_current_lowest = 0"
	}
	if s.filter.PlayerName != "" {
		filterClause += " AND p_name_show LIKE ?"
		args = append(args, "%"+s.filter.PlayerName+"%")
	}
	if s.filter.MinPrice > 0 {
		filterClause += " AND price_sale_lower >= ?"
		args = append(args, s.filter.MinPrice)
	}
	if s.filter.MaxPrice > 0 {
		filterClause += " AND price_sale_upper <= ?"
		args = append(args, s.filter.MaxPrice)
	}
	if s.filter.ExFree {
		filterClause += " AND team_abbr != '自由球员'"
	}

	args = append(args, s.filter.Limit, s.filter.GetOffset())

	q := fmt.Sprintf(`SELECT %s
FROM players 
WHERE price_standard >= 5000
%s
ORDER BY price_standard %s 
LIMIT ? OFFSET ?`, selectPlayersFields, filterClause, orderDir)
	// log.Println("query: ", q)
	players, err := s.queryPlayers(ctx, database, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query players by price: %w", err)
	}

	// 如果没有查询到球员，直接返回空结果
	if len(players) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 组装成 []*model.PlayerWithPriceChange 数据
	result := make([]*model.PlayerWithPriceChange, len(players))
	for i, p := range players {
		changeRatio := p.PriceChange1d
		if s.filter.Period == Period1Week {
			changeRatio = p.PriceChange7d
		}
		result[i] = &model.PlayerWithPriceChange{
			Players:     *p,
			PriceChange: changeRatio,
		}
	}
	return result, nil
}

// queryPlayersOrderByPriceRatioNew 按涨跌幅排序查询球员价格变动（使用预计算字段）
func (s *PlayersQuery) queryPlayersOrderByPriceRatioNew(ctx context.Context, database *DB) ([]*model.PlayerWithPriceChange, error) {
	orderDir := s.filter.GetOrderDirection()
	args := []any{}
	filterClause := ""

	orderByField := "price_change_1d"
	if s.filter.Period == Period1Week {
		orderByField = "price_change_7d"
	}

	if s.filter.SoldOut {
		filterClause = " AND price_current_lowest = 0"
	}
	if s.filter.PlayerName != "" {
		filterClause += " AND p_name_show LIKE ?"
		args = append(args, "%"+s.filter.PlayerName+"%")
	}
	if s.filter.MinPrice > 0 {
		filterClause += " AND price_sale_lower >= ?"
		args = append(args, s.filter.MinPrice)
	}
	if s.filter.MaxPrice > 0 {
		filterClause += " AND price_sale_upper <= ?"
		args = append(args, s.filter.MaxPrice)
	}
	if s.filter.ExFree {
		filterClause += " AND team_abbr != '自由球员'"
	}

	args = append(args, s.filter.Limit, s.filter.GetOffset())

	q := fmt.Sprintf(`SELECT %s
FROM players 
WHERE price_standard >= 5000
%s
ORDER BY %s %s 
LIMIT ? OFFSET ?`, selectPlayersFields, filterClause, orderByField, orderDir)

	players, err := s.queryPlayers(ctx, database, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query players by price ratio: %w", err)
	}

	// 组装成 []*model.PlayerWithPriceChange 数据
	result := make([]*model.PlayerWithPriceChange, len(players))
	for i, p := range players {
		changeRatio := p.PriceChange1d
		if s.filter.Period == Period1Week {
			changeRatio = p.PriceChange7d
		}
		result[i] = &model.PlayerWithPriceChange{
			Players:     *p,
			PriceChange: changeRatio,
		}
	}
	return result, nil
}

// queryPlayersOrderByPower 按战力值或能力值排序查询球员
func (s *PlayersQuery) queryPlayersOrderByPower(ctx context.Context, database *DB) ([]*model.PlayerWithPriceChange, error) {
	orderDir := s.filter.GetOrderDirection()
	args := []any{}
	filterClause := ""

	// 根据排序字段添加过滤条件
	if s.filter.OrderBy == OrderByPowerPer5 {
		filterClause = " AND power_per5 > 0"
	} else if s.filter.OrderBy == OrderByPowerPer10 {
		filterClause = " AND power_per10 > 0"
	}

	if s.filter.SoldOut {
		filterClause += " AND price_current_lowest = 0"
	}
	if s.filter.PlayerName != "" {
		filterClause += " AND p_name_show LIKE ?"
		args = append(args, "%"+s.filter.PlayerName+"%")
	}
	if s.filter.MinPrice > 0 {
		filterClause += " AND price_sale_lower >= ?"
		args = append(args, s.filter.MinPrice)
	}
	if s.filter.MaxPrice > 0 {
		filterClause += " AND price_sale_upper <= ?"
		args = append(args, s.filter.MaxPrice)
	}
	if s.filter.ExFree {
		filterClause += " AND team_abbr != '自由球员'"
	}

	args = append(args, s.filter.Limit, s.filter.GetOffset())

	q := fmt.Sprintf(`SELECT %s
FROM players 
WHERE price_standard >= 5000
%s
ORDER BY %s %s 
LIMIT ? OFFSET ?`, selectPlayersFields, filterClause, s.filter.OrderBy, orderDir)

	players, err := s.queryPlayers(ctx, database, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query players by power: %w", err)
	}

	// 如果没有查询到球员，直接返回空结果
	if len(players) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 组装成 []*model.PlayerWithPriceChange 数据
	result := make([]*model.PlayerWithPriceChange, len(players))
	for i, p := range players {
		changeRatio := p.PriceChange1d
		if s.filter.Period == Period1Week {
			changeRatio = p.PriceChange7d
		}
		result[i] = &model.PlayerWithPriceChange{
			Players:     *p,
			PriceChange: changeRatio,
		}
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

// GetPlayersByIDsWithFilter 根据球员 ID 列表获取球员信息，应用完整的过滤条件
func (s *PlayersQuery) GetPlayersByIDsWithFilter(ctx context.Context, database *DB, playerIDs []uint, filter PlayerFilter) ([]*model.Players, error) {
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

	filterClause := ""
	if filter.SoldOut {
		filterClause += " AND price_current_lowest = 0"
	}
	if filter.PlayerName != "" {
		filterClause += " AND p_name_show LIKE ?"
		args = append(args, "%"+filter.PlayerName+"%")
	}
	if filter.MinPrice > 0 {
		filterClause += " AND price_sale_lower >= ?"
		args = append(args, filter.MinPrice)
	}
	if filter.MaxPrice > 0 {
		filterClause += " AND price_sale_upper <= ?"
		args = append(args, filter.MaxPrice)
	}
	if filter.ExFree {
		filterClause += " AND team_abbr != '自由球员'"
	}

	q := fmt.Sprintf(`SELECT %s FROM players WHERE player_id IN (%s)%s`, selectPlayersFields, strings.Join(placeholders, ","), filterClause)

	return queryPlayers(ctx, database, q, args...)
}

// GetPlayersWithPriceChangeByIDs 根据球员ID列表获取包含价格变动的球员信息
func (s *PlayersQuery) GetPlayersWithPriceChangeByIDs(ctx context.Context, database *DB, playerIDs []uint, period uint8) ([]*model.PlayerWithPriceChange, error) {
	if len(playerIDs) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 1. 先获取所有球员基础信息
	players, err := s.GetPlayersByIDs(ctx, database, playerIDs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get players by IDs: %w", err)
	}

	if len(players) == 0 {
		return []*model.PlayerWithPriceChange{}, nil
	}

	// 2. 组装成 []*model.PlayerWithPriceChange 数据（注意：现在直接使用预计算字段）
	result := make([]*model.PlayerWithPriceChange, len(players))
	for i, p := range players {
		changeRatio := p.PriceChange1d
		if period == Period1Week {
			changeRatio = p.PriceChange7d
		}
		result[i] = &model.PlayerWithPriceChange{
			Players:     *p,
			PriceChange: changeRatio,
		}
	}
	return result, nil
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
		&r.NBAPlayerID,
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
		&r.OverAll,
		&r.PowerPer5,
		&r.PowerPer10,
		&r.PriceChange1d,
		&r.PriceChange7d,
	)
}

// UpdatePlayerPriceChanges 更新球员涨跌幅
func (s *PlayersQuery) UpdatePlayerPriceChanges(ctx context.Context, database *DB, playerID uint, pc1d, pc7d float64) error {
	q := `UPDATE players SET price_change_1d = ?, price_change_7d = ? WHERE player_id = ?`
	_, err := database.ExecContext(ctx, q, pc1d, pc7d, playerID)
	return err
}

// GetAllTargetPlayers 获取需要更新战力值的球员（nba_player_id > 0 且不是自由球员）
func (s *PlayersQuery) GetAllTargetPlayers(ctx context.Context, database *DB) ([]*model.Players, error) {
	q := fmt.Sprintf(`SELECT %s FROM players WHERE nba_player_id > 0 AND team_abbr != '自由球员'`, selectPlayersFields)
	return queryPlayers(ctx, database, q)
}

// UpdatePlayerPower 更新球员战力值
func (s *PlayersQuery) UpdatePlayerPower(ctx context.Context, database *DB, playerID uint, power5, power10 float64) error {
	q := `UPDATE players SET power_per5 = ?, power_per10 = ? WHERE player_id = ?`
	_, err := database.ExecContext(ctx, q, power5, power10, playerID)
	return err
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
