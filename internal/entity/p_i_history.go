package entity

import "time"

// ItemPriceHistory 道具价格历史实体，对应表 p_i_history（无涨跌幅字段，仅 price_standard、price_current_sale）
type ItemPriceHistory struct {
	ID               uint      `gorm:"primaryKey;column:id"`
	ItemID           uint      `gorm:"column:item_id;index:idx_pid"`
	AtDate           time.Time `gorm:"column:at_date"`
	AtDateHour       string    `gorm:"column:at_date_hour;uniqueIndex:idx_dh_pid,priority:1"`
	AtYear           string    `gorm:"column:at_year"`
	AtMonth          string    `gorm:"column:at_month"`
	AtDay            string    `gorm:"column:at_day"`
	AtHour           string    `gorm:"column:at_hour"`
	AtMinute         string    `gorm:"column:at_minute"`
	PriceStandard    uint      `gorm:"column:price_standard"`
	PriceCurrentSale uint      `gorm:"column:price_current_sale"` // 市场最低售价
	CTime            time.Time `gorm:"column:c_time;autoCreateTime"`
}

// TableName 表名
func (ItemPriceHistory) TableName() string {
	return "p_i_history"
}
