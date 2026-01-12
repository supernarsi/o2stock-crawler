package db

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/model"
	"time"
)

// PlayersQuery 获取球员列表
type PlayersQuery struct {
	QueryBase
}

// NewPlayersQuery 创建一个 PlayersQuery
func NewPlayersQuery(page, limit int, orderBy string, orderAsc bool) *PlayersQuery {
	orderDir := OrderAsc
	if !orderAsc {
		orderDir = OrderDesc
	}
	// 限制排序字段
	if orderBy != "price_change" {
		orderBy = "player_id"
	}
	return &PlayersQuery{
		QueryBase: QueryBase{
			limit:   limit,
			offset:  (page - 1) * limit,
			orderBy: NewOrderBy(orderBy, orderDir),
		},
	}
}

// ListPlayers 返回简单的球员列表，按 player_id 排序，可分页。
func (s *PlayersQuery) ListPlayers(ctx context.Context, database *DB, period uint8, orderBy string, orderAsc bool) ([]*model.PlayerWithPriceChange, error) {
	// 支持按价格排序 和 按涨跌幅排序
	switch orderBy {
	case "price_standard":
		// 1. 如果按价格排序，直接插叙 players 表，按 price_standard 排序，再查 p_p_history 表计算涨跌幅
		return s.queryPlayersOrderByPrice(ctx, database, period, orderAsc), nil
	case "price_change":
		// 2. 如果按涨跌幅排序，先使用窗口函数从 p_p_history 表中获取按涨跌幅排序后的球员 id，再使用 in 查询从 players 表中获取球员信息
	default:
		return s.queryPlayersOrderByPriceRatio(ctx, database, period, orderAsc), nil
	}

	return nil, nil
}

// 按价格排序查询球员价格
func (s *PlayersQuery) queryPlayersOrderByPrice(ctx context.Context, database *DB, period uint8, orderAsc bool) []*model.PlayerWithPriceChange {
	orderDir := "ASC"
	if !orderAsc {
		orderDir = "DESC"
	}
	q := fmt.Sprintf(`SELECT * FROM players ORDER BY price_standard %s LIMIT %d OFFSET %d`, orderDir, s.limit, s.offset)
	rows, err := database.QueryContext(ctx, q)
	if err != nil {
		// todo: 记录错误日志
		return nil
	}
	defer rows.Close()

	var result []*model.Players
	var playerIds []uint
	for rows.Next() {
		var r model.Players
		if err := rows.Scan(
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
		); err != nil {
			// todo: 记录错误日志
			return nil
		}
		result = append(result, &r)
		playerIds = append(playerIds, r.PlayerID)
	}

	// 查询涨跌幅数据
	sTime := time.Now()
	switch period {
	case 3:
		// 1 周数据涨跌幅
		sTime = sTime.AddDate(0, 0, -7)
	case 2:
		// 3 天数据涨跌幅
		sTime = sTime.AddDate(0, 0, -3)
	default:
		// 24 小时数据涨跌幅
		sTime = sTime.AddDate(0, 0, -1)
	}
	priceRatio := s.queryPlayersRatio(ctx, database, playerIds, sTime, orderAsc, s.limit, s.offset)

	// 组装成 []*model.Players 数据
	res := s.mergePlayersPriceChange(result, priceRatio)
	return res
}

// 按涨跌幅排序查询球员价格变动
func (s *PlayersQuery) queryPlayersOrderByPriceRatio(ctx context.Context, database *DB, period uint8, orderAsc bool) []*model.PlayerWithPriceChange {
	sTime := time.Now()
	switch period {
	case 3:
		// 1 周数据涨跌幅
		sTime = sTime.AddDate(0, 0, -7)
	case 2:
		// 3 天数据涨跌幅
		sTime = sTime.AddDate(0, 0, -3)
	default:
		// 24 小时数据涨跌幅
		sTime = sTime.AddDate(0, 0, -1)
	}

	// 查询涨跌幅数据
	priceRatio := s.queryPlayersRatio(ctx, database, []uint{}, sTime, orderAsc, s.limit, s.offset)
	// 获取 player_ids
	playerIds := []uint{}
	for _, ratio := range priceRatio {
		playerIds = append(playerIds, ratio.PlayerID)
	}

	// 查询 player_ids 对应的球员数据
	players, _ := NewPlayersByIDsQuery(playerIds).GetPlayersByIDs(ctx, database)

	// 组装成 []*model.Players 数据
	res := s.mergePlayersPriceChange(players, priceRatio)
	return res
}

// queryPlayersRatio 按涨幅排序查询球员价格变动
func (s *PlayersQuery) queryPlayersRatio(ctx context.Context, database *DB, playersIds []uint, sTime time.Time, orderAsc bool, limit, offset int) []*model.PlayerPriceChange {
	// 使用窗口函数，先从 p_p_history 表中获取按涨跌幅排序后的球员 id，再使用 in 查询从 players 表中获取球员信息
	orderDir := "ASC"
	if !orderAsc {
		orderDir = "DESC"
	}
	// 默认查询 3 天价格变化
	atDateHour := sTime.Format("200601021504")
	playersIdsStr, limitStr := "", ""

	// 构建 players_ids 字符串
	if len(playersIds) > 0 {
		for _, playerId := range playersIds {
			playersIdsStr += fmt.Sprintf("%d,", playerId)
		}
		playersIdsStr = "AND player_id IN (" + playersIdsStr + ")"
	}
	if limit > 0 {
		limitStr = fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	}

	q := fmt.Sprintf(`WITH recent_data AS (
  SELECT
    player_id, at_date_hour, price_standard,
    ROW_NUMBER() OVER ( PARTITION BY player_id ORDER BY at_date_hour DESC) AS rn_desc,
    ROW_NUMBER() OVER ( PARTITION BY player_id ORDER BY at_date_hour ASC) AS rn_asc
  FROM p_p_history
  WHERE at_date_hour >= %s
  %s
)
SELECT
  cur.player_id,
  old.price_standard AS price_old,
  cur.price_standard AS price_now,
  (CAST(cur.price_standard AS SIGNED) - CAST(old.price_standard AS SIGNED)) / old.price_standard AS price_ratio
FROM recent_data cur
JOIN recent_data old  ON cur.player_id = old.player_id
WHERE cur.rn_desc = 1
  AND old.rn_asc = 1
  AND old.price_standard > 0
ORDER BY price_ratio %s 
%s;`, atDateHour, playersIdsStr, orderDir, limitStr)

	// log.Println("query: ", q)

	rows, err := database.QueryContext(ctx, q)
	if err != nil {
		// todo: 记录错误日志
		return nil
	}
	defer rows.Close()

	var result []*model.PlayerPriceChange
	for rows.Next() {
		var r model.PlayerPriceChange
		if err := rows.Scan(
			&r.PlayerID,
			&r.PriceOld,
			&r.PriceNow,
			&r.ChangeRatio,
		); err != nil {
			return nil
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return result
}

func (s *PlayersQuery) mergePlayersPriceChange(players []*model.Players, priceChange []*model.PlayerPriceChange) []*model.PlayerWithPriceChange {
	res := []*model.PlayerWithPriceChange{}
	playersMap := make(map[uint]*model.Players)
	for _, p := range players {
		playersMap[p.PlayerID] = p
	}
	for _, pRation := range priceChange {
		if playerInfo, ok := playersMap[pRation.PlayerID]; !ok || playerInfo == nil {
			continue
		} else {
			res = append(res, &model.PlayerWithPriceChange{Players: *playerInfo, PriceChange: pRation.ChangeRatio})
		}
	}
	return res
}

// PlayersByIDsQuery 根据球员 ID 列表获取球员信息
type PlayersByIDsQuery struct {
	QueryBase
	playerIDs []uint
}

// NewPlayersByIDsQuery 创建一个 PlayersByIDsQuery
func NewPlayersByIDsQuery(playerIDs []uint) *PlayersByIDsQuery {
	return &PlayersByIDsQuery{
		QueryBase: QueryBase{},
		playerIDs: playerIDs,
	}
}

// GetPlayersByIDs 根据球员 ID 列表获取球员信息
func (s *PlayersByIDsQuery) GetPlayersByIDs(ctx context.Context, database *DB) ([]*model.Players, error) {
	if len(s.playerIDs) == 0 {
		return []*model.Players{}, nil
	}

	// 构建 IN 查询的占位符
	placeholders := ""
	args := make([]interface{}, 0, len(s.playerIDs))
	for i, pid := range s.playerIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, pid)
	}

	q := `SELECT 
	player_id, 
	p_name_show, 
	p_name_en, 
	team_abbr, 
	version, 
	card_type,
	player_img, 
	price_standard, 
	price_current_lowest, 
	price_sale_lower, 
	price_sale_upper
FROM players
WHERE player_id IN (` + placeholders + `)`

	rows, err := database.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Players
	for rows.Next() {
		var r model.Players
		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
