package entity

import "time"

// LineupSubscribe 今日NBA阵容订阅表
type LineupSubscribe struct {
	ID           uint      `gorm:"primaryKey;column:id"`
	UserID       uint      `gorm:"column:user_id;uniqueIndex"`
	Status       uint8     `gorm:"column:status;default:1"` // 订阅状态：1 已订阅；0 已取消
	PushCount    uint      `gorm:"column:push_count;default:0"`
	LastPushTime *time.Time `gorm:"column:last_push_time"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for GORM.
func (LineupSubscribe) TableName() string {
	return "lineup_subscribe"
}
