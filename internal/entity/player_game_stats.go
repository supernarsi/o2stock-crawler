package entity

import (
	"time"
)

// PlayerGameStats 球员比赛统计实体模型
type PlayerGameStats struct {
	ID                     uint      `gorm:"primaryKey;column:id"`
	PlayerID               uint      `gorm:"column:player_id"`
	GameID                 string    `gorm:"column:game_id"`
	GameDate               time.Time `gorm:"column:game_date"`
	PlayerTeamName         string    `gorm:"column:player_team_name"`
	VsTeamName             string    `gorm:"column:vs_team_name"`
	IsHome                 bool      `gorm:"column:is_home"`
	Points                 int       `gorm:"column:points"`
	Rebounds               int       `gorm:"column:rebounds"`
	Assists                int       `gorm:"column:assists"`
	Steals                 int       `gorm:"column:steals"`
	Blocks                 int       `gorm:"column:blocks"`
	Turnovers              int       `gorm:"column:turnovers"`
	Minutes                int       `gorm:"column:minutes"`
	FieldGoalsMade         int       `gorm:"column:field_goals_made"`
	FieldGoalsAttempted    int       `gorm:"column:field_goals_attempted"`
	ThreePointersMade      int       `gorm:"column:three_pointers_made"`
	ThreePointersAttempted int       `gorm:"column:three_pointers_attempted"`
	FreeThrowsMade         int       `gorm:"column:free_throws_made"`
	FreeThrowsAttempted    int       `gorm:"column:free_throws_attempted"`
	CreatedAt              time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (PlayerGameStats) TableName() string {
	return "player_game_stats"
}
