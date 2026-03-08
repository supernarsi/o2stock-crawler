package entity

import "time"

// NBAPlayerSalary 固定球员画像表，用于根据赛程自动生成每日候选池。
type NBAPlayerSalary struct {
	ID             uint      `gorm:"primaryKey;column:id;comment:主键 ID"`
	NBAPlayerID    uint      `gorm:"column:nba_player_id;not null;default:0;uniqueIndex:uk_nba_player;comment:NBA 球员 ID"`
	NBATeamID      string    `gorm:"column:nba_team_id;type:varchar(20);not null;default:'';index:idx_team;comment:NBA 球队 ID"`
	PlayerName     string    `gorm:"column:player_name;type:varchar(100);not null;default:'';comment:球员中文名"`
	PlayerEnName   string    `gorm:"column:player_en_name;type:varchar(100);not null;default:'';comment:球员英文名"`
	TeamName       string    `gorm:"column:team_name;type:varchar(50);not null;default:'';comment:球队中文名"`
	Salary         uint      `gorm:"column:salary;not null;default:0;comment:固定工资值"`
	CombatPower    float64   `gorm:"column:combat_power;type:decimal(5,1);not null;default:0;comment:固定基础战力值"`
	Position       uint      `gorm:"column:position;not null;default:0;comment:位置(0=前锋/中锋,1=后卫)"`
	SourceGameDate string    `gorm:"column:source_game_date;type:char(10);not null;default:'';comment:最近同步来源日期"`
	CreatedAt      time.Time `gorm:"column:created_at;type:datetime(3);not null;autoCreateTime;comment:数据创建时间"`
	UpdatedAt      time.Time `gorm:"column:updated_at;type:datetime(3);not null;autoUpdateTime;comment:数据更新时间"`
}

func (NBAPlayerSalary) TableName() string {
	return "nba_player_salary"
}
