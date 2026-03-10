package entity

import "time"

const (
	LineupRecommendationTypeAIRecommended uint8 = 1
	LineupRecommendationTypeAvg3Baseline  uint8 = 2
	LineupRecommendationTypeAvg5Baseline  uint8 = 3
)

// LineupRecommendation 推荐阵容
type LineupRecommendation struct {
	ID                  uint      `gorm:"primaryKey;column:id"`
	GameDate            string    `gorm:"column:game_date"`
	RecommendationType  uint8     `gorm:"column:recommendation_type"`
	Rank                uint      `gorm:"column:rank"`
	TotalPredictedPower float64   `gorm:"column:total_predicted_power"`
	TotalActualPower    *float64  `gorm:"column:total_actual_power"`
	TotalSalary         uint      `gorm:"column:total_salary"`
	Player1ID           uint      `gorm:"column:player1_id"`
	Player2ID           uint      `gorm:"column:player2_id"`
	Player3ID           uint      `gorm:"column:player3_id"`
	Player4ID           uint      `gorm:"column:player4_id"`
	Player5ID           uint      `gorm:"column:player5_id"`
	DetailJSON          string    `gorm:"column:detail_json;type:json"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (LineupRecommendation) TableName() string {
	return "lineup_recommendation"
}
