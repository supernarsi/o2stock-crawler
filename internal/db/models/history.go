package models

import (
	"time"
)

// PlayerPriceHistory 球员价格历史模型
type PlayerPriceHistory struct {
	ID               uint      `gorm:"primaryKey;column:id"`
	PlayerID         uint      `gorm:"column:player_id;index:idx_pid"`
	AtDate           time.Time `gorm:"column:at_date"`
	AtDateHour       string    `gorm:"column:at_date_hour;uniqueIndex:idx_dh_pid,priority:1"`
	AtYear           string    `gorm:"column:at_year"`
	AtMonth          string    `gorm:"column:at_month"`
	AtDay            string    `gorm:"column:at_day"`
	AtHour           string    `gorm:"column:at_hour"`
	AtMinute         string    `gorm:"column:at_minute"`
	PriceStandard    uint      `gorm:"column:price_standard"`
	PriceCurrentSale int       `gorm:"column:price_current_sale"`
	PriceLower       uint      `gorm:"column:price_lower"`
	PriceUpper       uint      `gorm:"column:price_upper"`
	CTime            time.Time `gorm:"column:c_time;autoCreateTime"`
}

// TableName returns the table name for GORM.
func (PlayerPriceHistory) TableName() string {
	return "p_p_history"
}
