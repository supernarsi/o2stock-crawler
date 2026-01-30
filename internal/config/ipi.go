package config

// IPIConfig IPI 计算相关配置（权重、阈值、OVR 区间、分位档位等）
type IPIConfig struct {
	Weights     IPIWeights `json:"weights" yaml:"weights"`
	SPerf       IPISPerf   `json:"s_perf" yaml:"s_perf"`
	VGap        IPIVGap    `json:"v_gap" yaml:"v_gap"`
	RRisk       IPIRRisk   `json:"r_risk" yaml:"r_risk"`
	HistoryDays int        `json:"history_days" yaml:"history_days"` // 价格历史取最近天数，默认 90
}

// IPIWeights 综合公式权重
type IPIWeights struct {
	SPerf   float64 `json:"s_perf" yaml:"s_perf"`     // w₁ 表现盈余分权重，默认 0.4
	VGap    float64 `json:"v_gap" yaml:"v_gap"`       // w₂ 价值洼地分权重，默认 0.35
	MGrowth float64 `json:"m_growth" yaml:"m_growth"` // w₃ 成长动能分权重，默认 0.25
}

// IPISPerf 表现盈余分子项权重
type IPISPerf struct {
	Alpha float64 `json:"alpha" yaml:"alpha"` // 表现盈余-赛季比权重，默认 0.6
	Beta  float64 `json:"beta" yaml:"beta"`   // 表现盈余-倒挂权重，默认 0.4
}

// IPIVGap 价值洼地相关参数
type IPIVGap struct {
	OVRRadius         int     `json:"ovr_radius" yaml:"ovr_radius"`                     // 同 OVR 段半径，默认 2（即 [OVR-2, OVR+2]）
	TaxRate           float64 `json:"tax_rate" yaml:"tax_rate"`                         // 交易税率，默认 0.25
	MinNetProfitRatio float64 `json:"min_net_profit_ratio" yaml:"min_net_profit_ratio"` // 税后最低净利比例，默认 0.1
}

// IPIRRisk 风险折现相关参数
type IPIRRisk struct {
	Pct90 float64 `json:"pct90" yaml:"pct90"` // 当前价格 ≥ 90 分位时的风险系数，默认 0.3
	Pct75 float64 `json:"pct75" yaml:"pct75"` // 当前价格 ≥ 75 分位时的风险系数，默认 0.15
}

// DefaultIPIConfig 返回默认 IPI 配置
func DefaultIPIConfig() IPIConfig {
	return IPIConfig{
		Weights: IPIWeights{
			SPerf:   0.4,
			VGap:    0.35,
			MGrowth: 0.25,
		},
		SPerf: IPISPerf{
			Alpha: 0.6,
			Beta:  0.4,
		},
		VGap: IPIVGap{
			OVRRadius:         2,
			TaxRate:           0.25,
			MinNetProfitRatio: 0.1,
		},
		RRisk: IPIRRisk{
			Pct90: 0.3,
			Pct75: 0.15,
		},
		HistoryDays: 90,
	}
}
