package db

import (
	"context"
	"o2stock-crawler/internal/model"
)

// ListPlayers 返回简单的球员列表，按 player_id 排序，可分页。
func ListPlayers(ctx context.Context, database *DB, limit, offset int) ([]*model.Players, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	const q = `
SELECT player_id, p_name_show, p_name_en, team_abbr, version, card_type,
       player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper
FROM players
LIMIT ? OFFSET ?`

	rows, err := database.QueryContext(ctx, q, limit, offset)
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

// GetPlayersByIDs 根据球员 ID 列表获取球员信息
func GetPlayersByIDs(ctx context.Context, database *DB, playerIDs []uint) ([]*model.Players, error) {
	if len(playerIDs) == 0 {
		return []*model.Players{}, nil
	}

	// 构建 IN 查询的占位符
	placeholders := ""
	args := make([]interface{}, 0, len(playerIDs))
	for i, pid := range playerIDs {
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
