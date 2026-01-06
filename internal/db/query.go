package db

import (
	"context"
	"time"
)

// PlayerRow 表示 players 表的一行，用于前端展示。
type PlayerRow struct {
	PlayerID          uint32 `json:"player_id"`
	ShowName          string `json:"p_name_show"`
	EnName            string `json:"p_name_en"`
	TeamAbbr          string `json:"team_abbr"`
	Version           uint32 `json:"version"`
	CardType          uint32 `json:"card_type"`
	PlayerImg         string `json:"player_img"`
	PriceStandard     uint32 `json:"price_standard"`
	PriceCurrentLower uint32 `json:"price_current_lowest"`
	PriceSaleLower    uint32 `json:"price_sale_lower"`
	PriceSaleUpper    uint32 `json:"price_sale_upper"`
}

// PriceHistoryRow 表示 p_p_history 表的一行。
type PriceHistoryRow struct {
	AtDate        time.Time `json:"at_date"`
	AtDateHourStr string    `json:"at_date_hour"`
	PriceStandard uint32    `json:"price_standard"`
	PriceLower    uint32    `json:"price_lower"`
	PriceUpper    uint32    `json:"price_upper"`
}

// ListPlayers 返回简单的球员列表，按 player_id 排序，可分页。
func ListPlayers(ctx context.Context, database *DB, limit, offset int) ([]PlayerRow, error) {
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
ORDER BY player_id ASC
LIMIT ? OFFSET ?`

	rows, err := database.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PlayerRow
	for rows.Next() {
		var r PlayerRow
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
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPlayerHistory 返回某个球员的历史价格，按时间升序。
func GetPlayerHistory(ctx context.Context, database *DB, playerID uint32, limit int) ([]PriceHistoryRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	const q = `
SELECT at_date, at_date_hour, price_standard, price_lower, price_upper
FROM p_p_history
WHERE player_id = ?
ORDER BY at_date, at_hour
LIMIT ?`

	rows, err := database.QueryContext(ctx, q, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PriceHistoryRow
	for rows.Next() {
		var r PriceHistoryRow
		if err := rows.Scan(&r.AtDate, &r.AtDateHourStr, &r.PriceStandard, &r.PriceLower, &r.PriceUpper); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
