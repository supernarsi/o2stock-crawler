package entity

import (
	"time"
)

// UserPlayerOwn 用户持有记录实体（球员/道具统一持仓表）
type UserPlayerOwn struct {
	ID         uint       `gorm:"primaryKey;column:id"`
	UserID     uint       `gorm:"column:uid;index:idx_uid"`
	OwnGoods   uint8      `gorm:"column:own_goods;index:idx_uid_own_goods"` // 持仓的类型：1.球员；2.道具
	PlayerID   uint       `gorm:"column:pid;index:idx_pid"`                 // 球员id 或 道具id
	BuyPrice   uint       `gorm:"column:price_in"`
	BuyCount   uint       `gorm:"column:num_in"`
	SellPrice  uint       `gorm:"column:price_out"`
	Sta        int        `gorm:"column:own_sta"` // 1:持有 2:已售出
	BuyTime    time.Time  `gorm:"column:dt_in"`
	SellTime   *time.Time `gorm:"column:dt_out"`
	NotifyType uint8      `gorm:"column:notify_type"` // 0:不订阅 1:回本 2:盈利15%
	NotifyTime *time.Time `gorm:"column:notify_time"`
}

// TableName returns the table name for GORM.
func (UserPlayerOwn) TableName() string {
	return "u_p_own"
}
