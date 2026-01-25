package models

import (
	"time"
)

// UserPlayerFav 用户收藏球员
type UserPlayerFav struct {
	ID       uint      `gorm:"primaryKey;column:id"`
	UserID   uint      `gorm:"column:uid;uniqueIndex:idx_uid_pid,priority:1"`
	PlayerID uint      `gorm:"column:pid;uniqueIndex:idx_uid_pid,priority:2"`
	CTime    time.Time `gorm:"column:c_time;autoCreateTime"`
}

// TableName returns the table name for GORM.
func (UserPlayerFav) TableName() string {
	return "u_p_fav"
}

// UserPlayerOwn 用户持有球员
type UserPlayerOwn struct {
	ID        uint       `gorm:"primaryKey;column:id"`
	UserID    uint       `gorm:"column:uid;index:idx_uid"`
	PlayerID  uint       `gorm:"column:pid;index:idx_pid"`
	BuyPrice  uint       `gorm:"column:price_in"`
	BuyCount  uint       `gorm:"column:num_in"`
	SellPrice uint       `gorm:"column:price_out"`
	SellCount uint       `gorm:"column:num_out;default:0"` // Existing table doesn't have num_out? Let me check
	Sta       int        `gorm:"column:own_sta"`           // 1:持有 2:已售出
	BuyTime   time.Time  `gorm:"column:dt_in"`
	SellTime  *time.Time `gorm:"column:dt_out"`
	CTime     time.Time  `gorm:"column:c_time;autoCreateTime"` // Note: c_time might not exist in old table, but let's keep it if we want to add it.
}

// TableName returns the table name for GORM.
func (UserPlayerOwn) TableName() string {
	return "u_p_own"
}
