package service

import (
	"math"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/entity"
)

// 因素权重与阈值常量化
const (
	// 趋势相关 (Trend)
	StatusTrendMin         = 0.88
	StatusTrendMax         = 1.12
	RecentStatusWeight     = 0.50
	DBTrendSampleThreshold = 5
	TrendRecent3Games      = 3
	TrendRecent10Games     = 10

	// 比赛匹配相关 (Matchup)
	MatchupWeightDefRating    = 0.43
	MatchupWeightPace         = 0.23
	MatchupWeightHistory      = 0.16
	MatchupWeightOpponentForm = 0.18
	MatchupMinFactor          = 0.86
	MatchupMaxFactor          = 1.14

	// DvP 置信度相关
	DVPMaxGames        = 24
	TeamMetricMaxGames = 7
	ConfidenceMin      = 0.35

	// 防守压制 (Defense Impact)
	FrontcourtRimWeight       = 0.72
	FrontcourtPerimeterWeight = 0.28
	BackcourtRimWeight        = 0.35
	BackcourtPerimeterWeight  = 0.65
	DefenseAnchorRimWeight    = 1.25
	DefenseAnchorStealWeight  = 0.45
	DefenseAnchorBackWeight   = 0.95

	// 角色与地位
	EliteFrontcourtBasePower = 45.0
	EliteFrontcourtSalaryMin = 40
	StableStarBasePower      = 46.0
	StableStarSalaryMin      = 35

	// 历史交手
	HistoryVsGamesThreshold = 3
	HistoryMinFactor        = 0.90
	HistoryMaxFactor        = 1.10

	// 赛程疲劳
	FatigueB2BPenalty         = 0.06 // 1.0 - 0.94
	FatigueRest2DaysFactor    = 0.98
	Fatigue4Days3GamesPenalty = 0.03
)

// resolveAvailabilityScore 计算球员出场可用性分数 [0, 1]。
// CombatPower 为 0 直接返回 0（未上场），否则根据伤病状态调整。
func resolveAvailabilityScore(
	player entity.NBAGamePlayer,
	injuryMap map[uint]crawler.InjuryReport,
) float64 {
	if player.CombatPower == 0 {
		return 0.0
	}
	if injury, ok := injuryMap[player.NBAPlayerID]; ok {
		availabilityScore := crawler.StatusToAvailabilityScore(injury.Status)
		return rebalanceAvailabilityScoreWithDescription(injury.Status, injury.Description, availabilityScore)
	}
	return 1.0
}

// calcStatusTrend 计算近期状态趋势因子，综合 DB 历史均值和近期比赛数据。
func calcStatusTrend(dbPlayer *entity.Player, stats []entity.PlayerGameStats) float64 {
	dbTrend := 1.0
	if dbPlayer != nil && dbPlayer.PowerPer10 > 0 && dbPlayer.PowerPer5 > 0 {
		dbTrend = clamp(dbPlayer.PowerPer5/dbPlayer.PowerPer10, StatusTrendMin, StatusTrendMax)
	}

	recentTrend := 1.0
	if len(stats) >= 5 {
		recent3 := 0.0
		recent10 := 0.0
		count3 := min(3, len(stats))
		count10 := min(10, len(stats))
		for i := 0; i < count10; i++ {
			power := calcPowerFromStats(stats[i])
			recent10 += power
			if i < count3 {
				recent3 += power
			}
		}
		if count3 > 0 && count10 > 0 {
			avg3 := recent3 / float64(count3)
			avg10 := recent10 / float64(count10)
			if avg10 > 0 {
				recentTrend = clamp(avg3/avg10, StatusTrendMin, StatusTrendMax)
			}
		}
	}

	// 增加近期状态权重
	recentWeight := 0.0
	if len(stats) >= DBTrendSampleThreshold {
		recentWeight = RecentStatusWeight
	}
	return clamp(dbTrend*(1-recentWeight)+recentTrend*recentWeight, StatusTrendMin, StatusTrendMax)
}

// clampDynamicMultiplier 根据样本数限制动态乘数范围，样本越少限制越严。
func clampDynamicMultiplier(multiplier float64, statCount int) float64 {
	if statCount >= 8 {
		return clamp(multiplier, 0.80, 1.18)
	}
	if statCount >= 5 {
		return clamp(multiplier, 0.76, 1.14)
	}
	if statCount >= 3 {
		return clamp(multiplier, 0.70, 1.10)
	}
	return clamp(multiplier, 0.58, 1.05)
}

// shrinkTowardsOne 将因子值向 1.0 收缩，收缩程度由信心度 confidence 控制。
// confidence=0 时完全收缩到 1.0，confidence=1 时保持原值。使用平方压缩低信心区域。
func shrinkTowardsOne(value, confidence float64) float64 {
	// 调整收缩逻辑：高信心时保持更多原始值
	conf := clamp(confidence, 0.0, 1.0)
	// 使用平方压缩低信心区域，让高信心保持更多
	return 1.0 + (value-1.0)*conf*conf
}

// calcMatchupFactor 计算简化版对手匹配因子（仅使用历史交手数据）。
func (s *LineupRecommendService) calcMatchupFactor(stats []entity.PlayerGameStats, opponentTeam string, baseValue float64) float64 {
	targetTeam := normalizeTeamCode(opponentTeam)
	if targetTeam == "" {
		return 1.0
	}
	if countVsTeamGames(stats, targetTeam) < 3 {
		return 1.0
	}
	return s.calcHistoryFactor(stats, opponentTeam, baseValue)
}

// calcMatchupFactorWithContext 计算综合对手匹配因子，融合防守效率、节奏、DVP、历史交手、
// 近期状态、篮下威慑和外线干扰等多维度指标。
func (s *LineupRecommendService) calcMatchupFactorWithContext(
	stats []entity.PlayerGameStats,
	opponentTeam string,
	baseValue float64,
	position uint,
	teamMatchupMap map[string]teamMatchupMetric,
	dvpFactorMap map[string]map[uint]positionDVPMetric,
) matchupFactorDetail {
	targetTeam := normalizeTeamCode(opponentTeam)
	if targetTeam == "" {
		return matchupFactorDetail{
			MatchupFactor:         1.0,
			DefRatingFactor:       1.0,
			PaceFactor:            1.0,
			DvPFactor:             1.0,
			HistoryFactor:         1.0,
			OpponentFormFactor:    1.0,
			RimDeterrenceFactor:   1.0,
			PerimeterImpactFactor: 1.0,
		}
	}

	historyFactor := s.calcHistoryFactor(stats, targetTeam, baseValue)
	defRatingFactor := 1.0
	paceFactor := 1.0
	opponentFormFactor := 1.0
	rimDeterrenceFactor := 1.0
	perimeterImpactFactor := 1.0
	if metric, ok := teamMatchupMap[targetTeam]; ok {
		defRatingFactor = metric.DefRatingFactor
		paceFactor = metric.PaceFactor
		opponentFormFactor = metric.OpponentFormFactor
		rimDeterrenceFactor = metric.RimDeterrenceFactor
		perimeterImpactFactor = metric.PerimeterImpactFactor
		if defRatingFactor <= 0 {
			defRatingFactor = 1.0
		}
		if paceFactor <= 0 {
			paceFactor = 1.0
		}
		if opponentFormFactor <= 0 {
			opponentFormFactor = 1.0
		}
		if rimDeterrenceFactor <= 0 {
			rimDeterrenceFactor = 1.0
		}
		if perimeterImpactFactor <= 0 {
			perimeterImpactFactor = 1.0
		}
		confidence := float64(min(metric.SampleCount, teamMetricMaxGames)) / float64(teamMetricMaxGames)
		confidence = clamp(confidence, ConfidenceMin, 1.0)
		defRatingFactor = shrinkTowardsOne(defRatingFactor, confidence)
		paceFactor = shrinkTowardsOne(paceFactor, confidence)
		opponentFormFactor = shrinkTowardsOne(opponentFormFactor, confidence)
		rimDeterrenceFactor = shrinkTowardsOne(rimDeterrenceFactor, confidence)
		perimeterImpactFactor = shrinkTowardsOne(perimeterImpactFactor, confidence)
	}

	dvpFactor := 1.0
	positionGroup := normalizePositionGroup(position)
	if byPosition, ok := dvpFactorMap[targetTeam]; ok {
		if metric, ok := byPosition[positionGroup]; ok {
			dvpFactor = metric.Factor
			confidence := float64(min(metric.SampleCount, 24)) / 24.0
			confidence = clamp(confidence, 0.25, 1.0)
			dvpFactor = shrinkTowardsOne(dvpFactor, confidence)
		}
	}

	historyConfidence := 0.20 + float64(min(countVsTeamGames(stats, targetTeam), 8))/10.0
	historyConfidence = clamp(historyConfidence, 0.20, 0.92)
	historyFactor = shrinkTowardsOne(historyFactor, historyConfidence)

	disruptionFactor := 1.0
	if positionGroup == 0 {
		disruptionFactor = FrontcourtRimWeight*rimDeterrenceFactor + FrontcourtPerimeterWeight*perimeterImpactFactor
	} else {
		disruptionFactor = BackcourtRimWeight*rimDeterrenceFactor + BackcourtPerimeterWeight*perimeterImpactFactor
	}
	disruptionFactor = clamp(disruptionFactor, 0.90, 1.08)

	baseMatchup := MatchupWeightDefRating*defRatingFactor + MatchupWeightPace*paceFactor + MatchupWeightHistory*historyFactor + MatchupWeightOpponentForm*opponentFormFactor
	matchupFactor := clamp(baseMatchup*dvpFactor*disruptionFactor, MatchupMinFactor, MatchupMaxFactor)
	return matchupFactorDetail{
		MatchupFactor:         matchupFactor,
		DefRatingFactor:       defRatingFactor,
		PaceFactor:            paceFactor,
		DvPFactor:             dvpFactor,
		HistoryFactor:         historyFactor,
		OpponentFormFactor:    opponentFormFactor,
		RimDeterrenceFactor:   rimDeterrenceFactor,
		PerimeterImpactFactor: perimeterImpactFactor,
	}
}

// normalizePositionGroup 将位置编号标准化为组：0=前场（中锋/前锋），1=后场（后卫）。
func normalizePositionGroup(position uint) uint {
	if position == 1 {
		return 1
	}
	return 0
}

// calcHistoryFactor 根据球员对阵特定对手的历史战力表现计算因子。
// 样本数越多置信度越高，因子范围越宽。
func (s *LineupRecommendService) calcHistoryFactor(stats []entity.PlayerGameStats, opponentTeam string, baseValue float64) float64 {
	if len(stats) == 0 || baseValue <= 0 {
		return 1.0
	}

	// 历史对阵因子：该球员对阵该对手的历史战力均值 / 基础战力。
	targetTeam := normalizeTeamCode(opponentTeam)
	if targetTeam == "" {
		return 1.0
	}

	vsGames := filterVsTeamGames(stats, targetTeam)

	if len(vsGames) >= 3 {
		totalPower := 0.0
		for _, g := range vsGames {
			totalPower += calcPowerFromStats(g)
		}
		avgPower := totalPower / float64(len(vsGames))
		return clamp(avgPower/baseValue, HistoryMinFactor, HistoryMaxFactor)
	}

	if len(vsGames) == 2 {
		totalPower := calcPowerFromStats(vsGames[0]) + calcPowerFromStats(vsGames[1])
		avgPower := totalPower / 2.0
		return clamp(avgPower/baseValue, 0.94, 1.06)
	}

	if len(vsGames) == 1 {
		oneGame := calcPowerFromStats(vsGames[0])
		return clamp(oneGame/baseValue, 0.96, 1.04)
	}

	return 1.0
}

func filterVsTeamGames(stats []entity.PlayerGameStats, targetTeam string) []entity.PlayerGameStats {
	var vsGames []entity.PlayerGameStats
	for _, g := range stats {
		if normalizeTeamCode(g.VsTeamName) == targetTeam {
			vsGames = append(vsGames, g)
		}
	}
	return vsGames
}

func countVsTeamGames(stats []entity.PlayerGameStats, targetTeam string) int {
	return len(filterVsTeamGames(stats, targetTeam))
}

// calcOpponentDefenseAnchorFactor 评估对手阵中防守核心球员对当前球员的压制效果。
// 根据对手的盖帽、抢断、上场时间等判断防守锚点强度。
func (s *LineupRecommendService) calcOpponentDefenseAnchorFactor(
	player entity.NBAGamePlayer,
	allPlayers []entity.NBAGamePlayer,
	txPlayerIDMap map[uint]uint,
	gameStatsMap map[uint][]entity.PlayerGameStats,
) float64 {
	positionGroup := normalizePositionGroup(player.Position)
	maxImpact := 0.0

	for _, opponent := range allPlayers {
		if opponent.MatchID != player.MatchID || opponent.NBATeamID == player.NBATeamID {
			continue
		}
		txOpponentID := txPlayerIDMap[opponent.NBAPlayerID]
		if txOpponentID == 0 {
			continue
		}
		stats := gameStatsMap[txOpponentID]
		if len(stats) == 0 {
			continue
		}

		recentCount := min(8, len(stats))
		if recentCount == 0 {
			continue
		}

		blocksTotal := 0.0
		stealsTotal := 0.0
		minutesTotal := 0.0
		for i := range recentCount {
			blocksTotal += float64(stats[i].Blocks)
			stealsTotal += float64(stats[i].Steals)
			minutesTotal += float64(stats[i].Minutes)
		}

		blocksAvg := blocksTotal / float64(recentCount)
		stealsAvg := stealsTotal / float64(recentCount)
		minutesAvg := minutesTotal / float64(recentCount)
		reliability := clamp(float64(recentCount)/8.0, 0.35, 1.0)

		impact := 0.0
		if positionGroup == 0 {
			impact = DefenseAnchorRimWeight*blocksAvg + DefenseAnchorStealWeight*stealsAvg
		} else {
			impact = DefenseAnchorStealWeight*blocksAvg + DefenseAnchorBackWeight*stealsAvg
		}
		if minutesAvg >= 30 {
			impact += 0.20
		} else if minutesAvg >= 24 {
			impact += 0.10
		}
		if opponent.Salary >= 35 {
			impact += 0.10
		}
		if opponent.CombatPower >= 45 {
			impact += 0.08
		}

		impact *= reliability
		if impact > maxImpact {
			maxImpact = impact
		}
	}

	if maxImpact <= 0 {
		return 1.0
	}

	penaltySlope := 0.032
	if positionGroup == 1 {
		penaltySlope = 0.020
	}
	penalty := penaltySlope * math.Max(0.0, maxImpact-1.10)
	return clamp(1.0-penalty, 0.88, 1.02)
}

// softenEliteFrontcourtNegativeFactors 对高价值前场球员软化负面匹配因子和防守锚点因子。
// 当前场球员基础值高、上场时间多、使用率高时，减少负面因子的惩罚力度。
func softenEliteFrontcourtNegativeFactors(
	player entity.NBAGamePlayer,
	baseValue float64,
	matchupFactor float64,
	defenseAnchorFactor float64,
	minutesFactor float64,
	usageFactor float64,
	defenseUpsideFactor float64,
) (float64, float64) {
	if normalizePositionGroup(player.Position) != 0 {
		return matchupFactor, defenseAnchorFactor
	}
	if baseValue < EliteFrontcourtBasePower || minutesFactor < 1.03 || usageFactor < 1.03 {
		return matchupFactor, defenseAnchorFactor
	}

	resilience := 0.30
	if baseValue >= 50 {
		resilience += 0.18
	}
	if player.Salary >= EliteFrontcourtSalaryMin {
		resilience += 0.10
	}
	if defenseUpsideFactor >= 1.10 {
		resilience += 0.14
	}
	resilience = clamp(resilience, 0.30, 0.62)

	if matchupFactor < 1.0 {
		matchupFactor = 1.0 + (matchupFactor-1.0)*(1.0-resilience)
	}
	if defenseAnchorFactor < 1.0 {
		matchConfidence := clamp(resilience*0.80, 0.18, 0.48)
		defenseAnchorFactor = 1.0 + (defenseAnchorFactor-1.0)*(1.0-matchConfidence)
	}

	return clamp(matchupFactor, 0.90, 1.14), clamp(defenseAnchorFactor, 0.90, 1.03)
}

// calcArchetypeFactor 根据球员原型（前场廉价工兵、后场低薪球员等）调整预测值。
// 识别不同类型球员的价值特征，给予相应的加成或惩罚。
func calcArchetypeFactor(
	player entity.NBAGamePlayer,
	baseValue float64,
	minutesFactor float64,
	usageFactor float64,
	stabilityFactor float64,
	defenseUpsideFactor float64,
	roleSecurityFactor float64,
	dataReliabilityFactor float64,
	teamContextFactor float64,
	profile recentPowerProfile,
) float64 {
	factor := 1.0
	positionGroup := normalizePositionGroup(player.Position)

	if positionGroup == 0 {
		if player.Salary <= 20 && player.CombatPower >= 18 {
			factor += clamp((minutesFactor-1.0)*0.40, 0.0, 0.05)
			factor += clamp((defenseUpsideFactor-1.0)*0.45, 0.0, 0.06)
			factor += clamp((teamContextFactor-1.0)*0.80, 0.0, 0.04)
		}
		if player.Salary <= 30 && baseValue >= 34 && minutesFactor >= 1.02 &&
			(profile.Upside5 >= 1.35 || profile.Upside3 >= 1.28) {
			factor += clamp((minutesFactor-1.02)*0.60, 0.0, 0.03)
			factor += clamp((defenseUpsideFactor-1.0)*0.35, 0.0, 0.03)
			factor += clamp((profile.Upside5-1.35)*0.12, 0.0, 0.04)
			factor += clamp((profile.Upside3-1.28)*0.10, 0.0, 0.03)
		}
		if player.Salary <= 12 && player.CombatPower >= 18 {
			factor += 0.05
		}
		if dataReliabilityFactor < 0.78 && player.CombatPower >= 20 {
			factor += clamp((0.82-dataReliabilityFactor)*0.55, 0.0, 0.07)
		}
		if baseValue >= 48 && minutesFactor >= 1.05 && usageFactor >= 1.05 {
			factor += 0.03
		}
		if stabilityFactor < 0.95 {
			factor -= clamp((0.95-stabilityFactor)*0.25, 0.0, 0.03)
		}
		// 爆发型球员加成：Upside3≥1.5 时给予额外加成
		if profile.Upside3 >= 1.5 {
			factor += clamp((profile.Upside3-1.5)*0.08, 0.0, 0.05)
		}
		if teamContextFactor > 1.05 {
			factor += clamp((teamContextFactor-1.05)*0.80, 0.0, 0.08)
		}
		return clamp(factor, 0.94, 1.15)
	}

	// 非核心位置球员：识别低薪爆发型球员
	valueRatio := 0.0
	if player.Salary > 0 {
		valueRatio = baseValue / float64(player.Salary)
	}
	// 低薪高能且有爆发力的球员给予加成
	if player.Salary <= 20 {
		if valueRatio >= 3.0 && profile.Upside3 >= 1.35 {
			factor += clamp((profile.Upside3-1.35)*0.15, 0.0, 0.08)
		} else if profile.Upside3 >= 1.45 {
			factor += clamp((profile.Upside3-1.45)*0.12, 0.0, 0.07)
		}
		if teamContextFactor > 1.05 {
			factor += clamp((teamContextFactor-1.05)*0.80, 0.0, 0.08)
		}
	}

	if player.Salary <= 12 {
		factor -= clamp(math.Max(0.0, minutesFactor-1.0)*0.35+math.Max(0.0, usageFactor-1.0)*0.35, 0.0, 0.08)
		factor -= clamp(math.Max(0.0, defenseUpsideFactor-1.0)*0.30, 0.0, 0.03)
		if roleSecurityFactor < 1.0 {
			factor -= clamp((1.0-roleSecurityFactor)*0.20, 0.0, 0.04)
		}
	}

	if player.Salary <= 18 && teamContextFactor > 1.04 && stabilityFactor < 1.0 {
		factor -= clamp((teamContextFactor-1.04)*0.35, 0.0, 0.03)
	}

	return clamp(factor, 0.90, 1.08)
}

// adjustOptimizedPowerForArchetype 根据球员原型调整保守估值与预测值之间的差距。
// 高价值前场球员在满足条件时可获得更多预测上行空间。
func adjustOptimizedPowerForArchetype(
	player entity.NBAGamePlayer,
	predictedPower float64,
	optimizedPower float64,
	baseValue float64,
	minutesFactor float64,
	usageFactor float64,
	defenseAnchorFactor float64,
	archetypeFactor float64,
) float64 {
	if predictedPower <= 0 || optimizedPower <= 0 || predictedPower <= optimizedPower {
		return optimizedPower
	}

	positionGroup := normalizePositionGroup(player.Position)
	upsideGap := predictedPower - optimizedPower

	if positionGroup == 0 && baseValue >= 45 && minutesFactor >= 1.03 && usageFactor >= 1.03 && defenseAnchorFactor >= 0.93 {
		optimizedPower += upsideGap * 0.30
	}
	if player.Salary <= 20 && archetypeFactor > 1.02 && defenseAnchorFactor >= 0.92 {
		optimizedPower += upsideGap * clamp((archetypeFactor-1.0)*3.5+0.1, 0.0, 0.55)
	}
	if archetypeFactor >= 1.06 {
		optimizedPower += upsideGap * 0.35 // 为表现突出的球员提供保底
	}

	return clamp(optimizedPower, predictedPower*0.62, predictedPower)
}

// calcTeamContextFactor 计算球队阵容上下文因子。
// 当同队核心球员缺阵时，在场球员可能获得更多使用机会（正面影响）。
func (s *LineupRecommendService) calcTeamContextFactor(
	player entity.NBAGamePlayer,
	allPlayers []entity.NBAGamePlayer,
	injuryMap map[uint]crawler.InjuryReport,
) float64 {
	// 统计同队球员中"明确缺阵或极大概率缺阵"的工资占比。
	var totalTeamSalary, absentSalary float64
	for _, p := range allPlayers {
		if p.NBATeamID == player.NBATeamID && p.NBAPlayerID != player.NBAPlayerID {
			totalTeamSalary += float64(p.Salary)
			if resolveAvailabilityScore(p, injuryMap) == 0 {
				absentSalary += float64(p.Salary)
			}
		}
	}

	if totalTeamSalary <= 0 {
		return 1.0
	}

	absentRatio := absentSalary / totalTeamSalary

	// 缺阵球员工资占比越高，在场球员可能获得更多机会
	return clamp(1.0+absentRatio*0.25, 1.0, 1.15)
}

// calcHomeAwayFactor 计算主客场因子。
// 优先使用该球员历史主客场战力差异，样本不足时使用默认值（主场+2%/客场-2%）。
func (s *LineupRecommendService) calcHomeAwayFactor(player entity.NBAGamePlayer, txPlayerID uint, gameStatsMap map[uint][]entity.PlayerGameStats) float64 {
	defaultFactor := 1.0
	if player.IsHome {
		defaultFactor = 1.02
	} else {
		defaultFactor = 0.98
	}

	if txPlayerID == 0 {
		return defaultFactor
	}

	stats := gameStatsMap[txPlayerID]
	if len(stats) < 5 {
		return defaultFactor
	}

	var homeTotal, awayTotal float64
	var homeCount, awayCount int
	for _, g := range stats {
		power := calcPowerFromStats(g)
		if g.IsHome {
			homeTotal += power
			homeCount++
		} else {
			awayTotal += power
			awayCount++
		}
	}

	if homeCount >= 3 && awayCount >= 3 {
		homeAvg := homeTotal / float64(homeCount)
		awayAvg := awayTotal / float64(awayCount)
		overallAvg := (homeAvg + awayAvg) / 2
		if overallAvg > 0 {
			if player.IsHome {
				return clamp(homeAvg/overallAvg, 0.95, 1.08)
			}
			return clamp(awayAvg/overallAvg, 0.92, 1.05)
		}
	}

	return defaultFactor
}

// calcMinutesFactor 计算上场时间趋势因子。
// 比较近 3 场与赛季/长期均值，判断上场时间分配是否增加。
func (s *LineupRecommendService) calcMinutesFactor(stats []entity.PlayerGameStats, seasonStats *entity.PlayerSeasonStats) float64 {
	if len(stats) == 0 {
		return 1.0
	}

	recentCount := min(3, len(stats))
	recentMinutes := 0.0
	for i := 0; i < recentCount; i++ {
		recentMinutes += float64(stats[i].Minutes)
	}
	recentAvg := recentMinutes / float64(recentCount)
	if recentAvg <= 0 {
		return 0.90
	}

	baseline := 0.0
	if seasonStats != nil && seasonStats.Minutes > 0 {
		baseline = seasonStats.Minutes
	} else {
		baselineCount := min(10, len(stats))
		total := 0.0
		for i := 0; i < baselineCount; i++ {
			total += float64(stats[i].Minutes)
		}
		if baselineCount > 0 {
			baseline = total / float64(baselineCount)
		}
	}
	if baseline <= 0 {
		return 1.0
	}

	return clamp(recentAvg/baseline, 0.90, 1.10)
}

// calcUsageFactor 计算使用率趋势因子。
// 使用代理指标（出手+罚球+助攻）比较近 3 场与近 10 场均值。
func (s *LineupRecommendService) calcUsageFactor(stats []entity.PlayerGameStats) float64 {
	if len(stats) < 3 {
		return 1.0
	}

	recentCount := min(3, len(stats))
	totalCount := min(10, len(stats))

	recentUsage := 0.0
	for i := 0; i < recentCount; i++ {
		recentUsage += calcUsageProxyFromStats(stats[i])
	}
	totalUsage := 0.0
	for i := 0; i < totalCount; i++ {
		totalUsage += calcUsageProxyFromStats(stats[i])
	}

	recentAvg := recentUsage / float64(recentCount)
	totalAvg := totalUsage / float64(totalCount)
	if totalAvg <= 0 {
		return 1.0
	}

	return clamp(recentAvg/totalAvg, 0.92, 1.10)
}

// calcDefenseUpsideFactor 计算防守成长因子。
// 比较近期 stocks（盖帽+抢断）与赛季基线，前场球员权重偏向盖帽。
func (s *LineupRecommendService) calcDefenseUpsideFactor(
	stats []entity.PlayerGameStats,
	seasonStats *entity.PlayerSeasonStats,
	position uint,
) float64 {
	if len(stats) == 0 {
		return 1.0
	}

	recentCount := min(5, len(stats))
	recentStocks := 0.0
	recentMinutes := 0.0
	for i := 0; i < recentCount; i++ {
		recentStocks += float64(stats[i].Blocks + stats[i].Steals)
		recentMinutes += float64(stats[i].Minutes)
	}
	recentStocksAvg := recentStocks / float64(recentCount)
	recentMinutesAvg := recentMinutes / float64(recentCount)

	baselineStocks := 0.0
	if seasonStats != nil && (seasonStats.Blocks > 0 || seasonStats.Steals > 0) {
		baselineStocks = seasonStats.Blocks + seasonStats.Steals
	} else {
		window := min(10, len(stats))
		total := 0.0
		for i := 0; i < window; i++ {
			total += float64(stats[i].Blocks + stats[i].Steals)
		}
		if window > 0 {
			baselineStocks = total / float64(window)
		}
	}
	if baselineStocks <= 0 {
		return 1.0
	}

	ratio := recentStocksAvg / baselineStocks
	positionGroup := normalizePositionGroup(position)
	if positionGroup == 0 {
		ratio = clamp(ratio, 0.92, 1.14)
	} else {
		ratio = clamp(ratio, 0.94, 1.10)
	}

	if recentMinutesAvg < 20 {
		ratio = 1.0 + (ratio-1.0)*0.55
	} else if recentMinutesAvg < 26 {
		ratio = 1.0 + (ratio-1.0)*0.75
	}
	return ratio
}

// calcRoleSecurityFactor 计算角色安全因子。
// 评估球员在轮换中的地位稳定性：上场时间波动大、频繁低上场时间意味着角色不安全。
func (s *LineupRecommendService) calcRoleSecurityFactor(
	stats []entity.PlayerGameStats,
	seasonStats *entity.PlayerSeasonStats,
	salary uint,
) float64 {
	if len(stats) == 0 {
		switch {
		case seasonStats != nil && seasonStats.Minutes >= 24:
			return 1.0
		case seasonStats != nil && seasonStats.Minutes >= 18:
			return 0.95
		case salary <= 10:
			return 0.70
		case salary <= 12:
			return 0.74
		case salary <= 15:
			return 0.78
		default:
			return 0.86
		}
	}

	recentCount := min(5, len(stats))
	window := min(8, len(stats))
	recentMinutes := 0.0
	for i := 0; i < recentCount; i++ {
		recentMinutes += float64(stats[i].Minutes)
	}
	recentAvg := recentMinutes / float64(recentCount)

	baseline := 0.0
	if seasonStats != nil && seasonStats.Minutes > 0 {
		baseline = seasonStats.Minutes
	} else {
		for i := 0; i < window; i++ {
			baseline += float64(stats[i].Minutes)
		}
		baseline /= float64(window)
	}

	lowMinutesCount := 0
	veryLowMinutesCount := 0
	for i := 0; i < window; i++ {
		if stats[i].Minutes <= 14 {
			lowMinutesCount++
		}
		if stats[i].Minutes <= 8 {
			veryLowMinutesCount++
		}
	}

	factor := 1.0
	if baseline > 0 {
		if recentAvg < 0.85*baseline {
			factor -= clamp((0.85*baseline-recentAvg)/baseline, 0.0, 0.16)
		} else if recentAvg > 1.08*baseline {
			factor += clamp((recentAvg-1.08*baseline)/baseline, 0.0, 0.04)
		}
	}

	lowRate := float64(lowMinutesCount) / float64(window)
	veryLowRate := float64(veryLowMinutesCount) / float64(window)
	factor -= 0.12 * lowRate
	factor -= 0.20 * veryLowRate

	if salary <= 12 {
		factor -= 0.05 * lowRate
	}
	if window < 5 {
		factor -= 0.03
	}

	return clamp(factor, 0.72, 1.05)
}

// calcDataReliabilityFactor 计算数据可靠性因子。
// 综合历史比赛样本数、DB 锚定数据和赛季数据的可用性评估预测可信度。
func calcDataReliabilityFactor(
	statCount int,
	dbPlayer *entity.Player,
	seasonStats *entity.PlayerSeasonStats,
	salary uint,
) float64 {
	hasDBAnchor := dbPlayer != nil && (dbPlayer.PowerPer5 > 0 || dbPlayer.PowerPer10 > 0)
	hasSeasonAnchor := seasonStats != nil && seasonStats.Minutes > 0

	factor := 1.0
	switch {
	case statCount >= 8:
		factor = 1.0
	case statCount >= 5:
		if hasDBAnchor || hasSeasonAnchor {
			factor = 0.95
		} else {
			factor = 0.92
		}
	case statCount >= 3:
		if hasDBAnchor || hasSeasonAnchor {
			factor = 0.90
		} else {
			factor = 0.82
		}
	case statCount >= 1:
		if hasDBAnchor || hasSeasonAnchor {
			factor = 0.84
		} else {
			factor = 0.70
		}
	default:
		if hasDBAnchor && hasSeasonAnchor {
			factor = 0.78
		} else if hasDBAnchor || hasSeasonAnchor {
			factor = 0.64
		} else {
			factor = 0.48
		}
	}

	if salary <= 10 && statCount < 3 {
		factor -= 0.08
	} else if salary <= 15 && statCount < 5 {
		factor -= 0.04
	}

	return clamp(factor, 0.42, 1.02)
}

// calcConservativePower 计算保守估值，融合预测值与历史下限（均值-0.85*标准差）。
// 用于阵容优化时提供更稳健的选人依据。
func (s *LineupRecommendService) calcConservativePower(
	predicted float64,
	stats []entity.PlayerGameStats,
	availabilityScore float64,
	roleSecurityFactor float64,
	dataReliabilityFactor float64,
	upside3 float64,
	teamContextFactor float64,
) float64 {
	if predicted <= 0 {
		return predicted
	}

	riskAnchor := clamp(roleSecurityFactor*dataReliabilityFactor, 0.65, 1.0)
	if len(stats) < 3 {
		conservative := predicted * (0.82 + 0.18*riskAnchor)
		return clamp(conservative, predicted*0.60, predicted)
	}

	window := min(8, len(stats))
	sum := 0.0
	powers := make([]float64, 0, window)
	for i := 0; i < window; i++ {
		power := calcPowerFromStats(stats[i])
		powers = append(powers, power)
		sum += power
	}
	mean := sum / float64(window)
	variance := 0.0
	for _, p := range powers {
		diff := p - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(window))
	floorPower := math.Max(0.0, mean-0.85*stdDev)

	blended := 0.72*predicted + 0.28*floorPower
	if upside3 >= 1.4 || teamContextFactor > 1.05 {
		// 对近期状态极佳或由于队友缺阵而获得大量机会的球员，抵消低地板分的过度拉扯
		blended = math.Max(blended, predicted*0.85)
	}

	availabilityRisk := clamp(0.85+0.15*availabilityScore, 0.75, 1.0)
	conservative := blended * clamp(0.90+0.10*riskAnchor, 0.80, 1.02) * availabilityRisk

	minLimit := predicted * 0.60
	if upside3 >= 1.45 || teamContextFactor > 1.08 {
		minLimit = predicted * 0.82
	} else if upside3 >= 1.35 || teamContextFactor > 1.04 {
		minLimit = predicted * 0.72
	}
	return clamp(conservative, minLimit, predicted*1.02)
}

// calcStabilityFactor 计算表现稳定性因子。
// 基于近 5 场变异系数（CV），CV 越低越稳定、因子越高。
func (s *LineupRecommendService) calcStabilityFactor(stats []entity.PlayerGameStats) float64 {
	window := min(5, len(stats))
	if window < 3 {
		return 1.0
	}

	powers := make([]float64, 0, window)
	sum := 0.0
	for i := 0; i < window; i++ {
		power := calcPowerFromStats(stats[i])
		powers = append(powers, power)
		sum += power
	}
	mean := sum / float64(window)
	if mean <= 0 {
		return 1.0
	}

	variance := 0.0
	for _, p := range powers {
		diff := p - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(window))
	cv := stdDev / mean

	switch {
	case cv <= 0.18:
		return 1.03
	case cv >= 0.45:
		return 0.92
	default:
		ratio := (cv - 0.18) / (0.45 - 0.18)
		return 1.03 - ratio*(1.03-0.92)
	}
}

// calcFatigueFactor 计算赛程疲劳因子。
// 背靠背（间隔1天）时施加最大惩罚，4天内3场也给予额外惩罚。
func (s *LineupRecommendService) calcFatigueFactor(stats []entity.PlayerGameStats, gameDate string) float64 {
	if len(stats) == 0 {
		return 1.0
	}

	targetDate, ok := parseISODate(gameDate)
	if !ok {
		return 1.0
	}

	lastGameDate := normalizeDateOnly(stats[0].GameDate)
	daysRest := int(targetDate.Sub(lastGameDate).Hours() / 24)

	factor := 1.0
	switch {
	case daysRest <= 0:
		factor = 1.0
	case daysRest == 1:
		factor = 1.0 - FatigueB2BPenalty
	case daysRest == 2:
		factor = FatigueRest2DaysFactor
	default:
		factor = 1.0
	}

	gamesIn4Days := 0
	for _, g := range stats {
		daysDiff := int(targetDate.Sub(normalizeDateOnly(g.GameDate)).Hours() / 24)
		if daysDiff > 0 && daysDiff <= 4 {
			gamesIn4Days++
		}
	}
	if gamesIn4Days >= 3 {
		factor -= Fatigue4Days3GamesPenalty
	}

	return clamp(factor, 0.88, 1.00)
}

// calcPredictiveFactorConfidence 计算因子系统整体置信度 [0.24, 0.94]。
// 综合样本数、波动性、稳定性、角色安全性和数据可靠性。
func calcPredictiveFactorConfidence(
	profile recentPowerProfile,
	stabilityFactor float64,
	roleSecurityFactor float64,
	dataReliabilityFactor float64,
) float64 {
	sampleConfidence := clamp(float64(profile.SampleCount)/10.0, 0.18, 1.0)
	volatilityPenalty := clamp((profile.Volatility-0.18)/(0.48-0.18), 0.0, 1.0)
	stabilityConfidence := clamp((stabilityFactor-0.90)/(1.03-0.90), 0.12, 1.0)
	roleConfidence := clamp((roleSecurityFactor-0.72)/(1.05-0.72), 0.12, 1.0)
	reliabilityConfidence := clamp((dataReliabilityFactor-0.42)/(1.02-0.42), 0.10, 1.0)

	confidence := 0.22 +
		0.30*sampleConfidence +
		0.20*stabilityConfidence +
		0.14*roleConfidence +
		0.18*reliabilityConfidence -
		0.12*volatilityPenalty
	return clamp(confidence, 0.24, 0.94)
}

// applyStableStarLift 对满足条件的稳定高薪前场明星球员施加保底提升。
// 基础值≥46、工资≥35、多项因子达标时，将预测值提升至基础值附近。
func applyStableStarLift(
	player entity.NBAGamePlayer,
	predictedPower float64,
	baseValue float64,
	minutesFactor float64,
	usageFactor float64,
	stabilityFactor float64,
	defenseUpsideFactor float64,
	roleSecurityFactor float64,
	dataReliabilityFactor float64,
	defenseAnchorFactor float64,
) float64 {
	if predictedPower <= 0 || baseValue <= 0 {
		return predictedPower
	}

	positionGroup := normalizePositionGroup(player.Position)
	stableSignal := 0.0
	if baseValue >= 50 {
		stableSignal += 0.18
	}
	if minutesFactor >= 1.02 {
		stableSignal += 0.18
	}
	if usageFactor >= 1.01 {
		stableSignal += 0.12
	}
	if stabilityFactor >= 0.99 {
		stableSignal += 0.18
	}
	if defenseUpsideFactor >= 1.03 {
		stableSignal += 0.16
	}
	if roleSecurityFactor >= 0.95 {
		stableSignal += 0.18
	}
	if dataReliabilityFactor >= 0.90 {
		stableSignal += 0.18
	}
	if positionGroup == 0 && defenseAnchorFactor >= 0.94 {
		stableSignal += 0.08
	}

	if positionGroup == 0 {
		if baseValue < StableStarBasePower || player.Salary < StableStarSalaryMin || stableSignal <= 0.25 {
			return predictedPower
		}

		lift := clamp((stableSignal-0.25)*0.09, 0.0, 0.07)
		targetFloor := baseValue * (1.00 + lift)
		return math.Max(predictedPower, targetFloor)
	}

	if player.Salary < 36 || baseValue < 40 || stableSignal <= 0.42 {
		return predictedPower
	}
	if minutesFactor < 1.0 || usageFactor < 1.0 || roleSecurityFactor < 0.90 || dataReliabilityFactor < 0.88 {
		return predictedPower
	}

	lift := clamp((stableSignal-0.42)*0.08, 0.0, 0.05)
	targetFloor := baseValue * (0.94 + lift)
	return math.Max(predictedPower, targetFloor)
}
