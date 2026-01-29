package entity

import "time"

// PlayerSeasonStats 球员赛季平均数据实体模型
type PlayerSeasonStats struct {
	ID                   uint      `gorm:"primaryKey;column:id"`
	TxPlayerID           uint      `gorm:"column:tx_player_id"`
	PlayerName           string    `gorm:"column:player_name"`
	Season               string    `gorm:"column:season"`
	SeasonType           int       `gorm:"column:season_type"` // 1.常规赛
	GamesPlayed          uint      `gorm:"column:games_played"`
	Minutes              float64   `gorm:"column:minutes"`
	RankMin              uint      `gorm:"column:rank_min"`
	Points               float64   `gorm:"column:points"`
	RankPts              uint      `gorm:"column:rank_pts"`
	Rebounds             float64   `gorm:"column:rebounds"`
	RankRb               uint      `gorm:"column:rank_rb"`
	ReboundsOffensive    float64   `gorm:"column:rebounds_offensive"`
	RankRbo              uint      `gorm:"column:rank_rbo"`
	ReboundsDefensive    float64   `gorm:"column:rebounds_defensive"`
	RankRbd              uint      `gorm:"column:rank_rbd"`
	Assists              float64   `gorm:"column:assists"`
	RankAst              uint      `gorm:"column:rank_ast"`
	Turnovers            float64   `gorm:"column:turnovers"`
	RankTov              uint      `gorm:"column:rank_tov"`
	Steals               float64   `gorm:"column:steals"`
	RankStl              uint      `gorm:"column:rank_stl"`
	Blocks               float64   `gorm:"column:blocks"`
	RankBlk              uint      `gorm:"column:rank_blk"`
	Fouls                float64   `gorm:"column:fouls"`
	RankPf               uint      `gorm:"column:rank_pf"`
	FieldGoalPercentage  float64   `gorm:"column:field_goal_percentage"`
	RankPct2             uint      `gorm:"column:rank_pct_2"`
	ThreePointPercentage float64   `gorm:"column:three_point_percentage"`
	RankPct3             uint      `gorm:"column:rank_pct_3"`
	FreeThrowPercentage  float64   `gorm:"column:free_throw_percentage"`
	RankPctFt            uint      `gorm:"column:rank_pct_ft"`
	UpdatedAt            time.Time `gorm:"column:updated_at;autoUpdateTime"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName returns the table name for GORM.
func (PlayerSeasonStats) TableName() string {
	return "player_season_stats"
}
