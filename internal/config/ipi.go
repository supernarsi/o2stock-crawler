package config

import (
	"os"
	"strconv"
)

// IPIConfig IPI 计算相关配置（权重、阈值、OVR 区间、分位档位等）
type IPIConfig struct {
	Weights     IPIWeights `json:"weights" yaml:"weights"`
	SPerf       IPISPerf   `json:"s_perf" yaml:"s_perf"`
	VGap        IPIVGap    `json:"v_gap" yaml:"v_gap"`
	RRisk       IPIRRisk   `json:"r_risk" yaml:"r_risk"`
	MGrowth     IPIMGrowth `json:"m_growth" yaml:"m_growth"`
	HistoryDays int        `json:"history_days" yaml:"history_days"` // 价格历史取最近天数，默认 90
	Season      string     `json:"season" yaml:"season"`             // 当前赛季，默认 "2025-26"
}

// IPIWeights 综合公式权重
type IPIWeights struct {
	SPerf   float64 `json:"s_perf" yaml:"s_perf"`     // w₁ 表现盈余分权重，默认 0.4
	VGap    float64 `json:"v_gap" yaml:"v_gap"`       // w₂ 价值洼地分权重，默认 0.35
	MGrowth float64 `json:"m_growth" yaml:"m_growth"` // w₃ 成长动能分权重，默认 0.25
}

// IPISPerf 表现盈余分子项权重
type IPISPerf struct {
	Alpha    float64 `json:"alpha" yaml:"alpha"`         // 表现盈余-赛季比权重，默认 0.6
	Beta     float64 `json:"beta" yaml:"beta"`           // 表现盈余-倒挂权重，默认 0.4
	RatioMin float64 `json:"ratio_min" yaml:"ratio_min"` // 赛季比下限裁剪，默认 0.65
	RatioMax float64 `json:"ratio_max" yaml:"ratio_max"` // 赛季比上限裁剪，默认 1.8
}

// IPIVGap 价值洼地相关参数
type IPIVGap struct {
	OVRRadius         int     `json:"ovr_radius" yaml:"ovr_radius"`                     // 同 OVR 段半径，默认 2（即 [OVR-2, OVR+2]）
	TaxRate           float64 `json:"tax_rate" yaml:"tax_rate"`                         // 交易税率，默认 0.25
	MinNetProfitRatio float64 `json:"min_net_profit_ratio" yaml:"min_net_profit_ratio"` // 税后最低净利比例，默认 0.1
}

// IPIRRisk 风险折现相关参数
type IPIRRisk struct {
	Pct90              float64 `json:"pct90" yaml:"pct90"`                             // 当前价格 ≥ 90 分位时的风险系数，默认 0.3
	Pct75              float64 `json:"pct75" yaml:"pct75"`                             // 当前价格 ≥ 75 分位时的风险系数，默认 0.15
	Pct99              float64 `json:"pct99" yaml:"pct99"`                             // 当前价格 ≥ 99 分位时的风险系数，默认 0.45
	VolatilityWeight   float64 `json:"volatility_weight" yaml:"volatility_weight"`     // 历史波动率风险权重，默认 0.18
	VolatilityBaseline float64 `json:"volatility_baseline" yaml:"volatility_baseline"` // 历史波动率基准（CV），默认 0.08
}

// IPIMGrowth 成长动能相关参数
type IPIMGrowth struct {
	RecentGames        int     `json:"recent_games" yaml:"recent_games"`                   // 近 N 场计算上场时间趋势，默认 10
	MinutesTrendMaxCap float64 `json:"minutes_trend_max_cap" yaml:"minutes_trend_max_cap"` // 趋势项绝对值上限（分钟/战力趋势共享），默认 0.2
	MinutesTrendWeight float64 `json:"minutes_trend_weight" yaml:"minutes_trend_weight"`   // 上场时间趋势权重，默认 0.65
	PowerTrendWeight   float64 `json:"power_trend_weight" yaml:"power_trend_weight"`       // 战力趋势权重，默认 0.35
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
			Alpha:    0.6,
			Beta:     0.4,
			RatioMin: 0.65,
			RatioMax: 1.8,
		},
		VGap: IPIVGap{
			OVRRadius:         2,
			TaxRate:           0.25,
			MinNetProfitRatio: 0.1,
		},
		RRisk: IPIRRisk{
			Pct90:              0.3,
			Pct75:              0.15,
			Pct99:              0.45,
			VolatilityWeight:   0.18,
			VolatilityBaseline: 0.08,
		},
		MGrowth: IPIMGrowth{
			RecentGames:        10,
			MinutesTrendMaxCap: 0.2,
			MinutesTrendWeight: 0.65,
			PowerTrendWeight:   0.35,
		},
		HistoryDays: 90,
		Season:      "2025-26",
	}
}

// LoadIPIConfigFromEnv 从环境变量加载 IPI 配置，未设置则使用默认值
func LoadIPIConfigFromEnv() IPIConfig {
	cfg := DefaultIPIConfig()

	// Weights
	if v := os.Getenv("IPI_WEIGHT_SPERF"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Weights.SPerf = f
		}
	}
	if v := os.Getenv("IPI_WEIGHT_VGAP"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Weights.VGap = f
		}
	}
	if v := os.Getenv("IPI_WEIGHT_MGROWTH"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Weights.MGrowth = f
		}
	}

	// SPerf
	if v := os.Getenv("IPI_SPERF_ALPHA"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.SPerf.Alpha = f
		}
	}
	if v := os.Getenv("IPI_SPERF_BETA"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.SPerf.Beta = f
		}
	}
	if v := os.Getenv("IPI_SPERF_RATIO_MIN"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.SPerf.RatioMin = f
		}
	}
	if v := os.Getenv("IPI_SPERF_RATIO_MAX"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.SPerf.RatioMax = f
		}
	}

	// VGap
	if v := os.Getenv("IPI_VGAP_OVR_RADIUS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.VGap.OVRRadius = i
		}
	}
	if v := os.Getenv("IPI_VGAP_TAX_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.VGap.TaxRate = f
		}
	}
	if v := os.Getenv("IPI_VGAP_MIN_NET_PROFIT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.VGap.MinNetProfitRatio = f
		}
	}

	// RRisk
	if v := os.Getenv("IPI_RRISK_PCT90"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RRisk.Pct90 = f
		}
	}
	if v := os.Getenv("IPI_RRISK_PCT75"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RRisk.Pct75 = f
		}
	}
	if v := os.Getenv("IPI_RRISK_PCT99"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RRisk.Pct99 = f
		}
	}
	if v := os.Getenv("IPI_RRISK_VOL_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RRisk.VolatilityWeight = f
		}
	}
	if v := os.Getenv("IPI_RRISK_VOL_BASELINE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RRisk.VolatilityBaseline = f
		}
	}

	// MGrowth
	if v := os.Getenv("IPI_MGROWTH_RECENT_GAMES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.MGrowth.RecentGames = i
		}
	}
	if v := os.Getenv("IPI_MGROWTH_MINUTES_CAP"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MGrowth.MinutesTrendMaxCap = f
		}
	}
	if v := os.Getenv("IPI_MGROWTH_MINUTES_TREND_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MGrowth.MinutesTrendWeight = f
		}
	}
	if v := os.Getenv("IPI_MGROWTH_POWER_TREND_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MGrowth.PowerTrendWeight = f
		}
	}

	// General
	if v := os.Getenv("IPI_HISTORY_DAYS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.HistoryDays = i
		}
	}
	if v := os.Getenv("IPI_SEASON"); v != "" {
		cfg.Season = v
	}

	return cfg
}
