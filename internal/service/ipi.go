package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"slices"
	"sort"

	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/model"
)

const (
	// minOVRSegmentCount 同 OVR 段最少样本数，低于则用全表均价回退
	minOVRSegmentCount = 3

	// minHistoryDaysForIPI 参与 IPI 计算至少需要的历史价格天数，少于此天数的球员被排除
	minHistoryDaysForIPI = 3

	// minPriceForIPI 参与 IPI 计算的球员最低价格（price_standard），只计算价格 > minPriceForIPI 的球员
	minPriceForIPI = 8000

	// minRecentGamesForIPI 参与 IPI 计算至少需要的近期比赛场次，少于此场次的球员被排除（样本太少）
	minRecentGamesForIPI = 5

	// ipiWinsorizePercentile 归一化前对 IPI 做截断的分位数，避免极端异常值拉偏整体尺度
	ipiWinsorizePercentile = 0.99

	// componentWinsorizeLowerPercentile 各子维度归一化前下分位截断，抑制低端异常值
	componentWinsorizeLowerPercentile = 0.01

	// componentWinsorizeUpperPercentile 各子维度归一化前上分位截断，抑制高端异常值
	componentWinsorizeUpperPercentile = 0.99
)

// ipiPreloadData 批量预加载的数据，避免 N+1 查询
type ipiPreloadData struct {
	priceHistory    map[uint][]entity.PlayerPriceHistory // player_id -> 近 N 天价格历史
	recentGameStats map[uint][]entity.PlayerGameStats    // tx_player_id -> 近 N 场比赛数据
	ovrAvgPrice     map[uint]float64                     // over_all -> 同 OVR 段均价
	ovrCount        map[uint]int64                       // over_all -> 同 OVR 段球员数
	globalAvgPrice  float64                              // 全表均价（用于回退）
}

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
func (s *IPIService) SeasonPowerFromStats(stats *entity.PlayerSeasonStats) float64 {
	if stats == nil {
		return 0
	}
	return calcPower(stats.Points, stats.Rebounds, stats.Assists, stats.Steals, stats.Blocks, stats.Turnovers)
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
	slices.Sort(prices)
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

// percentileAtFloat64 计算 float64 切片在给定分位（0~1）的值，线性插值
func percentileAtFloat64(sorted []float64, p float64) float64 {
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
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func clampValue(val, low, high float64) float64 {
	if high < low {
		low, high = high, low
	}
	if val < low {
		return low
	}
	if val > high {
		return high
	}
	return val
}

// ipiRankData 参与排名球员的 power_per5、over_all 排名，用于计算 RankInversionIndex
type ipiRankData struct {
	realPerfRank map[uint]int // playerID -> 真实表现排名（1=最高 power_per5）
	gameOVRRank  map[uint]int // playerID -> 游戏能力值排名（1=最高 over_all）
	n            int          // 参与排名球员总数
}

// buildRankDataFromPlayers 基于已有球员列表按 power_per5、over_all 降序排名，不访问 DB
func (s *IPIService) buildRankDataFromPlayers(players []entity.Player) *ipiRankData {
	if len(players) == 0 {
		return &ipiRankData{realPerfRank: make(map[uint]int), gameOVRRank: make(map[uint]int), n: 0}
	}
	// 排名基准只保留有有效 OVR 与近期战力的球员，避免噪声样本干扰倒挂指数
	rankPlayers := make([]entity.Player, 0, len(players))
	for i := range players {
		if players[i].OverAll > 0 && players[i].PowerPer5 > 0 && players[i].PriceStandard > minPriceForIPI {
			rankPlayers = append(rankPlayers, players[i])
		}
	}
	if len(rankPlayers) == 0 {
		return &ipiRankData{realPerfRank: make(map[uint]int), gameOVRRank: make(map[uint]int), n: 0}
	}
	// 按 power_per5 降序（同分按 player_id 稳定排序）
	sort.Slice(rankPlayers, func(i, j int) bool {
		if rankPlayers[i].PowerPer5 != rankPlayers[j].PowerPer5 {
			return rankPlayers[i].PowerPer5 > rankPlayers[j].PowerPer5
		}
		return rankPlayers[i].PlayerID < rankPlayers[j].PlayerID
	})
	realPerfRank := make(map[uint]int, len(rankPlayers))
	for r, p := range rankPlayers {
		realPerfRank[p.PlayerID] = r + 1
	}
	// 按 over_all 降序
	sort.Slice(rankPlayers, func(i, j int) bool {
		if rankPlayers[i].OverAll != rankPlayers[j].OverAll {
			return rankPlayers[i].OverAll > rankPlayers[j].OverAll
		}
		return rankPlayers[i].PlayerID < rankPlayers[j].PlayerID
	})
	gameOVRRank := make(map[uint]int, len(rankPlayers))
	for r, p := range rankPlayers {
		gameOVRRank[p.PlayerID] = r + 1
	}
	return &ipiRankData{realPerfRank: realPerfRank, gameOVRRank: gameOVRRank, n: len(rankPlayers)}
}

// BuildRankData 获取全量参与 IPI 计算的球员（排除自由球员与 tx_player_id=0），按 power_per5、over_all 降序排名
func (s *IPIService) BuildRankData(ctx context.Context) (*ipiRankData, error) {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	players, err := playerRepo.GetAllTxPlayers(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildRankDataFromPlayers(players), nil
}

// CalcSPerf 表现盈余分：S_perf = α×(PowerPer5/PowerSeasonAvg) + β×RankInversionIndex
// 若无赛季数据则用 power_per10 作为 PowerSeasonAvg；PowerSeasonAvg<=0 时该项比值为 0
func (s *IPIService) CalcSPerf(ctx context.Context, player *entity.Player, seasonStats *entity.PlayerSeasonStats, rankInversionIndex float64) float64 {
	cfg := s.config.SPerf
	powerSeasonAvg := s.SeasonPowerFromStats(seasonStats)
	if powerSeasonAvg <= 0 {
		powerSeasonAvg = player.PowerPer10
	}
	var ratio float64
	if powerSeasonAvg > 0 {
		ratio = player.PowerPer5 / powerSeasonAvg
	}
	ratioMin := cfg.RatioMin
	ratioMax := cfg.RatioMax
	if ratioMin <= 0 {
		ratioMin = 0.65
	}
	if ratioMax < ratioMin {
		ratioMax = ratioMin
	}
	ratio = clampValue(ratio, ratioMin, ratioMax)
	return cfg.Alpha*ratio + cfg.Beta*clampValue(rankInversionIndex, 0, 1)
}

// CalcVGap 价值洼地分：V_gap = PriceOVRAvg / PriceStandard；同 OVR 段样本过少时用全表均价
// 返回 vGap 与 priceOVRAvg（供 MeetsTaxSafeMargin 使用）。PriceStandard<=0 时返回 0
func (s *IPIService) CalcVGap(ctx context.Context, player *entity.Player) (vGap float64, priceOVRAvg float64, err error) {
	if player.PriceStandard <= 0 {
		return 0, 0, nil
	}
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	radius := max(s.config.VGap.OVRRadius, 0)
	avg, count, err := playerRepo.AvgPriceByOVRSegment(ctx, player.OverAll, radius)
	if err != nil {
		return 0, 0, err
	}
	if count < minOVRSegmentCount {
		avg, err = playerRepo.AvgPriceGlobal(ctx)
		if err != nil {
			return 0, 0, err
		}
	} else if count > 1 {
		// 对齐批量路径：排除球员自身，减少自引用偏差
		sumExSelf := avg*float64(count) - float64(player.PriceStandard)
		cntExSelf := count - 1
		if cntExSelf > 0 && sumExSelf > 0 {
			avg = sumExSelf / float64(cntExSelf)
		}
	}
	priceOVRAvg = avg
	if priceOVRAvg <= 0 {
		return 0, priceOVRAvg, nil
	}
	vGap = clampValue(priceOVRAvg/float64(player.PriceStandard), 0, 3)
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

// ageFactorFromAge 根据球员年龄计算成长因子：年轻球员加成，老将略降；age=0 返回 1.0
func ageFactorFromAge(age uint) float64 {
	if age == 0 {
		return 1.0
	}
	a := int(age)
	switch {
	case a <= 23:
		return 1.08 // 年轻球员成长空间
	case a <= 27:
		return 1.0
	case a <= 30:
		return 0.95
	default:
		return 0.90 // 31+ 略降
	}
}

func (s *IPIService) normalizedGrowthWeights() (minutesW, powerW float64) {
	minutesW = s.config.MGrowth.MinutesTrendWeight
	powerW = s.config.MGrowth.PowerTrendWeight
	if minutesW < 0 {
		minutesW = 0
	}
	if powerW < 0 {
		powerW = 0
	}
	sum := minutesW + powerW
	if sum <= 0 {
		return 1, 0
	}
	return minutesW / sum, powerW / sum
}

func (s *IPIService) trendCap() float64 {
	cap := s.config.MGrowth.MinutesTrendMaxCap
	if cap <= 0 {
		return 0.2
	}
	return cap
}

func trendRatio(delta, base, cap float64) float64 {
	if base <= 0 {
		return 0
	}
	return clampValue(delta/base, -cap, cap)
}

// CalcMGrowth 成长动能与题材分：AgeFactor×(1 + CombinedTrend + TradeRumorBonus)
// CombinedTrend = 上场时间趋势(近 N 场 vs 赛季) 与 战力趋势(power_per5 vs power_per10) 的加权融合
func (s *IPIService) CalcMGrowth(ctx context.Context, player *entity.Player, seasonStats *entity.PlayerSeasonStats) (float64, error) {
	ageFactor := ageFactorFromAge(player.Age)
	minutesTrend := 0.0
	powerTrend := 0.0

	if seasonStats != nil && seasonStats.Minutes > 0 && player.TxPlayerID > 0 {
		recentGames := s.config.MGrowth.RecentGames
		if recentGames <= 0 {
			recentGames = 10
		}
		mtRecent, err := s.AverageMinutesLastNGames(ctx, player.TxPlayerID, recentGames)
		if err == nil && mtRecent > 0 {
			minutesTrend = trendRatio(mtRecent-seasonStats.Minutes, seasonStats.Minutes, s.trendCap())
		}
	}
	if player.PowerPer10 > 0 {
		powerTrend = trendRatio(player.PowerPer5-player.PowerPer10, player.PowerPer10, s.trendCap())
	}

	minutesW, powerW := s.normalizedGrowthWeights()
	combinedTrend := minutesW*minutesTrend + powerW*powerTrend
	tradeRumorBonus := 0.0
	mGrowth := ageFactor * (1 + combinedTrend + tradeRumorBonus)
	if mGrowth < 0 {
		mGrowth = 0
	}
	return mGrowth, nil
}

func riskByPercentileRank(rank, pct75, pct90, pct99 float64) float64 {
	if rank <= 0.75 {
		return 0
	}
	if pct90 < pct75 {
		pct90 = pct75
	}
	if pct99 < pct90 {
		pct99 = pct90
	}
	if rank <= 0.90 {
		t := (rank - 0.75) / 0.15
		return pct75 + t*(pct90-pct75)
	}
	if rank <= 0.99 {
		t := (rank - 0.90) / 0.09
		return pct90 + t*(pct99-pct90)
	}
	return pct99
}

func percentileRank(sorted []uint, value uint) float64 {
	if len(sorted) == 0 {
		return 0
	}
	// 经验分位（<= value 的样本比例）
	idx := sort.Search(len(sorted), func(i int) bool {
		return sorted[i] > value
	})
	return float64(idx) / float64(len(sorted))
}

func priceVolatilityCV(prices []uint) float64 {
	if len(prices) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range prices {
		sum += float64(p)
	}
	mean := sum / float64(len(prices))
	if mean <= 0 {
		return 0
	}
	variance := 0.0
	for _, p := range prices {
		diff := float64(p) - mean
		variance += diff * diff
	}
	variance /= float64(len(prices))
	std := math.Sqrt(variance)
	return std / mean
}

func extractPrices(history []entity.PlayerPriceHistory) []uint {
	prices := make([]uint, 0, len(history))
	for i := range history {
		if history[i].PriceStandard > 0 {
			prices = append(prices, history[i].PriceStandard)
		}
	}
	return prices
}

func (s *IPIService) riskFromPriceHistory(currentPrice uint, history []entity.PlayerPriceHistory) float64 {
	prices := extractPrices(history)
	if len(prices) == 0 {
		return 0
	}
	slices.Sort(prices)
	cfg := s.config.RRisk
	percentileRisk := riskByPercentileRank(percentileRank(prices, currentPrice), cfg.Pct75, cfg.Pct90, cfg.Pct99)

	volatilityRisk := 0.0
	volBase := cfg.VolatilityBaseline
	if volBase <= 0 {
		volBase = 0.08
	}
	volWeight := cfg.VolatilityWeight
	if volWeight > 0 {
		// 历史样本过少时弱化波动率惩罚，避免 3~5 天数据过度放大噪声
		if len(prices) < 7 {
			volWeight *= float64(len(prices)) / 7.0
		}
		volatilityRisk = clampValue(priceVolatilityCV(prices)/volBase, 0, 1) * volWeight
	}

	return clampValue(percentileRisk+volatilityRisk, 0, 1)
}

// CalcRRisk 风险折现因子：InjuryRisk(占位 0) + 连续价格分位风险 + 价格波动率风险；结果 clamp 在 [0,1]
func (s *IPIService) CalcRRisk(ctx context.Context, playerID uint, currentPrice uint) (float64, error) {
	days := s.config.HistoryDays
	if days <= 0 {
		days = 90
	}
	historyRepo := repositories.NewHistoryRepository(s.db.DB)
	history, err := historyRepo.GetDays(ctx, playerID, days)
	if err != nil {
		return 0, err
	}
	return s.riskFromPriceHistory(currentPrice, history), nil
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
// 排除：价格 ≤ minPriceForIPI、本赛季无场均数据、能力值 over_all=0、历史价格数据少于 minHistoryDaysForIPI 天、
//
//	近期比赛数据 < minRecentGamesForIPI 场的球员；计算中非有限分量（NaN/Inf）的球员跳过；
//	归一化前对 IPI 做 99 分位截断以弱化异常高值影响。
//
// 优化：批量预加载价格历史、比赛数据、OVR 均价，避免 N+1 查询
func (s *IPIService) BatchCalcIPI(ctx context.Context, playerIDs []uint) ([]model.IPIResult, error) {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	statsRepo := repositories.NewStatsRepository(s.db.DB)
	historyRepo := repositories.NewHistoryRepository(s.db.DB)

	var players []entity.Player
	if len(playerIDs) == 0 {
		var err error
		players, err = playerRepo.GetAllTxPlayers(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		players, err = playerRepo.BatchGetByIDs(ctx, playerIDs)
		if err != nil {
			return nil, err
		}
	}
	log.Printf("[IPI] 初始候选球员数: %d", len(players))

	// 倒挂指数使用全量排名基准，避免子集计算时出现排名偏移
	rankData, err := s.BuildRankData(ctx)
	if err != nil {
		return nil, fmt.Errorf("build rank data: %w", err)
	}
	log.Printf("[IPI] 排名基准球员数: %d", rankData.n)

	// 排除低价噪声样本（此前仅定义常量，未真正生效）
	prevCount := len(players)
	playersWithMinPrice := make([]entity.Player, 0, len(players))
	for i := range players {
		if players[i].PriceStandard > minPriceForIPI {
			playersWithMinPrice = append(playersWithMinPrice, players[i])
		}
	}
	players = playersWithMinPrice
	log.Printf("[IPI] 价格过滤(>%d): %d -> %d", minPriceForIPI, prevCount, len(players))

	txIDs := make([]uint, 0, len(players))
	for _, p := range players {
		if p.TxPlayerID > 0 {
			txIDs = append(txIDs, p.TxPlayerID)
		}
	}

	// 本赛季场均数据
	season := s.config.Season
	if season == "" {
		season = "2025-26"
	}
	seasonStatsMap, err := statsRepo.GetSeasonStatsByTxPlayerIDs(ctx, txIDs, season, 1)
	if err != nil {
		return nil, err
	}

	// 排除本赛季没有场均数据的球员
	prevCount = len(players)
	playersWithSeason := make([]entity.Player, 0, len(players))
	for i := range players {
		if seasonStatsMap[players[i].TxPlayerID] != nil {
			playersWithSeason = append(playersWithSeason, players[i])
		}
	}
	players = playersWithSeason
	log.Printf("[IPI] 赛季数据过滤: %d -> %d", prevCount, len(players))

	// 排除能力值为 0 的球员（无效或未设定 OVR）
	prevCount = len(players)
	playersWithOVR := make([]entity.Player, 0, len(players))
	for i := range players {
		if players[i].OverAll > 0 {
			playersWithOVR = append(playersWithOVR, players[i])
		}
	}
	players = playersWithOVR
	log.Printf("[IPI] OVR 过滤: %d -> %d", prevCount, len(players))

	// 排除历史价格数据少于 minHistoryDaysForIPI 天的球员
	withinDays := s.config.HistoryDays
	if withinDays <= 0 {
		withinDays = 90
	}
	enoughHistory, err := historyRepo.GetPlayerIDsWithAtLeastDays(ctx, withinDays, minHistoryDaysForIPI)
	if err != nil {
		return nil, err
	}
	prevCount = len(players)
	playersWithHistory := make([]entity.Player, 0, len(players))
	for i := range players {
		if enoughHistory[players[i].PlayerID] {
			playersWithHistory = append(playersWithHistory, players[i])
		}
	}
	players = playersWithHistory
	log.Printf("[IPI] 历史价格过滤(>= %d 天): %d -> %d", minHistoryDaysForIPI, prevCount, len(players))
	if len(players) == 0 {
		return []model.IPIResult{}, nil
	}

	// 收集需要预加载的 ID
	playerIDsToLoad := make([]uint, len(players))
	txIDsToLoad := make([]uint, 0, len(players))
	for i := range players {
		playerIDsToLoad[i] = players[i].PlayerID
		if players[i].TxPlayerID > 0 {
			txIDsToLoad = append(txIDsToLoad, players[i].TxPlayerID)
		}
	}

	// 批量预加载数据（性能优化核心）
	preload, err := s.preloadBatchData(ctx, playerIDsToLoad, txIDsToLoad)
	if err != nil {
		return nil, err
	}
	log.Printf("[IPI] 预加载完成: %d 个球员", len(playerIDsToLoad))

	// 排除近期比赛数据 < minRecentGamesForIPI 场的球员（样本太少，近期战力/趋势不可靠）
	prevCount = len(players)
	playersWithEnoughGames := make([]entity.Player, 0, len(players))
	for i := range players {
		n := len(preload.recentGameStats[players[i].TxPlayerID])
		if n >= minRecentGamesForIPI {
			playersWithEnoughGames = append(playersWithEnoughGames, players[i])
		}
	}
	players = playersWithEnoughGames
	log.Printf("[IPI] 近期比赛过滤(>= %d 场): %d -> %d", minRecentGamesForIPI, prevCount, len(players))
	if len(players) == 0 {
		return []model.IPIResult{}, nil
	}

	// 第一轮：计算所有球员的原始分数（非有限分量会跳过）
	rawResults := make([]model.IPIResult, 0, len(players))
	for _, p := range players {
		res, err := s.calcOneIPIWithPreload(ctx, &p, seasonStatsMap[p.TxPlayerID], rankData, preload)
		if err != nil {
			continue
		}
		rawResults = append(rawResults, *res)
	}
	log.Printf("[IPI] 计算完成: 原始结果=%d, 候选=%d", len(rawResults), len(players))

	// 第二轮：归一化 + 添加可解释性
	results := s.normalizeAndExplain(rawResults)
	return results, nil
}

// preloadBatchData 批量预加载 IPI 计算所需的数据
func (s *IPIService) preloadBatchData(ctx context.Context, playerIDs, txPlayerIDs []uint) (*ipiPreloadData, error) {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	statsRepo := repositories.NewStatsRepository(s.db.DB)
	historyRepo := repositories.NewHistoryRepository(s.db.DB)

	preload := &ipiPreloadData{}

	// 1. 批量获取价格历史（用于 R_risk 计算）
	days := s.config.HistoryDays
	if days <= 0 {
		days = 90
	}
	priceHistory, err := historyRepo.BatchGetDays(ctx, playerIDs, days)
	if err != nil {
		return nil, fmt.Errorf("preload price history: %w", err)
	}
	log.Printf("[IPI] 预加载价格历史: %d 条", len(priceHistory))
	preload.priceHistory = priceHistory

	// 2. 批量获取近 N 场比赛数据（用于 M_growth 上场时间趋势）
	recentGames := s.config.MGrowth.RecentGames
	if recentGames <= 0 {
		recentGames = 10
	}
	gameStats, err := statsRepo.BatchGetRecentGameStats(ctx, txPlayerIDs, recentGames)
	if err != nil {
		return nil, fmt.Errorf("preload game stats: %w", err)
	}
	log.Printf("[IPI] 预加载比赛数据: %d 条", len(gameStats))
	preload.recentGameStats = gameStats

	// 3. 预加载 OVR 均价 map（用于 V_gap 计算）
	ovrAvg, err := playerRepo.OVRAvgPriceMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("preload OVR avg: %w", err)
	}
	log.Printf("[IPI] 预加载 OVR 均价: %d 条", len(ovrAvg))
	preload.ovrAvgPrice = ovrAvg

	// 4. 预加载 OVR 数量 map
	ovrCount, err := playerRepo.OVRCountMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("preload OVR count: %w", err)
	}
	log.Printf("[IPI] 预加载 OVR 数量: %d 条", len(ovrCount))
	preload.ovrCount = ovrCount

	// 5. 全表均价（用于 V_gap 回退）
	globalAvg, err := playerRepo.AvgPriceGlobal(ctx)
	if err != nil {
		return nil, fmt.Errorf("preload global avg: %w", err)
	}
	log.Printf("[IPI] 预加载全表均价: %f", globalAvg)
	preload.globalAvgPrice = globalAvg

	return preload, nil
}

// calcOneIPIWithPreload 使用预加载数据计算单球员 IPI（避免单独查询 DB）
// 若任一维度为 NaN/Inf 则返回错误，该球员被排除，避免明显异常潜力值进入结果
func (s *IPIService) calcOneIPIWithPreload(ctx context.Context, player *entity.Player, seasonStats *entity.PlayerSeasonStats, rankData *ipiRankData, preload *ipiPreloadData) (*model.IPIResult, error) {
	rankInv := s.rankInversionIndex(player.PlayerID, rankData)
	sPerf := s.CalcSPerf(ctx, player, seasonStats, rankInv)

	// V_gap：使用预加载的 OVR 均价
	vGap, priceOVRAvg := s.calcVGapWithPreload(player, preload)

	// M_growth：使用预加载的比赛数据
	mGrowth := s.calcMGrowthWithPreload(player, seasonStats, preload)

	// R_risk：使用预加载的价格历史
	rRisk := s.calcRRiskWithPreload(player, preload)

	if !isFinite(sPerf) || !isFinite(vGap) || !isFinite(mGrowth) || !isFinite(rRisk) {
		return nil, fmt.Errorf("non-finite IPI component: player_id=%d", player.PlayerID)
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

// calcVGapWithPreload 使用预加载数据计算 V_gap
func (s *IPIService) calcVGapWithPreload(player *entity.Player, preload *ipiPreloadData) (vGap float64, priceOVRAvg float64) {
	if player.PriceStandard <= 0 {
		return 0, 0
	}

	radius := max(s.config.VGap.OVRRadius, 0)
	ovr := player.OverAll

	// 计算 [OVR-radius, OVR+radius] 范围内的加权均价
	var sumPrice float64
	var sumCount int64
	for o := int(ovr) - radius; o <= int(ovr)+radius; o++ {
		if o < 0 {
			continue
		}
		if avg, ok := preload.ovrAvgPrice[uint(o)]; ok {
			cnt := preload.ovrCount[uint(o)]
			if cnt <= 0 {
				continue
			}

			segmentSum := avg * float64(cnt)
			segmentCount := cnt

			// 排除球员自身，避免同 OVR 段均价受本人价格反向影响
			if uint(o) == ovr {
				if segmentCount <= 1 {
					continue
				}
				segmentSum -= float64(player.PriceStandard)
				segmentCount--
			}
			if segmentCount <= 0 || segmentSum <= 0 {
				continue
			}

			sumPrice += segmentSum
			sumCount += segmentCount
		}
	}

	if sumCount >= minOVRSegmentCount && sumCount > 0 {
		priceOVRAvg = sumPrice / float64(sumCount)
	} else {
		priceOVRAvg = preload.globalAvgPrice
	}

	if priceOVRAvg <= 0 {
		return 0, priceOVRAvg
	}
	vGap = clampValue(priceOVRAvg/float64(player.PriceStandard), 0, 3)
	return vGap, priceOVRAvg
}

// calcMGrowthWithPreload 使用预加载数据计算 M_growth
func (s *IPIService) calcMGrowthWithPreload(player *entity.Player, seasonStats *entity.PlayerSeasonStats, preload *ipiPreloadData) float64 {
	ageFactor := ageFactorFromAge(player.Age)
	minutesTrend := 0.0
	powerTrend := 0.0

	if seasonStats != nil && seasonStats.Minutes > 0 && player.TxPlayerID > 0 {
		games := preload.recentGameStats[player.TxPlayerID]
		if len(games) > 0 {
			sum := 0
			for _, g := range games {
				sum += g.Minutes
			}
			mtRecent := float64(sum) / float64(len(games))
			if mtRecent > 0 {
				minutesTrend = trendRatio(mtRecent-seasonStats.Minutes, seasonStats.Minutes, s.trendCap())
			}
		}
	}
	if player.PowerPer10 > 0 {
		powerTrend = trendRatio(player.PowerPer5-player.PowerPer10, player.PowerPer10, s.trendCap())
	}

	minutesW, powerW := s.normalizedGrowthWeights()
	combinedTrend := minutesW*minutesTrend + powerW*powerTrend
	tradeRumorBonus := 0.0
	return max(ageFactor*(1+combinedTrend+tradeRumorBonus), 0)
}

// calcRRiskWithPreload 使用预加载数据计算 R_risk
func (s *IPIService) calcRRiskWithPreload(player *entity.Player, preload *ipiPreloadData) float64 {
	history := preload.priceHistory[player.PlayerID]
	return s.riskFromPriceHistory(player.PriceStandard, history)
}

// isFinite 判断 float64 为有限值（非 NaN、非 Inf）
func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// normalizeAndExplain 对原始分数进行 Min-Max 归一化，并添加可解释性说明
// 归一化前对 IPI 做 99 分位截断，排除明显异常高值对尺度的拉偏
func (s *IPIService) normalizeAndExplain(rawResults []model.IPIResult) []model.IPIResult {
	if len(rawResults) == 0 {
		return rawResults
	}

	// 对 IPI 做分位数截断，避免极端异常值影响归一化
	ipiValues := make([]float64, len(rawResults))
	for i := range rawResults {
		ipiValues[i] = rawResults[i].IPI
	}
	sort.Float64s(ipiValues)
	p99 := percentileAtFloat64(ipiValues, ipiWinsorizePercentile)
	for i := range rawResults {
		if rawResults[i].IPI > p99 {
			rawResults[i].IPI = p99
		}
	}

	// 对分项同样做分位数截断，避免单一异常值挤压 Min-Max 区间。
	// 注意：仅用于归一化与解释，不改写原始分项值（保持输出分项与计算时口径一致）
	sPerfValues := make([]float64, len(rawResults))
	vGapValues := make([]float64, len(rawResults))
	mGrowthValues := make([]float64, len(rawResults))
	for i := range rawResults {
		sPerfValues[i] = rawResults[i].SPerf
		vGapValues[i] = rawResults[i].VGap
		mGrowthValues[i] = rawResults[i].MGrowth
	}
	sort.Float64s(sPerfValues)
	sort.Float64s(vGapValues)
	sort.Float64s(mGrowthValues)

	sPerfLow := percentileAtFloat64(sPerfValues, componentWinsorizeLowerPercentile)
	sPerfHigh := percentileAtFloat64(sPerfValues, componentWinsorizeUpperPercentile)
	vGapLow := percentileAtFloat64(vGapValues, componentWinsorizeLowerPercentile)
	vGapHigh := percentileAtFloat64(vGapValues, componentWinsorizeUpperPercentile)
	mGrowthLow := percentileAtFloat64(mGrowthValues, componentWinsorizeLowerPercentile)
	mGrowthHigh := percentileAtFloat64(mGrowthValues, componentWinsorizeUpperPercentile)

	sPerfClipped := make([]float64, len(rawResults))
	vGapClipped := make([]float64, len(rawResults))
	mGrowthClipped := make([]float64, len(rawResults))
	for i := range rawResults {
		sPerfClipped[i] = clampValue(rawResults[i].SPerf, sPerfLow, sPerfHigh)
		vGapClipped[i] = clampValue(rawResults[i].VGap, vGapLow, vGapHigh)
		mGrowthClipped[i] = clampValue(rawResults[i].MGrowth, mGrowthLow, mGrowthHigh)
	}

	// 计算各维度的 min/max
	stats := model.IPIBatchStats{
		SPerfMin:   math.MaxFloat64,
		SPerfMax:   -math.MaxFloat64,
		VGapMin:    math.MaxFloat64,
		VGapMax:    -math.MaxFloat64,
		MGrowthMin: math.MaxFloat64,
		MGrowthMax: -math.MaxFloat64,
	}

	for i := range rawResults {
		if sPerfClipped[i] < stats.SPerfMin {
			stats.SPerfMin = sPerfClipped[i]
		}
		if sPerfClipped[i] > stats.SPerfMax {
			stats.SPerfMax = sPerfClipped[i]
		}
		if vGapClipped[i] < stats.VGapMin {
			stats.VGapMin = vGapClipped[i]
		}
		if vGapClipped[i] > stats.VGapMax {
			stats.VGapMax = vGapClipped[i]
		}
		if mGrowthClipped[i] < stats.MGrowthMin {
			stats.MGrowthMin = mGrowthClipped[i]
		}
		if mGrowthClipped[i] > stats.MGrowthMax {
			stats.MGrowthMax = mGrowthClipped[i]
		}
	}

	// 归一化 + 生成说明
	results := make([]model.IPIResult, len(rawResults))
	for i := range rawResults {
		results[i] = rawResults[i]
		results[i].SPerfNorm = minMaxNorm(sPerfClipped[i], stats.SPerfMin, stats.SPerfMax)
		results[i].VGapNorm = minMaxNorm(vGapClipped[i], stats.VGapMin, stats.VGapMax)
		results[i].MGrowthNorm = minMaxNorm(mGrowthClipped[i], stats.MGrowthMin, stats.MGrowthMax)
		results[i].MainFactors = s.generateExplanation(&results[i])
	}

	return results
}

// minMaxNorm Min-Max 归一化到 [0, 1]
func minMaxNorm(val, min, max float64) float64 {
	if max <= min {
		return 0.5
	}
	norm := (val - min) / (max - min)
	if norm < 0 {
		return 0
	}
	if norm > 1 {
		return 1
	}
	return norm
}

// generateExplanation 生成可解释性说明
func (s *IPIService) generateExplanation(r *model.IPIResult) []string {
	factors := make([]string, 0, 4)

	// 表现盈余分析
	if r.SPerfNorm >= 0.7 {
		factors = append(factors, "近期表现优异")
	} else if r.SPerfNorm <= 0.3 {
		factors = append(factors, "近期表现一般")
	}

	// 价值洼地分析
	if r.VGapNorm >= 0.7 {
		factors = append(factors, "价格被低估")
	} else if r.VGapNorm <= 0.3 {
		factors = append(factors, "价格偏高")
	}

	// 成长动能分析
	if r.MGrowthNorm >= 0.7 {
		factors = append(factors, "成长潜力大")
	}

	// 风险分析
	if r.RRisk >= 0.25 {
		factors = append(factors, "价格接近历史高位")
	}

	// 能力值倒挂
	if r.RankInversionIndex >= 0.1 {
		factors = append(factors, "能力值存在倒挂")
	}

	// 税后安全边际
	if r.MeetsTaxSafeMargin {
		factors = append(factors, "具备税后安全边际")
	}

	return factors
}

// calcOneIPI 单球员 IPI 计算，聚合四维度与税后边际、倒挂指数
func (s *IPIService) calcOneIPI(ctx context.Context, player *entity.Player, seasonStats *entity.PlayerSeasonStats, rankData *ipiRankData) (*model.IPIResult, error) {
	rankInv := s.rankInversionIndex(player.PlayerID, rankData)
	sPerf := s.CalcSPerf(ctx, player, seasonStats, rankInv)
	vGap, priceOVRAvg, err := s.CalcVGap(ctx, player)
	if err != nil {
		return nil, err
	}
	mGrowth, err := s.CalcMGrowth(ctx, player, seasonStats)
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

// rankInverrankInversionIndexsionIndex 能力值倒挂指数：diff = GameOVRRank - RealPerfRank；diff<=0 则为 0，否则 min(1, diff/N)
func (s *IPIService) rankInversionIndex(playerID uint, rankData *ipiRankData) float64 {
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
