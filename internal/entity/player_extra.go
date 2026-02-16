package entity

// PlayerExtra 球员信息扩展表
type PlayerExtra struct {
	ID             uint    `gorm:"primaryKey;column:id"`
	PlayerID       uint    `gorm:"column:player_id;uniqueIndex"`
	PNameCn        string  `gorm:"column:p_name_cn"`
	Height         uint    `gorm:"column:height"`
	Wingspan       uint    `gorm:"column:wingspan"`
	Weight         float64 `gorm:"column:weight"`
	Birthday       string  `gorm:"column:birthday"`
	Pos            string  `gorm:"column:pos"`
	Team           string  `gorm:"column:team"`
	JerseyNumber   uint    `gorm:"column:jersey_number"`
	BadgesHof      uint    `gorm:"column:badges_hof"`
	BadgesGold     uint    `gorm:"column:badges_gold"`
	BadgesSilver   uint    `gorm:"column:badges_silver"`
	BadgesBronze   uint    `gorm:"column:badges_bronze"`
	BadgesTrained  uint    `gorm:"column:badges_trained"`
	Overall        uint    `gorm:"column:overall"`
	OverallTrained uint    `gorm:"column:overall_trained"`
	ScoreAth       uint    `gorm:"column:score_ath"`
	ScoreBrk       uint    `gorm:"column:score_brk"`
	ScoreIns       uint    `gorm:"column:score_ins"`
	ScoreBak       uint    `gorm:"column:score_bak"`
	ScoreMds       uint    `gorm:"column:score_mds"`
	ScoreTps       uint    `gorm:"column:score_tps"`
	ScorePlm       uint    `gorm:"column:score_plm"`
	ScoreDfi       uint    `gorm:"column:score_dfi"`
	ScoreDfo       uint    `gorm:"column:score_dfo"`
	ScoreStl       uint    `gorm:"column:score_stl"`
	ScoreReb       uint    `gorm:"column:score_reb"`
}

// TableName returns the table name for GORM.
func (PlayerExtra) TableName() string {
	return "player_extra"
}
