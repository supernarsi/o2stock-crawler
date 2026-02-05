package entity

import "time"

// Item 道具实体，对应表 items
type Item struct {
	ID                 uint      `gorm:"primaryKey;column:id"`
	ItemID             uint      `gorm:"column:item_id;uniqueIndex"`
	Name               string    `gorm:"column:name"`
	Desc               string    `gorm:"column:desc"` // 与 MySQL 关键字冲突时用 column 指定
	Icon               string    `gorm:"column:icon"`
	PriceStandard      uint      `gorm:"column:price_standard"`
	PriceCurrentLowest uint      `gorm:"column:price_current_lowest"`
	PriceChange1d      float64   `gorm:"column:price_change_1d"`
	PriceChange7d      float64   `gorm:"column:price_change_7d"`
	UpdatedAt          time.Time `gorm:"column:update_at;autoUpdateTime"`
}

// TableName 表名
func (Item) TableName() string {
	return "items"
}
