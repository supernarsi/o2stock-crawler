package entity

// PlayerBadge 球员徽章数据表
type PlayerBadge struct {
	ID        uint  `gorm:"primaryKey;column:id"`
	PlayerID  uint  `gorm:"column:player_id"`
	BadgeID   uint  `gorm:"column:badge_id"`
	Lv        uint8 `gorm:"column:lv"` // 徽章等级：0.个性徽章；1.铜徽章；2.银徽章；3.金徽章；4.紫徽章
	IsTrained bool  `gorm:"column:is_trained"`
}

// TableName returns the table name for GORM.
func (PlayerBadge) TableName() string {
	return "player_badge"
}
