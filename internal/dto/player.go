// Package dto contains Data Transfer Objects for API responses.
// These are used for JSON serialization in HTTP responses.
package dto

// Players 球员API响应DTO
type Players struct {
	PlayerID          uint    `json:"player_id"`
	TxPlayerID        uint    `json:"tx_player_id"`
	ShowName          string  `json:"p_name_show"`
	EnName            string  `json:"p_name_en"`
	TeamAbbr          string  `json:"team_abbr"`
	Age               uint    `json:"age"`
	Version           uint    `json:"version"`
	CardType          uint    `json:"card_type"`
	PlayerImg         string  `json:"player_img"`
	PriceStandard     uint    `json:"price_standard"`
	PriceCurrentLower uint    `json:"price_current_lowest"`
	PriceSaleLower    uint    `json:"price_sale_lower"`
	PriceSaleUpper    uint    `json:"price_sale_upper"`
	OverAll           uint    `json:"over_all"`
	PowerPer5         float64 `json:"power_per5"`
	PowerPer10        float64 `json:"power_per10"`
	PriceChange1d     float64 `json:"price_change_1d"`
	PriceChange7d     float64 `json:"price_change_7d"`
	UpdatedAt         string  `json:"update_at"`
}

// PlayerWithPriceChange 带涨跌幅的球员DTO
type PlayerWithPriceChange struct {
	Players
	PriceChange float64 `json:"price_change"`
}

// PlayerPriceChange 球员涨跌变化DTO
type PlayerPriceChange struct {
	PlayerID    uint    `json:"player_id"`
	PriceOld    uint    `json:"price_old"`
	PriceNow    uint    `json:"price_now"`
	ChangeRatio float64 `json:"change_ratio"`
}

// PlayerInvestmentStats 球员投资盈亏统计（全平台 u_p_own + players 聚合）
type PlayerInvestmentStats struct {
	PlayerID           uint    `json:"player_id"`
	TotalCost          uint64  `json:"total_cost"`           // 总成本（所有仓位 price_in 之和）
	TotalRealizedPnl   int64   `json:"total_realized_pnl"`   // 已实现盈亏（已售：price_out - price_in 之和）
	TotalUnrealizedPnl int64   `json:"total_unrealized_pnl"` // 未实现盈亏（持有：当前市值 - price_in 之和）
	TotalPnl           int64   `json:"total_pnl"`            // 总盈亏 = 已实现 + 未实现
	PositionCount      int     `json:"position_count"`       // 仓位笔数（含持有+已售）
	ProfitCount        int     `json:"profit_count"`         // 盈利笔数（单笔 pnl > 0）
	BullBearRate       float64 `json:"bull_bear_rate"`       // 牛熊率 0~1：1=全赚，0=全亏，0.5=中性（按金额：盈利/(盈利+亏损)）
}
