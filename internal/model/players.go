package model

// Players 表示 players 表的一行，用于前端展示。
type Players struct {
	// Id                uint   `json:"id"`
	PlayerID          uint   `json:"player_id"`
	ShowName          string `json:"p_name_show"`
	EnName            string `json:"p_name_en"`
	TeamAbbr          string `json:"team_abbr"`
	Version           uint   `json:"version"`
	CardType          uint   `json:"card_type"`
	PlayerImg         string `json:"player_img"`
	PriceStandard     uint   `json:"price_standard"`
	PriceCurrentLower uint   `json:"price_current_lowest"`
	PriceSaleLower    uint   `json:"price_sale_lower"`
	PriceSaleUpper    uint   `json:"price_sale_upper"`
}
