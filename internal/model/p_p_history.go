package model

import (
	"time"
)

// PriceHistoryRow 表示 p_p_history 表的一行。
type PriceHistoryRow struct {
	PlayerId      uint      `json:"player_id"`
	AtDate        time.Time `json:"at_date"`
	AtDateHourStr string    `json:"at_date_hour"`
	AtYear        uint16    `json:"at_year"`
	AtMonth       uint8     `json:"at_month"`
	AtDay         uint8     `json:"at_day"`
	AtHour        uint8     `json:"at_hour"`
	PriceStandard uint32    `json:"price_standard"`
	PriceLower    uint32    `json:"price_lower"`
	PriceUpper    uint32    `json:"price_upper"`
}
