package api

// NBALineupsRes NBA 推荐阵容列表响应
type NBALineupsRes struct {
	Today   *NBALineupDay  `json:"today"`
	History []NBALineupDay `json:"history"`
}

// NBALineupDay 单日的 NBA 推荐与回测数据
type NBALineupDay struct {
	GameDate    string          `json:"game_date"`
	AIRecommend []NBALineupItem `json:"ai_recommend"` // AI推荐阵容 TopN
	ActualBest  []NBALineupItem `json:"actual_best"`  // 真实最佳阵容 TopN (仅历史记录有)
}

// NBALineupItem 单个阵容信息
type NBALineupItem struct {
	Rank                uint               `json:"rank"`
	TotalPredictedPower float64            `json:"total_predicted_power"` // 对于实际最佳阵容，此值为 0
	TotalActualPower    float64            `json:"total_actual_power"`
	TotalSalary         uint               `json:"total_salary"`
	Detail              []*NBALineupPlayer `json:"detail"` // 具体的球员详情
}

// NBALineupPlayer 单个阵容中的展示球员信息
type NBALineupPlayer struct {
	NBAPlayerID    uint    `json:"nba_player_id"`
	Name           string  `json:"name"`
	Team           string  `json:"team"`
	Salary         uint    `json:"salary"`
	AvgPower       float64 `json:"avg_power"`
	PredictedPower float64 `json:"predicted_power"`
	ActualPower    float64 `json:"actual_power"`
	Avatar         string  `json:"avatar"`
	Available      float64 `json:"available"`
}
