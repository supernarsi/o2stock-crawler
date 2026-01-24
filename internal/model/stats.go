package model

import "time"

// PlayerSeasonStats 球员赛季平均数据
type PlayerSeasonStats struct {
	ID                   uint      `json:"id"`
	PlayerID             uint      `json:"player_id"`
	PlayerName           string    `json:"player_name"`
	Season               string    `json:"season"`
	SeasonType           int       `json:"season_type"`
	GamesPlayed          uint      `json:"games_played"`
	Minutes              float64   `json:"minutes"`
	Points               float64   `json:"points"`
	Rebounds             float64   `json:"rebounds"`
	ReboundsOffensive    float64   `json:"rebounds_offensive"`
	ReboundsDefensive    float64   `json:"rebounds_defensive"`
	Assists              float64   `json:"assists"`
	Turnovers            float64   `json:"turnovers"`
	Steals               float64   `json:"steals"`
	Blocks               float64   `json:"blocks"`
	Fouls                float64   `json:"fouls"`
	FieldGoalPercentage  float64   `json:"field_goal_percentage"`
	ThreePointPercentage float64   `json:"three_point_percentage"`
	FreeThrowPercentage  float64   `json:"free_throw_percentage"`
	UpdatedAt            time.Time `json:"updated_at"`
	CreatedAt            time.Time `json:"created_at"`
}

// PlayerGameStats 球员单场比赛数据
type PlayerGameStats struct {
	ID                     uint      `json:"id"`
	PlayerID               uint      `json:"player_id"`
	GameID                 string    `json:"game_id"`
	GameDate               time.Time `json:"game_date"`
	PlayerTeamName         string    `json:"player_team_name"`
	VsTeamName             string    `json:"vs_team_name"`
	IsHome                 bool      `json:"is_home"`
	Points                 uint      `json:"points"`
	Rebounds               uint      `json:"rebounds"`
	Assists                uint      `json:"assists"`
	Steals                 uint      `json:"steals"`
	Blocks                 uint      `json:"blocks"`
	Turnovers              uint      `json:"turnovers"`
	Minutes                uint      `json:"minutes"`
	FieldGoalsMade         uint      `json:"field_goals_made"`
	FieldGoalsAttempted    uint      `json:"field_goals_attempted"`
	ThreePointersMade      uint      `json:"three_pointers_made"`
	ThreePointersAttempted uint      `json:"three_pointers_attempted"`
	FreeThrowsMade         uint      `json:"free_throws_made"`
	FreeThrowsAttempted    uint      `json:"free_throws_attempted"`
	CreatedAt              time.Time `json:"created_at"`
}
