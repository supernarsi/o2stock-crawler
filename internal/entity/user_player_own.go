package entity

import (
	"time"
)

// UserPlayerOwn 用户持有球员实体
type UserPlayerOwn struct {
	ID        uint       `gorm:"primaryKey;column:id"`
	UserID    uint       `gorm:"column:uid;index:idx_uid"`
	PlayerID  uint       `gorm:"column:pid;index:idx_pid"`
	BuyPrice  uint       `gorm:"column:price_in"`
	BuyCount  uint       `gorm:"column:num_in"`
	SellPrice uint       `gorm:"column:price_out"`
	Sta       int        `gorm:"column:own_sta"` // 1:持有 2:已售出
	BuyTime   time.Time  `gorm:"column:dt_in"`
	SellTime  *time.Time `gorm:"column:dt_out"`
}

// TableName returns the table name for GORM.
func (UserPlayerOwn) TableName() string {
	return "u_p_own"
}
