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
