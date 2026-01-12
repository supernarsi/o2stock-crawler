package db

import (
	"context"
	"log"
	"o2stock-crawler/internal/model"
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
func (s *PlayersQuery) ListPlayers(ctx context.Context, database *DB) ([]*model.Players, error) {
	q := `
SELECT player_id, p_name_show, p_name_en, team_abbr, version, card_type, player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper, price_change
FROM players
ORDER BY ` + s.orderBy.GetOrderByClause() + `
LIMIT ? OFFSET ?`

	log.Printf("query: %s", q)
	log.Printf("parsed orderBy: %s, limit: %d, offset: %d", s.orderBy.GetOrderByClause(), s.limit, s.offset)
	rows, err := database.QueryContext(ctx, q, s.limit, s.offset)
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
			&r.PriceChange,
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

	q := `
SELECT player_id, p_name_show, p_name_en, team_abbr, version, card_type,
       player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper
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
