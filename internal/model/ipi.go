package model

// PricePercentileResult 单球员历史价格分位数结果（用于 R_risk 价格饱和度等）
type PricePercentileResult struct {
	P75 uint // 75 分位价格
	P90 uint // 90 分位价格
	// HasEnoughData 是否有足够历史数据（如近 90 天内至少有一条）
	HasEnoughData bool
}

// IPIResult 单球员 IPI 计算结果，供排序、筛选与 API 输出
type IPIResult struct {
	PlayerID           uint    `json:"player_id"`
	IPI                float64 `json:"ipi"`                   // 综合 IPI 分数
	SPerf              float64 `json:"s_perf"`                // 表现盈余分（原始值）
	VGap               float64 `json:"v_gap"`                 // 价值洼地分（原始值）
	MGrowth            float64 `json:"m_growth"`              // 成长动能与题材分（原始值）
	RRisk              float64 `json:"r_risk"`                // 风险折现因子
	MeetsTaxSafeMargin bool    `json:"meets_tax_safe_margin"` // 税后安全边际是否满足
	RankInversionIndex float64 `json:"rank_inversion_index"`  // 能力值倒挂指数

	// 归一化后的分数（0~1 范围，便于跨球员比较）
	SPerfNorm   float64 `json:"s_perf_norm"`
	VGapNorm    float64 `json:"v_gap_norm"`
	MGrowthNorm float64 `json:"m_growth_norm"`

	// 可解释性：主要加分/扣分因素
	MainFactors []string `json:"main_factors,omitempty"` // 主要影响因素说明
}

// IPIBatchStats 批量计算时的统计信息，用于归一化
type IPIBatchStats struct {
	SPerfMin, SPerfMax     float64
	VGapMin, VGapMax       float64
	MGrowthMin, MGrowthMax float64
}
