package models

import (
	"time"
)

// User 用户模型
type User struct {
	ID           uint      `gorm:"primaryKey;column:id"`
	Nick         string    `gorm:"column:nick"`
	Avatar       string    `gorm:"column:avatar"`
	WxOpenID     string    `gorm:"column:wx_openid;uniqueIndex"`
	WxUnionID    string    `gorm:"column:wx_unionid"`
	WxSessionKey string    `gorm:"column:wx_session_key"`
	Sta          int       `gorm:"column:sta"`
	CTime        time.Time `gorm:"column:c_time;autoCreateTime"`
}

// TableName returns the table name for GORM.
func (User) TableName() string {
	return "user"
}
