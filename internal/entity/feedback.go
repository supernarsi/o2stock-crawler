package entity

import (
	"time"
)

// Feedback 反馈数据实体
type Feedback struct {
	ID         uint      `gorm:"primaryKey;column:id"`
	UID        uint      `gorm:"column:uid;default:0"`                 // 用户 id
	WxOpenID   string    `gorm:"column:wx_openid;default:''"`          // 微信 openid
	Content    string    `gorm:"column:content"`                       // 反馈内容
	AppVersion string    `gorm:"column:app_version;default:''"`        // 版本号
	IP         []byte    `gorm:"column:ip"`                            // IP 地址
	OS         uint8     `gorm:"column:os;default:1"`                  // 系统
	CTime      time.Time `gorm:"column:c_time;autoCreateTime"`         // 反馈时间
}

// TableName returns the table name for GORM.
func (Feedback) TableName() string {
	return "feedback"
}
