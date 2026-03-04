package entity

import "time"

// NBAGamePlayerActual 球员真实战力反馈
type NBAGamePlayerActual struct {
	ID          uint      `gorm:"primaryKey;column:id"`
	GameDate    string    `gorm:"column:game_date"`
	Rank        uint      `gorm:"column:rank"`
	NBAPlayerID uint      `gorm:"column:nba_player_id"`
	Salary      uint      `gorm:"column:salary"`
	ActualPower float64   `gorm:"column:actual_power"`
	Source      string    `gorm:"column:source"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (NBAGamePlayerActual) TableName() string {
	return "nba_game_player_actual"
}
