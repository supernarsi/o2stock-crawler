package entity

import (
	"time"
)

// UserPlayerFav 用户收藏球员实体
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
