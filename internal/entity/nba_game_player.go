package entity

import "time"

// NBAGamePlayer 今日NBA候选球员
type NBAGamePlayer struct {
	ID             uint      `gorm:"primaryKey;column:id"`
	GameDate       string    `gorm:"column:game_date"`
	MatchID        string    `gorm:"column:match_id"`
	NBAPlayerID    uint      `gorm:"column:nba_player_id"`
	NBATeamID      string    `gorm:"column:nba_team_id"`
	PlayerName     string    `gorm:"column:player_name"`
	PlayerEnName   string    `gorm:"column:player_en_name"`
	TeamName       string    `gorm:"column:team_name"`
	IsHome         bool      `gorm:"column:is_home"`
	Salary         uint      `gorm:"column:salary"`
	CombatPower    float64   `gorm:"column:combat_power"`
	Position       uint      `gorm:"column:position"`
	PredictedPower *float64  `gorm:"column:predicted_power"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (NBAGamePlayer) TableName() string {
	return "nba_game_player"
}
