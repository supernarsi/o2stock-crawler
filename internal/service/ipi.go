package service

import (
	"context"
	"sort"

	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/model"
)

// IPIService IPI 计算服务
type IPIService struct {
	db     *db.DB
	config config.IPIConfig
}

// NewIPIService 创建 IPI 服务（使用默认配置）
func NewIPIService(database *db.DB) *IPIService {
	return &IPIService{db: database, config: config.DefaultIPIConfig()}
}

// NewIPIServiceWithConfig 使用指定配置创建 IPI 服务
func NewIPIServiceWithConfig(database *db.DB, cfg config.IPIConfig) *IPIService {
	return &IPIService{db: database, config: cfg}
}

// SeasonPowerFromStats 根据赛季场均数据计算战力值（与单场战力公式一致）
// 公式: Power = Points + 1.2×Rebounds + 1.5×Assists + 3×Steals + 3×Blocks - Turnovers
func SeasonPowerFromStats(stats *entity.PlayerSeasonStats) float64 {
	if stats == nil {
		return 0
	}
	return stats.Points +
		1.2*stats.Rebounds +
		1.5*stats.Assists +
		3.0*stats.Steals +
		3.0*stats.Blocks -
		stats.Turnovers
}

// AverageMinutesLastNGames 计算球员近 N 场场均上场时间（分钟）
// 基于 player_game_stats，复用 stats 查询封装
func (s *IPIService) AverageMinutesLastNGames(ctx context.Context, txPlayerID uint, n int) (float64, error) {
	if n <= 0 {
		return 0, nil
	}
	statsRepo := repositories.NewStatsRepository(s.db.DB)
	games, err := statsRepo.GetRecentGameStats(ctx, txPlayerID, n)
	if err != nil || len(games) == 0 {
		return 0, err
	}
	sum := 0
	for _, g := range games {
		sum += g.Minutes
	}
	return float64(sum) / float64(len(games)), nil
}

// PricePercentile 计算单球员近 days 天内历史价格分位数，返回 75、90 分位值
// 基于 p_p_history，用于 R_risk 价格饱和度
func (s *IPIService) PricePercentile(ctx context.Context, playerID uint, days int) (*model.PricePercentileResult, error) {
	if days <= 0 {
		days = s.config.HistoryDays
	}
	historyRepo := repositories.NewHistoryRepository(s.db.DB)
	history, err := historyRepo.GetDays(ctx, playerID, days)
	if err != nil {
		return nil, err
	}
	return pricePercentileFromHistory(history), nil
}

// pricePercentileFromHistory 从价格历史切片计算 75、90 分位
func pricePercentileFromHistory(history []entity.PlayerPriceHistory) *model.PricePercentileResult {
	out := &model.PricePercentileResult{}
	if len(history) == 0 {
		return out
	}
	out.HasEnoughData = true
	prices := make([]uint, len(history))
	for i := range history {
		prices[i] = history[i].PriceStandard
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i] < prices[j] })
	out.P75 = percentileAt(prices, 0.75)
	out.P90 = percentileAt(prices, 0.90)
	return out
}

// percentileAt 计算切片在给定分位（0~1）的值，线性插值
func percentileAt(sorted []uint, p float64) uint {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := p * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lo)
	return uint(float64(sorted[lo])*(1-frac) + float64(sorted[hi])*frac)
}

// ipiRankData 参与排名球员的 power_per5、over_all 排名，用于计算 RankInversionIndex
type ipiRankData struct {
	realPerfRank map[uint]int // playerID -> 真实表现排名（1=最高 power_per5）
	gameOVRRank  map[uint]int // playerID -> 游戏能力值排名（1=最高 over_all）
	n            int          // 参与排名球员总数
}

// BuildRankData 获取全量参与 IPI 计算的球员（排除自由球员与 tx_player_id=0），按 power_per5、over_all 降序排名
func (s *IPIService) BuildRankData(ctx context.Context) (*ipiRankData, error) {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	players, err := playerRepo.GetAllTxPlayers(ctx) // 与 AvgPriceByOVRSegment / AvgPriceGlobal 使用同一人群
	if err != nil {
		return nil, err
	}
	if len(players) == 0 {
		return &ipiRankData{realPerfRank: make(map[uint]int), gameOVRRank: make(map[uint]int), n: 0}, nil
	}
	// 按 power_per5 降序（同分按 player_id 稳定排序）
	sort.Slice(players, func(i, j int) bool {
		if players[i].PowerPer5 != players[j].PowerPer5 {
			return players[i].PowerPer5 > players[j].PowerPer5
		}
		return players[i].PlayerID < players[j].PlayerID
	})
	realPerfRank := make(map[uint]int, len(players))
	for r, p := range players {
		realPerfRank[p.PlayerID] = r + 1
	}
	// 按 over_all 降序
	sort.Slice(players, func(i, j int) bool {
		if players[i].OverAll != players[j].OverAll {
			return players[i].OverAll > players[j].OverAll
		}
		return players[i].PlayerID < players[j].PlayerID
	})
	gameOVRRank := make(map[uint]int, len(players))
	for r, p := range players {
		gameOVRRank[p.PlayerID] = r + 1
	}
	return &ipiRankData{realPerfRank: realPerfRank, gameOVRRank: gameOVRRank, n: len(players)}, nil
}

// RankInversionIndex 能力值倒挂指数：diff = GameOVRRank - RealPerfRank；diff<=0 则为 0，否则 min(1, diff/N)
func RankInversionIndex(playerID uint, rankData *ipiRankData) float64 {
	if rankData == nil || rankData.n == 0 {
		return 0
	}
	gameRank, ok1 := rankData.gameOVRRank[playerID]
	realRank, ok2 := rankData.realPerfRank[playerID]
	if !ok1 || !ok2 {
		return 0
	}
	diff := gameRank - realRank
	if diff <= 0 {
		return 0
	}
	n := rankData.n
	if n <= 0 {
		return 0
	}
	v := float64(diff) / float64(n)
	if v > 1 {
		return 1
	}
	return v
}

// CalcSPerf 表现盈余分：S_perf = α×(PowerPer5/PowerSeasonAvg) + β×RankInversionIndex
// 若无赛季数据则用 power_per10 作为 PowerSeasonAvg；PowerSeasonAvg<=0 时该项比值为 0
func (s *IPIService) CalcSPerf(ctx context.Context, player *entity.Player, seasonStats *entity.PlayerSeasonStats, rankInversionIndex float64) float64 {
	cfg := s.config.SPerf
	powerSeasonAvg := SeasonPowerFromStats(seasonStats)
	if powerSeasonAvg <= 0 {
		powerSeasonAvg = player.PowerPer10
	}
	var ratio float64
	if powerSeasonAvg > 0 {
		ratio = player.PowerPer5 / powerSeasonAvg
	}
	return cfg.Alpha*ratio + cfg.Beta*rankInversionIndex
}

// minOVRSegmentCount 同 OVR 段最少样本数，低于则用全表均价回退
const minOVRSegmentCount = 3

// CalcVGap 价值洼地分：V_gap = PriceOVRAvg / PriceStandard；同 OVR 段样本过少时用全表均价
// 返回 vGap 与 priceOVRAvg（供 MeetsTaxSafeMargin 使用）。PriceStandard<=0 时返回 0
func (s *IPIService) CalcVGap(ctx context.Context, player *entity.Player) (vGap float64, priceOVRAvg float64, err error) {
	if player.PriceStandard <= 0 {
		return 0, 0, nil
	}
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	radius := s.config.VGap.OVRRadius
	if radius < 0 {
		radius = 0
	}
	avg, count, err := playerRepo.AvgPriceByOVRSegment(ctx, player.OverAll, radius)
	if err != nil {
		return 0, 0, err
	}
	if count < minOVRSegmentCount {
		avg, err = playerRepo.AvgPriceGlobal(ctx)
		if err != nil {
			return 0, 0, err
		}
	}
	priceOVRAvg = avg
	if priceOVRAvg <= 0 {
		return 0, priceOVRAvg, nil
	}
	vGap = priceOVRAvg / float64(player.PriceStandard)
	return vGap, priceOVRAvg, nil
}

// MeetsTaxSafeMargin 税后安全边际：P_target×0.75 > PriceStandard×1.1（税后至少约 10% 净利）
// P_target 使用同 OVR 段均价×1.05
func (s *IPIService) MeetsTaxSafeMargin(priceStandard uint, priceOVRAvg float64) bool {
	if priceStandard <= 0 || priceOVRAvg <= 0 {
		return false
	}
	cfg := s.config.VGap
	pTarget := priceOVRAvg * 1.05
	afterTax := pTarget * (1 - cfg.TaxRate)
	minRevenue := float64(priceStandard) * (1 + cfg.MinNetProfitRatio)
	return afterTax >= minRevenue
}

// CalcMGrowth 成长动能与题材分：AgeFactor×(1 + MinutesTrendBonus + TradeRumorBonus)
// 年龄暂无数据默认 1.0；TradeRumorBonus 固定 0；上场时间趋势来自近 10 场 vs 赛季场均
func (s *IPIService) CalcMGrowth(ctx context.Context, seasonStats *entity.PlayerSeasonStats, txPlayerID uint) (float64, error) {
	ageFactor := 1.0 // 无年龄数据，占位默认 1.0
	minutesTrendBonus := 0.0
	if seasonStats != nil && seasonStats.Minutes > 0 {
		mtRecent, err := s.AverageMinutesLastNGames(ctx, txPlayerID, 10)
		if err == nil && mtRecent > 0 {
			delta := mtRecent - seasonStats.Minutes
			if delta > 0 {
				minutesTrendBonus = delta / seasonStats.Minutes
				if minutesTrendBonus > 0.2 {
					minutesTrendBonus = 0.2
				}
			}
		}
	}
	tradeRumorBonus := 0.0 // 无数据，占位 0
	mGrowth := ageFactor * (1 + minutesTrendBonus + tradeRumorBonus)
	return mGrowth, nil
}

// CalcRRisk 风险折现因子：InjuryRisk(占位 0) + PriceSaturationRisk；结果 clamp 在 [0,1]
func (s *IPIService) CalcRRisk(ctx context.Context, playerID uint, currentPrice uint) (float64, error) {
	injuryRisk := 0.0 // 无伤病数据，占位 0
	perc, err := s.PricePercentile(ctx, playerID, s.config.HistoryDays)
	if err != nil {
		return 0, err
	}
	priceSaturationRisk := 0.0
	if perc.HasEnoughData {
		if currentPrice >= perc.P90 {
			priceSaturationRisk = s.config.RRisk.Pct90
		} else if currentPrice >= perc.P75 {
			priceSaturationRisk = s.config.RRisk.Pct75
		}
	}
	rRisk := injuryRisk + priceSaturationRisk
	if rRisk < 0 {
		rRisk = 0
	}
	if rRisk > 1 {
		rRisk = 1
	}
	return rRisk, nil
}

// CalcIPI 综合 IPI：IPI = (w₁·S_perf + w₂·V_gap + w₃·M_growth) × (1 - R_risk)，结果不小于 0
func (s *IPIService) CalcIPI(sPerf, vGap, mGrowth, rRisk float64) float64 {
	w := s.config.Weights
	weighted := w.SPerf*sPerf + w.VGap*vGap + w.MGrowth*mGrowth
	multiplier := 1 - rRisk
	if multiplier < 0 {
		multiplier = 0
	}
	if multiplier > 1 {
		multiplier = 1
	}
	ipi := weighted * multiplier
	if ipi < 0 {
		return 0
	}
	return ipi
}

// BatchCalcIPI 批量计算 IPI：给定球员 ID 列表；若 playerIDs 为空则对「全部参与 IPI 计算的球员」计算
// 返回按球员 ID 顺序的 IPIResult 列表；单球员计算失败时跳过该球员，不中断整体
func (s *IPIService) BatchCalcIPI(ctx context.Context, playerIDs []uint) ([]model.IPIResult, error) {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	statsRepo := repositories.NewStatsRepository(s.db.DB)

	if len(playerIDs) == 0 {
		all, err := playerRepo.GetAllTxPlayers(ctx)
		if err != nil {
			return nil, err
		}
		playerIDs = make([]uint, len(all))
		for i := range all {
			playerIDs[i] = all[i].PlayerID
		}
	}

	players, err := playerRepo.BatchGetByIDs(ctx, playerIDs)
	if err != nil {
		return nil, err
	}
	playerMap := make(map[uint]*entity.Player)
	for i := range players {
		playerMap[players[i].PlayerID] = &players[i]
	}

	rankData, err := s.BuildRankData(ctx)
	if err != nil {
		return nil, err
	}

	// 批量拉取赛季数据（按 tx_player_id）
	txIDs := make([]uint, 0, len(players))
	for _, p := range players {
		if p.TxPlayerID > 0 {
			txIDs = append(txIDs, p.TxPlayerID)
		}
	}
	seasonStatsMap := make(map[uint]*entity.PlayerSeasonStats)
	for _, txID := range txIDs {
		stats, _ := statsRepo.GetSeasonStats(ctx, txID)
		if stats != nil {
			seasonStatsMap[txID] = stats
		}
	}

	var results []model.IPIResult
	for _, playerID := range playerIDs {
		p, ok := playerMap[playerID]
		if !ok {
			continue
		}
		res, err := s.calcOneIPI(ctx, p, seasonStatsMap[p.TxPlayerID], rankData)
		if err != nil {
			continue
		}
		results = append(results, *res)
	}
	return results, nil
}

// calcOneIPI 单球员 IPI 计算，聚合四维度与税后边际、倒挂指数
func (s *IPIService) calcOneIPI(ctx context.Context, player *entity.Player, seasonStats *entity.PlayerSeasonStats, rankData *ipiRankData) (*model.IPIResult, error) {
	rankInv := RankInversionIndex(player.PlayerID, rankData)
	sPerf := s.CalcSPerf(ctx, player, seasonStats, rankInv)
	vGap, priceOVRAvg, err := s.CalcVGap(ctx, player)
	if err != nil {
		return nil, err
	}
	mGrowth, err := s.CalcMGrowth(ctx, seasonStats, player.TxPlayerID)
	if err != nil {
		return nil, err
	}
	rRisk, err := s.CalcRRisk(ctx, player.PlayerID, player.PriceStandard)
	if err != nil {
		return nil, err
	}
	ipi := s.CalcIPI(sPerf, vGap, mGrowth, rRisk)
	meetsTax := s.MeetsTaxSafeMargin(player.PriceStandard, priceOVRAvg)
	return &model.IPIResult{
		PlayerID:           player.PlayerID,
		IPI:                ipi,
		SPerf:              sPerf,
		VGap:               vGap,
		MGrowth:            mGrowth,
		RRisk:              rRisk,
		MeetsTaxSafeMargin: meetsTax,
		RankInversionIndex: rankInv,
	}, nil
}
