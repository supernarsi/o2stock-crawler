package dto

import "time"

// Item 道具 API 响应 DTO
type Item struct {
	ItemID             uint       `json:"item_id"`
	Name               string     `json:"name"`
	Desc               string     `json:"desc"`
	Icon               string     `json:"icon"`
	PriceStandard      uint       `json:"price_standard"`
	PriceCurrentLowest uint       `json:"price_current_lowest"`
	PriceChange1d      float64    `json:"price_change_1d"`
	PriceChange7d      float64    `json:"price_change_7d"`
	Owned              []*OwnInfo `json:"owned"`
	IsFav              bool       `json:"is_fav"`
}

// ItemPriceHistoryRow 道具价格历史行 DTO
type ItemPriceHistoryRow struct {
	ItemID           uint      `json:"item_id"`
	AtDate           time.Time `json:"at_date"`
	AtDateHourStr    string    `json:"at_date_hour"`
	PriceStandard    uint32    `json:"price_standard"`
	PriceCurrentSale uint32    `json:"price_current_sale"`
}
