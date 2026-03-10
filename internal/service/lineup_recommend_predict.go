// lineup_recommend_predict.go 包含球员战力预测的核心入口 predictPower，
// 以及近期战力分析（calcRecentPowerProfile）和预测值校准（calibratePredictedPower）逻辑。
// 数据加载见 lineup_recommend_data.go，ID 映射见 lineup_recommend_mapping.go，
// 因子计算见 lineup_recommend_factors.go。
package service

import (
	"math"
	"sort"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/entity"
)

// 预测相关全局常量。
const (
	matchupLookbackDays    = 120 // 对手指标回溯天数
	teamMetricMaxGames     = 24  // 球队指标最大样本场次
	teamMetricMinGames     = 6   // 球队指标最小有效场次
	teamTrendRecentGames   = 6   // 球队近期趋势窗口
	dvpMetricMinSampleSize = 8   // DVP 因子最小样本量
	recentBaseMinGames     = 3   // 近期基础值最小样本量
)

// teamMatchupMetric 球队防守端对手匹配指标。
type teamMatchupMetric struct {
	DefRatingFactor       float64 // 防守效率因子
	PaceFactor            float64 // 比赛节奏因子
	OpponentFormFactor    float64 // 对手近期状态因子
	RimDeterrenceFactor   float64 // 篮下威慑因子（盖帽相关）
	PerimeterImpactFactor float64 // 外线干扰因子（抢断相关）
	SampleCount           int     // 样本场次数
}

// positionDVPMetric 按位置的对手防守价值指标（Defensive Value Per position）。
type positionDVPMetric struct {
	Factor      float64 // DVP 因子，>1 表示该对手该位置防守薄弱
	SampleCount int     // 样本数
}

// txLineupPlayer 腾讯体育阵容球员简要信息，用于 ID 映射。
type txLineupPlayer struct {
	ID     uint
	CnName string
	EnName string
}

// matchupFactorDetail 对手匹配因子的分项明细。
type matchupFactorDetail struct {
	MatchupFactor         float64 // 综合匹配因子
	DefRatingFactor       float64
	PaceFactor            float64
	DvPFactor             float64
	HistoryFactor         float64
	OpponentFormFactor    float64
	RimDeterrenceFactor   float64
	PerimeterImpactFactor float64
}

// recentPowerProfile 近期战力分布画像，用于校准预测值和评估数据可信度。
type recentPowerProfile struct {
	Avg3        float64 // 近 3 场平均战力
	Avg5        float64 // 近 5 场平均战力
	Avg10       float64 // 近 10 场平均战力
	Median5     float64 // 近 5 场中位数战力
	Volatility  float64 // 近 5 场变异系数（标准差/均值）
	SampleCount int     // 有效样本数
	Upside3     float64 // 近 3 场最高表现与 Avg3 的比率
	Upside5     float64 // 近 5 场最高表现与 Avg5 的比率
}

// predictPower 计算单个候选球员的预测战力及因子明细。
func (s *LineupRecommendService) predictPower(
	player entity.NBAGamePlayer,
	allPlayers []entity.NBAGamePlayer,
	injuryMap map[uint]crawler.InjuryReport,
	dbPlayerMap map[uint]*entity.Player,
	txPlayerIDMap map[uint]uint,
	gameStatsMap map[uint][]entity.PlayerGameStats,
	seasonStatsMap map[uint]*entity.PlayerSeasonStats,
	teamMatchupMap map[string]teamMatchupMetric,
	dvpFactorMap map[string]map[uint]positionDVPMetric,
) PlayerPrediction {

	// Step 1: 因素1 — 球员出场可用性 (AvailabilityScore)
	availabilityScore := resolveAvailabilityScore(player, injuryMap)
	if availabilityScore == 0 {
		return PlayerPrediction{AvailabilityScore: 0.0}
	}

	// Step 2: 基础战力值 (BaseValue)
	gamePower := player.CombatPower
	baseValue := gamePower
	dbPlayer := dbPlayerMap[player.NBAPlayerID]
	txPlayerID := txPlayerIDMap[player.NBAPlayerID]
	var stats []entity.PlayerGameStats
	if txPlayerID > 0 {
		stats = gameStatsMap[txPlayerID]
	}

	var dbPower5, dbPower10 float64
	if dbPlayer != nil && dbPlayer.PowerPer10 > 0 {
		dbPower10 = dbPlayer.PowerPer10
		dbPower5 = dbPlayer.PowerPer5
		baseValue = 0.4*dbPower10 + 0.3*dbPower5 + 0.3*gamePower
	}
	recentPower5, recentPower10 := calcRecentPowerAverages(stats)
	recentProfile := calcRecentPowerProfile(stats)
	baseValue = stabilizeBaseValue(baseValue, gamePower, recentPower5, recentPower10, len(stats))
	baseValue = buildRobustBaseValue(baseValue, gamePower, recentProfile)

	// Step 3: 因素3 — 近期状态趋势 (StatusTrend)
	statusTrend := calcStatusTrend(dbPlayer, stats)

	// Step 4: 因素4 — 对手实力匹配 (MatchupFactor)
	matchupFactor := 1.0
	defRatingFactor := 1.0
	paceFactor := 1.0
	dvpFactor := 1.0
	historyFactor := 1.0
	opponentFormFactor := 1.0
	rimDeterrenceFactor := 1.0
	opponentTeam := s.getOpponentTeamCode(player, allPlayers)
	matchupDetail := s.calcMatchupFactorWithContext(
		stats,
		opponentTeam,
		baseValue,
		player.Position,
		teamMatchupMap,
		dvpFactorMap,
	)
	matchupFactor = matchupDetail.MatchupFactor
	defRatingFactor = matchupDetail.DefRatingFactor
	paceFactor = matchupDetail.PaceFactor
	dvpFactor = matchupDetail.DvPFactor
	historyFactor = matchupDetail.HistoryFactor
	opponentFormFactor = matchupDetail.OpponentFormFactor
	rimDeterrenceFactor = matchupDetail.RimDeterrenceFactor

	defenseAnchorFactor := s.calcOpponentDefenseAnchorFactor(
		player,
		allPlayers,
		txPlayerIDMap,
		gameStatsMap,
	)

	// Step 5: 因素5 — 球队阵容上下文 (TeamContextFactor)
	teamContextFactor := s.calcTeamContextFactor(player, allPlayers, injuryMap)

	// Step 6: 因素6 — 主客场因子 (HomeAwayFactor)
	homeAwayFactor := s.calcHomeAwayFactor(player, txPlayerID, gameStatsMap)

	// Step 7: 额外因子 — 上场时间趋势、使用率趋势、稳定性、赛程疲劳
	minutesFactor := 1.0
	usageFactor := 1.0
	stabilityFactor := 1.0
	defenseUpsideFactor := 1.0
	roleSecurityFactor := 1.0
	dataReliabilityFactor := 1.0
	fatigueFactor := 1.0
	seasonStats := seasonStatsMap[txPlayerID]
	if txPlayerID > 0 {
		stats := gameStatsMap[txPlayerID]
		minutesFactor = s.calcMinutesFactor(stats, seasonStats)
		usageFactor = s.calcUsageFactor(stats)
		stabilityFactor = s.calcStabilityFactor(stats)
		defenseUpsideFactor = s.calcDefenseUpsideFactor(stats, seasonStats, player.Position)
		roleSecurityFactor = s.calcRoleSecurityFactor(stats, seasonStats, player.Salary)
		fatigueFactor = s.calcFatigueFactor(stats, player.GameDate)
	}
	dataReliabilityFactor = calcDataReliabilityFactor(len(stats), dbPlayer, seasonStats, player.Salary)

	// 新增因子：多面手、爆发力、稳定保底
	versatilityFactor := 1.0
	explosivenessFactor := 1.0
	stableFloorFactor := 1.0
	if txPlayerID > 0 && len(stats) > 0 {
		// 多面手因子：基于最近 5 场场均数据
		versatilityFactor = calcRecentVersatilityFactor(stats, player.Position)
		// 爆发力因子：基于历史表现
		explosivenessFactor = calcExplosivenessFactor(stats)
		// 稳定保底因子：识别稳定但上限低的球员
		stableFloorFactor = calcStableFloorFactor(stats)
	}

	factorConfidence := calcPredictiveFactorConfidence(recentProfile, stabilityFactor, roleSecurityFactor, dataReliabilityFactor)

	statusTrend = shrinkTowardsOne(statusTrend, factorConfidence*0.65)
	matchupFactor = shrinkTowardsOne(matchupFactor, factorConfidence*0.72)
	homeAwayFactor = shrinkTowardsOne(homeAwayFactor, factorConfidence*0.45)
	teamContextFactor = shrinkTowardsOne(teamContextFactor, factorConfidence*0.42)
	minutesFactor = shrinkTowardsOne(minutesFactor, factorConfidence*0.62)
	usageFactor = shrinkTowardsOne(usageFactor, factorConfidence*0.58)
	defenseUpsideFactor = shrinkTowardsOne(defenseUpsideFactor, factorConfidence*0.52)
	defenseAnchorFactor = shrinkTowardsOne(defenseAnchorFactor, factorConfidence*0.75)
	fatigueFactor = shrinkTowardsOne(fatigueFactor, factorConfidence*0.78)
	versatilityFactor = shrinkTowardsOne(versatilityFactor, factorConfidence*0.55)
	explosivenessFactor = shrinkTowardsOne(explosivenessFactor, factorConfidence*0.50)
	stableFloorFactor = shrinkTowardsOne(stableFloorFactor, factorConfidence*0.60)

	matchupFactor, defenseAnchorFactor = softenEliteFrontcourtNegativeFactors(
		player,
		baseValue,
		matchupFactor,
		defenseAnchorFactor,
		minutesFactor,
		usageFactor,
		defenseUpsideFactor,
	)
	archetypeFactor := calcArchetypeFactor(
		player,
		baseValue,
		minutesFactor,
		usageFactor,
		stabilityFactor,
		defenseUpsideFactor,
		roleSecurityFactor,
		dataReliabilityFactor,
		teamContextFactor,
		recentProfile,
	)
	archetypeFactor = shrinkTowardsOne(archetypeFactor, factorConfidence*0.48)

	// Step 8: 因素2 — 比赛取消风险 (GameRiskFactor)
	gameRiskFactor := 1.0 // NBA 室内运动，默认无风险

	// Step 9: 综合计算（所有因子连乘）
	dynamicMultiplier := statusTrend * matchupFactor * homeAwayFactor * teamContextFactor *
		minutesFactor * usageFactor * stabilityFactor * defenseUpsideFactor *
		archetypeFactor * roleSecurityFactor * dataReliabilityFactor *
		defenseAnchorFactor * fatigueFactor * versatilityFactor *
		explosivenessFactor * stableFloorFactor
	dynamicMultiplier = clampDynamicMultiplier(dynamicMultiplier, len(stats))

	predictedPower := baseValue * availabilityScore * dynamicMultiplier * gameRiskFactor
	predictedPower = calibratePredictedPower(predictedPower, baseValue, recentProfile, len(stats))
	predictedPower = applyStableStarLift(
		player,
		predictedPower,
		baseValue,
		minutesFactor,
		usageFactor,
		stabilityFactor,
		defenseUpsideFactor,
		roleSecurityFactor,
		dataReliabilityFactor,
		defenseAnchorFactor,
	)
	optimizedPower := s.calcConservativePower(
		predictedPower,
		stats,
		availabilityScore,
		roleSecurityFactor,
		dataReliabilityFactor,
	)
	optimizedPower = adjustOptimizedPowerForArchetype(
		player,
		predictedPower,
		optimizedPower,
		baseValue,
		minutesFactor,
		usageFactor,
		defenseAnchorFactor,
		archetypeFactor,
	)

	return PlayerPrediction{
		PredictedPower:        roundTo(predictedPower, 1),
		OptimizedPower:        roundTo(optimizedPower, 1),
		BaseValue:             roundTo(baseValue, 1),
		AvailabilityScore:     availabilityScore,
		StatusTrend:           roundTo(statusTrend, 2),
		MatchupFactor:         roundTo(matchupFactor, 2),
		DefRatingFactor:       roundTo(defRatingFactor, 2),
		PaceFactor:            roundTo(paceFactor, 2),
		DvPFactor:             roundTo(dvpFactor, 2),
		HistoryFactor:         roundTo(historyFactor, 2),
		OpponentFormFactor:    roundTo(opponentFormFactor, 2),
		RimDeterrenceFactor:   roundTo(rimDeterrenceFactor, 2),
		DefenseAnchorFactor:   roundTo(defenseAnchorFactor, 2),
		HomeAwayFactor:        roundTo(homeAwayFactor, 2),
		TeamContextFactor:     roundTo(teamContextFactor, 2),
		MinutesFactor:         roundTo(minutesFactor, 2),
		UsageFactor:           roundTo(usageFactor, 2),
		StabilityFactor:       roundTo(stabilityFactor, 2),
		DefenseUpsideFactor:   roundTo(defenseUpsideFactor, 2),
		ArchetypeFactor:       roundTo(archetypeFactor, 2),
		RoleSecurityFactor:    roundTo(roleSecurityFactor, 2),
		DataReliabilityFactor: roundTo(dataReliabilityFactor, 2),
		TeamExposureFactor:    1.0,
		FatigueFactor:         roundTo(fatigueFactor, 2),
		GameRiskFactor:        roundTo(gameRiskFactor, 2),
		Upside3:               recentProfile.Upside3,
		Upside5:               recentProfile.Upside5,
		VersatilityFactor:     roundTo(versatilityFactor, 2),
		ExplosivenessFactor:   roundTo(explosivenessFactor, 2),
		StableFloorFactor:     roundTo(stableFloorFactor, 2),
	}
}

// --- 近期战力分析与基础值校准 ---

func calcRecentPowerAverages(stats []entity.PlayerGameStats) (float64, float64) {
	if len(stats) == 0 {
		return 0, 0
	}
	count5 := min(5, len(stats))
	count10 := min(10, len(stats))

	sum5 := 0.0
	sum10 := 0.0
	for i := range count10 {
		power := calcPowerFromStats(stats[i])
		sum10 += power
		if i < count5 {
			sum5 += power
		}
	}

	avg5 := 0.0
	if count5 > 0 {
		avg5 = sum5 / float64(count5)
	}
	avg10 := 0.0
	if count10 > 0 {
		avg10 = sum10 / float64(count10)
	}
	return avg5, avg10
}

func calcRecentPowerProfile(stats []entity.PlayerGameStats) recentPowerProfile {
	profile := recentPowerProfile{}
	if len(stats) == 0 {
		return profile
	}

	powers := make([]float64, 0, min(10, len(stats)))
	for i := 0; i < min(10, len(stats)); i++ {
		power := calcPowerFromStats(stats[i])
		if power <= 0 {
			continue
		}
		powers = append(powers, power)
	}
	if len(powers) == 0 {
		return profile
	}

	profile.SampleCount = len(powers)
	for i := 0; i < min(3, len(powers)); i++ {
		profile.Avg3 += powers[i]
	}
	profile.Avg3 /= float64(min(3, len(powers)))
	for i := 0; i < min(5, len(powers)); i++ {
		profile.Avg5 += powers[i]
	}
	profile.Avg5 /= float64(min(5, len(powers)))
	for _, power := range powers {
		profile.Avg10 += power
	}
	profile.Avg10 /= float64(len(powers))

	medianWindow := append([]float64(nil), powers[:min(5, len(powers))]...)
	sort.Float64s(medianWindow)
	mid := len(medianWindow) / 2
	if len(medianWindow)%2 == 0 {
		profile.Median5 = (medianWindow[mid-1] + medianWindow[mid]) / 2.0
	} else {
		profile.Median5 = medianWindow[mid]
	}

	window := powers[:min(5, len(powers))]
	mean := 0.0
	for _, power := range window {
		mean += power
	}
	mean /= float64(len(window))
	if mean > 0 {
		variance := 0.0
		for _, power := range window {
			diff := power - mean
			variance += diff * diff
		}
		profile.Volatility = math.Sqrt(variance/float64(len(window))) / mean
	}

	// 计算爆发系数：近 3 场/近 5 场最高表现与均值的比率
	if len(powers) >= 3 {
		max3 := powers[0]
		for _, p := range powers[:min(3, len(powers))] {
			if p > max3 {
				max3 = p
			}
		}
		if profile.Avg3 > 0 {
			profile.Upside3 = max3 / profile.Avg3
		}
	}
	if len(powers) >= 5 {
		max5 := powers[0]
		for _, p := range powers[:min(5, len(powers))] {
			if p > max5 {
				max5 = p
			}
		}
		if profile.Avg5 > 0 {
			profile.Upside5 = max5 / profile.Avg5
		}
	}

	return profile
}

func stabilizeBaseValue(baseValue, gamePower, recentPower5, recentPower10 float64, statCount int) float64 {
	if recentPower10 <= 0 || statCount < recentBaseMinGames {
		return baseValue
	}

	recentComposite := recentPower10
	if recentPower5 > 0 {
		recentComposite = 0.65*recentPower5 + 0.35*recentPower10
	}

	reliability := clamp(float64(min(statCount, 10))/10.0, 0.30, 1.0)
	recentWeight := 0.25 + 0.35*reliability
	gameWeight := 0.10
	baseWeight := 1.0 - recentWeight - gameWeight
	if baseWeight < 0.20 {
		baseWeight = 0.20
		gameWeight = 0.10
		recentWeight = 0.70
	}

	mixed := baseWeight*baseValue + recentWeight*recentComposite + gameWeight*gamePower
	lower := recentPower10 * 0.78
	upper := recentPower10 * 1.28
	return clamp(mixed, lower, upper)
}

func buildRobustBaseValue(baseValue, gamePower float64, profile recentPowerProfile) float64 {
	if profile.SampleCount == 0 {
		return baseValue
	}

	type weightedPoint struct {
		value  float64
		weight float64
	}
	points := []weightedPoint{
		{value: baseValue, weight: 0.32},
		{value: gamePower, weight: 0.08},
	}
	if profile.Avg10 > 0 {
		points = append(points, weightedPoint{value: profile.Avg10, weight: 0.30})
	}
	if profile.Avg5 > 0 {
		points = append(points, weightedPoint{value: profile.Avg5, weight: 0.18})
	}
	if profile.Median5 > 0 {
		points = append(points, weightedPoint{value: profile.Median5, weight: 0.12})
	}

	totalWeight := 0.0
	mixed := 0.0
	for _, point := range points {
		if point.value <= 0 || point.weight <= 0 {
			continue
		}
		totalWeight += point.weight
		mixed += point.value * point.weight
	}
	if totalWeight <= 0 {
		return baseValue
	}
	mixed /= totalWeight

	anchor := profile.Avg10
	if anchor <= 0 {
		anchor = profile.Avg5
	}
	if anchor <= 0 {
		anchor = profile.Median5
	}
	if anchor <= 0 {
		return mixed
	}

	volatilityPenalty := clamp((profile.Volatility-0.18)/(0.50-0.18), 0.0, 1.0)
	lower := anchor * (0.84 - 0.03*volatilityPenalty)
	upper := anchor * (1.18 - 0.05*volatilityPenalty)
	if upper < lower {
		upper = lower
	}
	return clamp(mixed, lower, upper)
}

func calibratePredictedPower(predicted, baseValue float64, profile recentPowerProfile, statCount int) float64 {
	if predicted <= 0 {
		return predicted
	}
	anchor := profile.Avg10
	if anchor <= 0 {
		anchor = profile.Avg5
	}
	if anchor <= 0 {
		anchor = profile.Median5
	}
	if anchor <= 0 || statCount < 5 {
		// 无足够近期样本时，避免过拟合因素把结果拉太离谱。
		modelWeight := 0.82
		if statCount == 0 {
			modelWeight = 0.66
		} else if statCount < 3 {
			modelWeight = 0.74
		}
		return modelWeight*predicted + (1-modelWeight)*baseValue
	}

	// 增加近期 3 场 avg 的权重，减少 10 场 avg 的权重
	robustAnchor := 0.50*anchor + 0.35*profile.Avg3 + 0.15*profile.Median5
	reliability := clamp(float64(min(statCount, 10))/10.0, 0.40, 1.0)
	volatilityPenalty := clamp((profile.Volatility-0.18)/(0.50-0.18), 0.0, 1.0)
	modelWeight := 0.55 + 0.20*reliability - 0.08*volatilityPenalty
	modelWeight = clamp(modelWeight, 0.50, 0.80)
	anchored := modelWeight*predicted + (1-modelWeight)*robustAnchor
	lower := robustAnchor * (0.80 - 0.02*volatilityPenalty)
	upper := robustAnchor * (1.32 - 0.08*volatilityPenalty)
	if upper < lower {
		upper = lower
	}
	return clamp(anchored, lower, upper)
}
