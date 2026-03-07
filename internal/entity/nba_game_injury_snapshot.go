package entity

import "time"

// NBAGameInjurySnapshot 推荐时抓取的伤病快照，用于历史回放。
type NBAGameInjurySnapshot struct {
	ID          uint      `gorm:"primaryKey;column:id;comment:主键 ID"`
	GameDate    string    `gorm:"column:game_date;type:char(10);not null;default:'';uniqueIndex:idx_date_player,priority:1;comment:比赛日期"`
	NBAPlayerID uint      `gorm:"column:nba_player_id;not null;default:0;uniqueIndex:idx_date_player,priority:2;comment:球员 id"`
	PlayerName  string    `gorm:"column:player_name;type:varchar(50);not null;default:'';comment:球员名称"`
	TeamName    string    `gorm:"column:team_name;type:varchar(50);not null;default:'';comment:球队名称"`
	Status      string    `gorm:"column:status;type:varchar(50);not null;default:'';comment:伤病状态"`
	Description string    `gorm:"column:description;type:varchar(255);not null;default:'';comment:伤病描述"`
	ReportDate  string    `gorm:"column:report_date;type:varchar(20);not null;default:'';comment:报告伤病日期"`
	Source      string    `gorm:"column:source;type:varchar(10);not null;default:'';comment:伤病报告来源"`
	FetchedAt   time.Time `gorm:"column:fetched_at;type:datetime(3);not null;comment:抓取数据时间"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime(3);not null;autoCreateTime;comment:数据创建时间"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime(3);not null;autoUpdateTime;comment:数据更新时间"`
}

func (NBAGameInjurySnapshot) TableName() string {
	return "nba_game_injury_snapshot"
}
