package entity

import (
	"time"
)

// User 用户实体模型
type User struct {
	ID           uint       `gorm:"primaryKey;column:id"`
	Nick         string     `gorm:"column:nick"`
	Avatar       string     `gorm:"column:avatar"`
	WxOpenID     string     `gorm:"column:wx_openid;uniqueIndex"`
	WxUnionID    string     `gorm:"column:wx_unionid"`
	WxSessionKey string     `gorm:"column:wx_session_key"`
	Sta          int        `gorm:"column:sta"`
	CTime        time.Time  `gorm:"column:c_time;autoCreateTime"`
	RegOS        int        `gorm:"column:reg_os"`                    // 注册时的系统：1.iOS；2.安卓；3.鸿蒙；0.未知
	RegIP        []byte     `gorm:"column:reg_ip;type:varbinary(16)"` // 注册时的 IP（IPv4/IPv6，16 字节）
	LoginTime    *time.Time `gorm:"column:login_time"`                // 最近登录时间
}

// TableName returns the table name for GORM.
func (User) TableName() string {
	return "user"
}
