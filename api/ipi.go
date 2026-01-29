package api

import "time"

// IPIRankItem 单条 IPI 排行（含 p_name_show）
type IPIRankItem struct {
	PlayerID      uint    `json:"player_id"`
	PNameShow     string  `json:"p_name_show"`
	IPI           float64 `json:"ipi"`
	PriceStandard uint    `json:"price_standard"`
	// SPerf              float64   `json:"s_perf"`
	// VGap               float64   `json:"v_gap"`
	// MGrowth            float64   `json:"m_growth"`
	// RRisk              float64   `json:"r_risk"`
	// MeetsTaxSafeMargin bool      `json:"meets_tax_safe_margin"`
	// RankInversionIndex float64   `json:"rank_inversion_index"`
	// CalculatedAt       time.Time `json:"calculated_at"`
}

// IPIRankRes GET /ipi/rank 响应
type IPIRankRes struct {
	List         []IPIRankItem `json:"list"`
	CalculatedAt time.Time     `json:"calculated_at"`
	Page         int           `json:"page"`
	Limit        int           `json:"limit"`
	Total        int64         `json:"total"`
}

// IPIPlayerRes GET /ipi/player 响应（单球员，含 p_name_show）
type IPIPlayerRes struct {
	PlayerID           uint      `json:"player_id"`
	PNameShow          string    `json:"p_name_show"`
	IPI                float64   `json:"ipi"`
	SPerf              float64   `json:"s_perf"`
	VGap               float64   `json:"v_gap"`
	MGrowth            float64   `json:"m_growth"`
	RRisk              float64   `json:"r_risk"`
	MeetsTaxSafeMargin bool      `json:"meets_tax_safe_margin"`
	RankInversionIndex float64   `json:"rank_inversion_index"`
	CalculatedAt       time.Time `json:"calculated_at"`
}
