package entity

import "time"

// UserItemOwn 用户持有道具实体（u_i_own）
type UserItemOwn struct {
	ID         uint       `gorm:"primaryKey;column:id"`
	UserID     uint       `gorm:"column:uid;index:idx_uid"`
	ItemID     uint       `gorm:"column:item_id;index:idx_item"`
	BuyPrice   uint       `gorm:"column:price_in"`  // 购买总价
	SellPrice  uint       `gorm:"column:price_out"` // 出售总价
	BuyCount   uint       `gorm:"column:num_in"`    // 购买数量
	Sta        int        `gorm:"column:own_sta"`   // consts: OwnStaPurchased/OwnStaSold
	BuyTime    time.Time  `gorm:"column:dt_in"`
	SellTime   *time.Time `gorm:"column:dt_out"`
	NotifyType uint8      `gorm:"column:notify_type"` // 0:不订阅 1:回本 2:盈利15%
	NotifyTime *time.Time `gorm:"column:notify_time"`
}

func (UserItemOwn) TableName() string {
	return "u_i_own"
}
