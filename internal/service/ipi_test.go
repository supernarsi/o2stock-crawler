package service

import (
	"context"
	"math"
	"testing"

	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/entity"
)

func almostEqual(got, want float64) bool {
	const eps = 1e-9
	return math.Abs(got-want) <= eps
}

func TestRiskByPercentileRank(t *testing.T) {
	type tc struct {
		name string
		rank float64
		want float64
	}

	tests := []tc{
		{name: "below p75", rank: 0.74, want: 0},
		{name: "at p75", rank: 0.75, want: 0},
		{name: "between p75 and p90", rank: 0.825, want: 0.225},
		{name: "between p90 and p99", rank: 0.95, want: 0.3833333333333333},
		{name: "above p99", rank: 1, want: 0.45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := riskByPercentileRank(tt.rank, 0.15, 0.30, 0.45)
			if !almostEqual(got, tt.want) {
				t.Fatalf("riskByPercentileRank(%v)=%v, want %v", tt.rank, got, tt.want)
			}
		})
	}
}

func TestPercentileRank(t *testing.T) {
	sorted := []uint{100, 100, 200, 300}

	type tc struct {
		value uint
		want  float64
	}
	tests := []tc{
		{value: 50, want: 0},
		{value: 100, want: 0.5},
		{value: 150, want: 0.5},
		{value: 300, want: 1},
	}

	for _, tt := range tests {
		got := percentileRank(sorted, tt.value)
		if !almostEqual(got, tt.want) {
			t.Fatalf("percentileRank(%d)=%v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestCalcSPerfClampsRatio(t *testing.T) {
	svc := &IPIService{config: config.DefaultIPIConfig()}

	tests := []struct {
		name   string
		player entity.Player
		want   float64
	}{
		{
			name: "clamp high ratio",
			player: entity.Player{
				PowerPer5:  300,
				PowerPer10: 50,
			},
			want: svc.config.SPerf.Alpha * svc.config.SPerf.RatioMax,
		},
		{
			name: "clamp low ratio",
			player: entity.Player{
				PowerPer5:  10,
				PowerPer10: 100,
			},
			want: svc.config.SPerf.Alpha * svc.config.SPerf.RatioMin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.CalcSPerf(context.Background(), &tt.player, nil, 0)
			if !almostEqual(got, tt.want) {
				t.Fatalf("CalcSPerf()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalcMGrowthWithPreloadIncludesNegativeTrend(t *testing.T) {
	svc := &IPIService{config: config.DefaultIPIConfig()}
	player := &entity.Player{
		PlayerID:   1,
		TxPlayerID: 11,
		Age:        24,
		PowerPer5:  100,
		PowerPer10: 100,
	}
	seasonStats := &entity.PlayerSeasonStats{Minutes: 20}

	preload := &ipiPreloadData{
		recentGameStats: map[uint][]entity.PlayerGameStats{
			11: {
				{Minutes: 20},
				{Minutes: 20},
			},
		},
	}
	baseline := svc.calcMGrowthWithPreload(player, seasonStats, preload)

	player.PowerPer5 = 120
	preload.recentGameStats[11] = []entity.PlayerGameStats{{Minutes: 30}, {Minutes: 30}}
	positive := svc.calcMGrowthWithPreload(player, seasonStats, preload)

	player.PowerPer5 = 80
	preload.recentGameStats[11] = []entity.PlayerGameStats{{Minutes: 10}, {Minutes: 10}}
	negative := svc.calcMGrowthWithPreload(player, seasonStats, preload)

	if !(positive > baseline && baseline > negative) {
		t.Fatalf("unexpected growth order: positive=%v baseline=%v negative=%v", positive, baseline, negative)
	}
}

func TestCalcVGapWithPreloadExcludesSelf(t *testing.T) {
	cfg := config.DefaultIPIConfig()
	cfg.VGap.OVRRadius = 0
	svc := &IPIService{config: cfg}

	player := &entity.Player{
		PlayerID:      1,
		OverAll:       90,
		PriceStandard: 500,
	}
	preload := &ipiPreloadData{
		ovrAvgPrice:    map[uint]float64{90: 1000},
		ovrCount:       map[uint]int64{90: 4},
		globalAvgPrice: 2000,
	}

	vGap, avg := svc.calcVGapWithPreload(player, preload)
	if !almostEqual(avg, 1166.6666666666667) {
		t.Fatalf("priceOVRAvg=%v, want 1166.6666666666667", avg)
	}
	if !almostEqual(vGap, 2.3333333333333335) {
		t.Fatalf("vGap=%v, want 2.3333333333333335", vGap)
	}
}

func TestRiskFromPriceHistoryAddsVolatilityPenalty(t *testing.T) {
	svc := &IPIService{config: config.DefaultIPIConfig()}

	stableHistory := []entity.PlayerPriceHistory{
		{PriceStandard: 100},
		{PriceStandard: 100},
		{PriceStandard: 100},
		{PriceStandard: 100},
	}
	volatileHistory := []entity.PlayerPriceHistory{
		{PriceStandard: 50},
		{PriceStandard: 150},
		{PriceStandard: 50},
		{PriceStandard: 150},
	}

	stableRisk := svc.riskFromPriceHistory(50, stableHistory)
	volatileRisk := svc.riskFromPriceHistory(50, volatileHistory)
	if !(volatileRisk > stableRisk) {
		t.Fatalf("volatile risk should be higher: stable=%v volatile=%v", stableRisk, volatileRisk)
	}
}
