package entity

import "time"

// PlayerIPI 球员 IPI 计算结果快照（对应表 player_ipi）
type PlayerIPI struct {
	ID                 uint      `gorm:"primaryKey;column:id"`
	PlayerID           uint      `gorm:"column:player_id;index:idx_player_id"`
	IPI                float64   `gorm:"column:ipi"`
	SPerf              float64   `gorm:"column:s_perf"`
	VGap               float64   `gorm:"column:v_gap"`
	MGrowth            float64   `gorm:"column:m_growth"`
	RRisk              float64   `gorm:"column:r_risk"`
	MeetsTaxSafeMargin bool      `gorm:"column:meets_tax_safe_margin"`
	RankInversionIndex float64   `gorm:"column:rank_inversion_index"`
	CalculatedAt       time.Time `gorm:"column:calculated_at;index:idx_calculated_at"`
}

// TableName returns the table name for GORM.
func (PlayerIPI) TableName() string {
	return "player_ipi"
}
