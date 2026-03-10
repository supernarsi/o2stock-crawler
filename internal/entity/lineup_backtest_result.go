package entity

import "time"

const (
	LineupBacktestResultTypeRecommendedActual uint8 = 1
	LineupBacktestResultTypeActualOptimal     uint8 = 2
	LineupBacktestResultTypeAvg3Benchmark     uint8 = 3
	LineupBacktestResultTypeAvg5Benchmark     uint8 = 4
)

// LineupBacktestResult 回测结果（推荐阵容实得 / 基准阵容 / 实际最优阵容）
type LineupBacktestResult struct {
	ID               uint      `gorm:"primaryKey;column:id"`
	GameDate         string    `gorm:"column:game_date"`
	ResultType       uint8     `gorm:"column:result_type"`
	Rank             uint      `gorm:"column:rank"`
	TotalActualPower float64   `gorm:"column:total_actual_power"`
	TotalSalary      uint      `gorm:"column:total_salary"`
	Player1ID        uint      `gorm:"column:player1_id"`
	Player2ID        uint      `gorm:"column:player2_id"`
	Player3ID        uint      `gorm:"column:player3_id"`
	Player4ID        uint      `gorm:"column:player4_id"`
	Player5ID        uint      `gorm:"column:player5_id"`
	DetailJSON       string    `gorm:"column:detail_json;type:json"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (LineupBacktestResult) TableName() string {
	return "lineup_backtest_result"
}
