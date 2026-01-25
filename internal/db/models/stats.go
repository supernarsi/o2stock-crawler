package models

import (
	"time"
)

// PlayerSeasonStats 球员赛季统计模型
type PlayerSeasonStats struct {
	ID                   uint      `gorm:"primaryKey;column:id"`
	PlayerID             uint      `gorm:"column:player_id"`
	PlayerName           string    `gorm:"column:player_name"`
	Season               string    `gorm:"column:season"`
	SeasonType           int       `gorm:"column:season_type"`
	GamesPlayed          int       `gorm:"column:games_played"`
	Minutes              float64   `gorm:"column:minutes"`
	Points               float64   `gorm:"column:points"`
	Rebounds             float64   `gorm:"column:rebounds"`
	ReboundsOffensive    float64   `gorm:"column:rebounds_offensive"`
	ReboundsDefensive    float64   `gorm:"column:rebounds_defensive"`
	Assists              float64   `gorm:"column:assists"`
	Turnovers            float64   `gorm:"column:turnovers"`
	Steals               float64   `gorm:"column:steals"`
	Blocks               float64   `gorm:"column:blocks"`
	Fouls                float64   `gorm:"column:fouls"`
	FieldGoalPercentage  float64   `gorm:"column:field_goal_percentage"`
	ThreePointPercentage float64   `gorm:"column:three_point_percentage"`
	FreeThrowPercentage  float64   `gorm:"column:free_throw_percentage"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (PlayerSeasonStats) TableName() string {
	return "player_season_stats"
}

// PlayerGameStats 球员比赛统计模型
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
