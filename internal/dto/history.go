package dto

import "time"

// PriceHistoryRow 价格历史行DTO
type PriceHistoryRow struct {
	PlayerId         uint      `json:"player_id"`
	AtDate           time.Time `json:"at_date"`
	AtDateHourStr    string    `json:"at_date_hour"`
	AtYear           uint16    `json:"at_year"`
	AtMonth          uint8     `json:"at_month"`
	AtDay            uint8     `json:"at_day"`
	AtHour           uint8     `json:"at_hour"`
	AtMinute         uint8     `json:"at_minute"`
	PriceStandard    uint32    `json:"price_standard"`
	PriceCurrentSale int32     `json:"price_current_sale"`
	PriceLower       uint32    `json:"price_lower"`
	PriceUpper       uint32    `json:"price_upper"`
}

// PriceHistoryMap 球员历史价格 map
type PriceHistoryMap map[uint]*PriceHistoryRow
