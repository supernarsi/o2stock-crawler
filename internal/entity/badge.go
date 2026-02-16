package entity

// Badge 徽章数据表
type Badge struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	BadgeID   uint   `gorm:"column:badge_id;uniqueIndex"`
	BadgeName string `gorm:"column:badge_name"`
	Desc      string `gorm:"column:desc"`
	IconName  string `gorm:"column:icon_name"`
}

// TableName returns the table name for GORM.
func (Badge) TableName() string {
	return "badges"
}
