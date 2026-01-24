package model

// Players 表示 players 表的一行，用于前端展示。
type Players struct {
	// Id                uint   `json:"id"`
	PlayerID          uint    `json:"player_id"`
	NBAPlayerID       uint    `json:"nba_player_id"`
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
}

type PlayerWithPriceChange struct {
	Players
	PriceChange float64 `json:"price_change"`
}

type PlayerPriceChange struct {
	PlayerID    uint    `json:"player_id"`
	PriceOld    uint    `json:"price_old"`
	PriceNow    uint    `json:"price_now"`
	ChangeRatio float64 `json:"change_ratio"`
}
