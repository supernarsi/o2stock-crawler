package entity

import (
	"time"
)

// UserItemFav 用户收藏道具实体
type UserItemFav struct {
	ID     uint      `gorm:"primaryKey;column:id"`
	UserID uint      `gorm:"column:uid;uniqueIndex:idx_uid_item,priority:1"`
	ItemID uint      `gorm:"column:item_id;uniqueIndex:idx_uid_item,priority:2"`
	CTime  time.Time `gorm:"column:c_time;autoCreateTime"`
}

// TableName returns the table name for GORM.
func (UserItemFav) TableName() string {
	return "u_i_fav"
}
