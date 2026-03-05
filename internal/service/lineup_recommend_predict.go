package service

import (
	"context"
	"log"
	"math"
	"sort"
	"time"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

const (
	matchupLookbackDays    = 120
	teamMetricMaxGames     = 24
	teamMetricMinGames     = 6
	dvpMetricMinSampleSize = 8
)

type teamMatchupMetric struct {
	DefRatingFactor float64
	PaceFactor      float64
	SampleCount     int
}

type positionDVPMetric struct {
	Factor      float64
	SampleCount int
}

// predictPower 计算单个候选球员的预测战力及因子明细。
func (s *LineupRecommendService) predictPower(
	player entity.NBAGamePlayer,
	allPlayers []entity.NBAGamePlayer,
	injuryMap map[uint]crawler.InjuryReport,
	dbPlayerMap map[uint]*entity.Player,
	gameStatsMap map[uint][]entity.PlayerGameStats,
	seasonStatsMap map[uint]*entity.PlayerSeasonStats,
	teamMatchupMap map[string]teamMatchupMetric,
	dvpFactorMap map[string]map[uint]positionDVPMetric,
) PlayerPrediction {

	// Step 1: 因素1 — 球员出场可用性 (AvailabilityScore)
	availabilityScore := 1.0
	if player.CombatPower == 0 {
		return PlayerPrediction{AvailabilityScore: 0.0}
	}
	if injury, ok := injuryMap[player.NBAPlayerID]; ok {
		availabilityScore = crawler.StatusToAvailabilityScore(injury.Status)
		if availabilityScore == 0 {
			return PlayerPrediction{AvailabilityScore: 0.0}
		}
	}

	// Step 2: 基础战力值 (BaseValue)
	gamePower := player.CombatPower
	baseValue := gamePower
	dbPlayer := dbPlayerMap[player.NBAPlayerID]
	var dbPower5, dbPower10 float64
	if dbPlayer != nil && dbPlayer.PowerPer10 > 0 {
		dbPower10 = dbPlayer.PowerPer10
		dbPower5 = dbPlayer.PowerPer5
		baseValue = 0.4*dbPower10 + 0.3*dbPower5 + 0.3*gamePower
	}

	// Step 3: 因素3 — 近期状态趋势 (StatusTrend)
	statusTrend := 1.0
	if dbPlayer != nil && dbPlayer.PowerPer10 > 0 && dbPlayer.PowerPer5 > 0 {
		rawTrend := dbPlayer.PowerPer5 / dbPlayer.PowerPer10
		statusTrend = clamp(rawTrend, 0.85, 1.15)
	}

	// Step 4: 因素4 — 对手实力匹配 (MatchupFactor)
	matchupFactor := 1.0
	defRatingFactor := 1.0
	paceFactor := 1.0
	dvpFactor := 1.0
	historyFactor := 1.0
	txPlayerID := uint(0)
	if dbPlayer != nil {
		txPlayerID = dbPlayer.TxPlayerID
	}
	var stats []entity.PlayerGameStats
	if txPlayerID > 0 {
		stats = gameStatsMap[txPlayerID]
	}
	opponentTeam := s.getOpponentTeamCode(player, allPlayers)
	matchupFactor, defRatingFactor, paceFactor, dvpFactor, historyFactor = s.calcMatchupFactorWithContext(
		stats,
		opponentTeam,
		baseValue,
		player.Position,
		teamMatchupMap,
		dvpFactorMap,
	)

	// Step 5: 因素5 — 球队阵容上下文 (TeamContextFactor)
	teamContextFactor := s.calcTeamContextFactor(player, allPlayers)

	// Step 6: 因素6 — 主客场因子 (HomeAwayFactor)
	homeAwayFactor := s.calcHomeAwayFactor(player, txPlayerID, gameStatsMap)

	// Step 7: 额外因子 — 上场时间趋势、使用率趋势、稳定性、赛程疲劳
	minutesFactor := 1.0
	usageFactor := 1.0
	stabilityFactor := 1.0
	fatigueFactor := 1.0
	if txPlayerID > 0 {
		stats := gameStatsMap[txPlayerID]
		minutesFactor = s.calcMinutesFactor(stats, seasonStatsMap[txPlayerID])
		usageFactor = s.calcUsageFactor(stats)
		stabilityFactor = s.calcStabilityFactor(stats)
		fatigueFactor = s.calcFatigueFactor(stats, player.GameDate)
	}

	// Step 8: 因素2 — 比赛取消风险 (GameRiskFactor)
	gameRiskFactor := 1.0 // NBA 室内运动，默认无风险

	// Step 9: 综合计算
	predictedPower := baseValue * availabilityScore * statusTrend * matchupFactor *
		homeAwayFactor * teamContextFactor * minutesFactor * usageFactor *
		stabilityFactor * fatigueFactor * gameRiskFactor

	return PlayerPrediction{
		PredictedPower:    roundTo(predictedPower, 1),
		BaseValue:         roundTo(baseValue, 1),
		AvailabilityScore: availabilityScore,
		StatusTrend:       roundTo(statusTrend, 2),
		MatchupFactor:     roundTo(matchupFactor, 2),
		DefRatingFactor:   roundTo(defRatingFactor, 2),
		PaceFactor:        roundTo(paceFactor, 2),
		DvPFactor:         roundTo(dvpFactor, 2),
		HistoryFactor:     roundTo(historyFactor, 2),
		HomeAwayFactor:    roundTo(homeAwayFactor, 2),
		TeamContextFactor: roundTo(teamContextFactor, 2),
		MinutesFactor:     roundTo(minutesFactor, 2),
		UsageFactor:       roundTo(usageFactor, 2),
		StabilityFactor:   roundTo(stabilityFactor, 2),
		FatigueFactor:     roundTo(fatigueFactor, 2),
		GameRiskFactor:    roundTo(gameRiskFactor, 2),
	}
}

// --- 0-1 背包 DP 求解 ---

// 预测流程所需的数据准备与因子计算函数。
func (s *LineupRecommendService) fetchInjuryMap(ctx context.Context, players []entity.NBAGamePlayer) map[uint]crawler.InjuryReport {
	result := make(map[uint]crawler.InjuryReport)

	reports, err := s.injuryClient.GetInjuryReports(ctx)
	if err != nil {
		log.Printf("获取伤病报告失败（将跳过伤病因素）: %v", err)
		return result
	}

	exactNameMap := make(map[string][]entity.NBAGamePlayer)
	for _, p := range players {
		key := normalizePlayerName(p.PlayerEnName)
		if key == "" {
			continue
		}
		exactNameMap[key] = append(exactNameMap[key], p)
	}

	for _, report := range reports {
		nbaPlayerID, ok := pickInjuryMatchedPlayer(report, players, exactNameMap)
		if !ok {
			continue
		}
		result[nbaPlayerID] = report
	}

	return result
}

func pickInjuryMatchedPlayer(
	report crawler.InjuryReport,
	players []entity.NBAGamePlayer,
	exactNameMap map[string][]entity.NBAGamePlayer,
) (uint, bool) {
	reportTeamCode := normalizeTeamCode(report.TeamName)
	reportName := normalizePlayerName(report.PlayerName)

	if reportName != "" {
		if exactMatches := exactNameMap[reportName]; len(exactMatches) > 0 {
			if id, ok := selectPlayerByTeamCode(exactMatches, reportTeamCode); ok {
				return id, true
			}
			return exactMatches[0].NBAPlayerID, true
		}
	}

	var fuzzyMatches []entity.NBAGamePlayer
	for _, p := range players {
		if crawler.MatchInjuryToPlayer(report.PlayerName, p.PlayerEnName) {
			fuzzyMatches = append(fuzzyMatches, p)
		}
	}
	if len(fuzzyMatches) == 0 {
		return 0, false
	}
	if id, ok := selectPlayerByTeamCode(fuzzyMatches, reportTeamCode); ok {
		return id, true
	}
	return fuzzyMatches[0].NBAPlayerID, true
}

func selectPlayerByTeamCode(players []entity.NBAGamePlayer, teamCode string) (uint, bool) {
	if teamCode == "" {
		return 0, false
	}
	for _, p := range players {
		if normalizeTeamCode(p.TeamName) == teamCode {
			return p.NBAPlayerID, true
		}
	}
	return 0, false
}

func (s *LineupRecommendService) loadDBPlayerMap(ctx context.Context, gamePlayers []entity.NBAGamePlayer) map[uint]*entity.Player {
	result := make(map[uint]*entity.Player)

	// 收集所有 NBAPlayerID
	seenNBAIDs := make(map[uint]struct{})
	var nbaIDs []uint
	for _, p := range gamePlayers {
		if p.NBAPlayerID == 0 {
			continue
		}
		if _, ok := seenNBAIDs[p.NBAPlayerID]; ok {
			continue
		}
		seenNBAIDs[p.NBAPlayerID] = struct{}{}
		nbaIDs = append(nbaIDs, p.NBAPlayerID)
	}
	if len(nbaIDs) == 0 {
		return result
	}

	// 从 players 表批量查询
	var dbPlayers []entity.Player
	if err := s.db.WithContext(ctx).Where("nba_player_id IN ?", nbaIDs).Find(&dbPlayers).Error; err != nil {
		log.Printf("查询 DB 球员失败: %v", err)
		return result
	}

	for i := range dbPlayers {
		result[dbPlayers[i].NBAPlayerID] = &dbPlayers[i]
	}

	return result
}

func (s *LineupRecommendService) loadGameStatsMap(ctx context.Context, repo *repositories.StatsRepository, dbPlayerMap map[uint]*entity.Player) map[uint][]entity.PlayerGameStats {
	result := make(map[uint][]entity.PlayerGameStats)

	// 收集所有有 tx_player_id 的球员
	seenTxIDs := make(map[uint]struct{})
	var txPlayerIDs []uint
	for _, p := range dbPlayerMap {
		if p.TxPlayerID > 0 {
			if _, ok := seenTxIDs[p.TxPlayerID]; ok {
				continue
			}
			seenTxIDs[p.TxPlayerID] = struct{}{}
			txPlayerIDs = append(txPlayerIDs, p.TxPlayerID)
		}
	}

	if len(txPlayerIDs) == 0 {
		return result
	}

	// 批量获取近 10 场数据
	statsMap, err := repo.BatchGetRecentGameStats(ctx, txPlayerIDs, 10)
	if err != nil {
		log.Printf("批量获取历史比赛数据失败: %v", err)
		return result
	}

	for txPlayerID := range statsMap {
		sort.Slice(statsMap[txPlayerID], func(i, j int) bool {
			return statsMap[txPlayerID][i].GameDate.After(statsMap[txPlayerID][j].GameDate)
		})
	}

	return statsMap
}

func (s *LineupRecommendService) loadSeasonStatsMap(
	ctx context.Context,
	repo *repositories.StatsRepository,
	dbPlayerMap map[uint]*entity.Player,
	gameDate string,
) map[uint]*entity.PlayerSeasonStats {
	result := make(map[uint]*entity.PlayerSeasonStats)

	seenTxIDs := make(map[uint]struct{})
	var txPlayerIDs []uint
	for _, p := range dbPlayerMap {
		if p.TxPlayerID == 0 {
			continue
		}
		if _, ok := seenTxIDs[p.TxPlayerID]; ok {
			continue
		}
		seenTxIDs[p.TxPlayerID] = struct{}{}
		txPlayerIDs = append(txPlayerIDs, p.TxPlayerID)
	}
	if len(txPlayerIDs) == 0 {
		return result
	}

	season := inferSeasonByGameDate(gameDate)
	seasonStatsMap, err := repo.GetSeasonStatsByTxPlayerIDs(ctx, txPlayerIDs, season, 1)
	if err != nil {
		log.Printf("批量获取赛季数据失败: %v", err)
		return result
	}
	return seasonStatsMap
}

func (s *LineupRecommendService) loadTeamMatchupMetrics(
	ctx context.Context,
	repo *repositories.StatsRepository,
) map[string]teamMatchupMetric {
	rows, err := repo.GetRecentTeamGameAggregates(ctx, matchupLookbackDays)
	if err != nil {
		log.Printf("加载对手 DefRating/Pace 数据失败: %v", err)
		return map[string]teamMatchupMetric{}
	}
	return buildTeamMatchupMetricsFromAggregates(rows)
}

func buildTeamMatchupMetricsFromAggregates(rows []repositories.TeamGameAggregate) map[string]teamMatchupMetric {
	type teamGameSample struct {
		Date          time.Time
		PointsAllowed float64
		GameTotal     float64
	}

	gameTotalMap := make(map[string]float64)
	for _, row := range rows {
		offenseTeam := normalizeTeamCode(row.PlayerTeamName)
		defenseTeam := normalizeTeamCode(row.VsTeamName)
		if offenseTeam == "" || defenseTeam == "" {
			continue
		}
		gameTotalMap[row.TxGameID] += row.TeamPoints
	}

	teamSampleMap := make(map[string][]teamGameSample)
	for _, row := range rows {
		offenseTeam := normalizeTeamCode(row.PlayerTeamName)
		defenseTeam := normalizeTeamCode(row.VsTeamName)
		if offenseTeam == "" || defenseTeam == "" {
			continue
		}

		gameTotal := gameTotalMap[row.TxGameID]
		if gameTotal <= 0 {
			continue
		}

		teamSampleMap[defenseTeam] = append(teamSampleMap[defenseTeam], teamGameSample{
			Date:          row.GameDate,
			PointsAllowed: row.TeamPoints,
			GameTotal:     gameTotal,
		})
	}
	if len(teamSampleMap) == 0 {
		return map[string]teamMatchupMetric{}
	}

	trimmedByTeam := make(map[string][]teamGameSample, len(teamSampleMap))
	leagueAllowedTotal := 0.0
	leagueTotalPoints := 0.0
	leagueSampleCount := 0
	for team, samples := range teamSampleMap {
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Date.After(samples[j].Date)
		})
		if len(samples) > teamMetricMaxGames {
			samples = samples[:teamMetricMaxGames]
		}
		trimmedByTeam[team] = samples
		for _, sample := range samples {
			leagueAllowedTotal += sample.PointsAllowed
			leagueTotalPoints += sample.GameTotal
			leagueSampleCount++
		}
	}

	if leagueSampleCount == 0 {
		return map[string]teamMatchupMetric{}
	}
	leagueAvgAllowed := leagueAllowedTotal / float64(leagueSampleCount)
	leagueAvgTotal := leagueTotalPoints / float64(leagueSampleCount)
	if leagueAvgAllowed <= 0 {
		leagueAvgAllowed = 112.0
	}
	if leagueAvgTotal <= 0 {
		leagueAvgTotal = 224.0
	}

	result := make(map[string]teamMatchupMetric, len(trimmedByTeam))
	for team, samples := range trimmedByTeam {
		metric := teamMatchupMetric{
			DefRatingFactor: 1.0,
			PaceFactor:      1.0,
			SampleCount:     len(samples),
		}
		if len(samples) < teamMetricMinGames {
			result[team] = metric
			continue
		}

		teamAllowed := 0.0
		teamTotal := 0.0
		for _, sample := range samples {
			teamAllowed += sample.PointsAllowed
			teamTotal += sample.GameTotal
		}
		avgAllowed := teamAllowed / float64(len(samples))
		avgTotal := teamTotal / float64(len(samples))
		metric.DefRatingFactor = clamp(avgAllowed/leagueAvgAllowed, 0.90, 1.15)
		metric.PaceFactor = clamp(avgTotal/leagueAvgTotal, 0.95, 1.10)
		result[team] = metric
	}
	return result
}

func (s *LineupRecommendService) buildDVPFactorMap(
	allPlayers []entity.NBAGamePlayer,
	dbPlayerMap map[uint]*entity.Player,
	gameStatsMap map[uint][]entity.PlayerGameStats,
) map[string]map[uint]positionDVPMetric {
	opponentPositionPowers := make(map[string]map[uint][]float64)
	leaguePositionPowers := make(map[uint][]float64)

	for _, player := range allPlayers {
		dbPlayer := dbPlayerMap[player.NBAPlayerID]
		if dbPlayer == nil || dbPlayer.TxPlayerID == 0 {
			continue
		}
		stats := gameStatsMap[dbPlayer.TxPlayerID]
		if len(stats) == 0 {
			continue
		}
		position := normalizePositionGroup(player.Position)
		for _, g := range stats {
			opponent := normalizeTeamCode(g.VsTeamName)
			if opponent == "" {
				continue
			}
			power := calcPowerFromStats(g)
			if power <= 0 {
				continue
			}
			if _, ok := opponentPositionPowers[opponent]; !ok {
				opponentPositionPowers[opponent] = make(map[uint][]float64)
			}
			opponentPositionPowers[opponent][position] = append(opponentPositionPowers[opponent][position], power)
			leaguePositionPowers[position] = append(leaguePositionPowers[position], power)
		}
	}

	leagueAvgByPosition := make(map[uint]float64)
	for position, powers := range leaguePositionPowers {
		if len(powers) == 0 {
			continue
		}
		total := 0.0
		for _, power := range powers {
			total += power
		}
		leagueAvgByPosition[position] = total / float64(len(powers))
	}

	result := make(map[string]map[uint]positionDVPMetric)
	for opponent, byPosition := range opponentPositionPowers {
		result[opponent] = make(map[uint]positionDVPMetric)
		for position, powers := range byPosition {
			metric := positionDVPMetric{
				Factor:      1.0,
				SampleCount: len(powers),
			}
			leagueAvg := leagueAvgByPosition[position]
			if len(powers) < dvpMetricMinSampleSize || leagueAvg <= 0 {
				result[opponent][position] = metric
				continue
			}
			total := 0.0
			for _, power := range powers {
				total += power
			}
			avgPower := total / float64(len(powers))
			metric.Factor = clamp(avgPower/leagueAvg, 0.92, 1.10)
			result[opponent][position] = metric
		}
	}
	return result
}

func (s *LineupRecommendService) getOpponentTeamCode(player entity.NBAGamePlayer, allPlayers []entity.NBAGamePlayer) string {
	for _, p := range allPlayers {
		if p.MatchID == player.MatchID && p.NBATeamID != player.NBATeamID {
			return normalizeTeamCode(p.TeamName)
		}
	}
	return ""
}

func (s *LineupRecommendService) calcMatchupFactor(stats []entity.PlayerGameStats, opponentTeam string, baseValue float64) float64 {
	return s.calcHistoryFactor(stats, opponentTeam, baseValue)
}

func (s *LineupRecommendService) calcMatchupFactorWithContext(
	stats []entity.PlayerGameStats,
	opponentTeam string,
	baseValue float64,
	position uint,
	teamMatchupMap map[string]teamMatchupMetric,
	dvpFactorMap map[string]map[uint]positionDVPMetric,
) (float64, float64, float64, float64, float64) {
	targetTeam := normalizeTeamCode(opponentTeam)
	if targetTeam == "" {
		return 1.0, 1.0, 1.0, 1.0, 1.0
	}

	historyFactor := s.calcHistoryFactor(stats, targetTeam, baseValue)
	defRatingFactor := 1.0
	paceFactor := 1.0
	if metric, ok := teamMatchupMap[targetTeam]; ok {
		defRatingFactor = metric.DefRatingFactor
		paceFactor = metric.PaceFactor
	}

	dvpFactor := 1.0
	positionGroup := normalizePositionGroup(position)
	if byPosition, ok := dvpFactorMap[targetTeam]; ok {
		if metric, ok := byPosition[positionGroup]; ok {
			dvpFactor = metric.Factor
		}
	}

	baseMatchup := 0.5*defRatingFactor + 0.3*paceFactor + 0.2*historyFactor
	matchupFactor := clamp(baseMatchup*dvpFactor, 0.88, 1.18)
	return matchupFactor, defRatingFactor, paceFactor, dvpFactor, historyFactor
}

func normalizePositionGroup(position uint) uint {
	if position == 1 {
		return 1
	}
	return 0
}

func (s *LineupRecommendService) calcHistoryFactor(stats []entity.PlayerGameStats, opponentTeam string, baseValue float64) float64 {
	if len(stats) == 0 || baseValue <= 0 {
		return 1.0
	}

	// 历史对阵因子：该球员对阵该对手的历史战力均值 / 基础战力。
	var vsGames []entity.PlayerGameStats
	targetTeam := normalizeTeamCode(opponentTeam)
	if targetTeam == "" {
		return 1.0
	}

	for _, g := range stats {
		if normalizeTeamCode(g.VsTeamName) == targetTeam {
			vsGames = append(vsGames, g)
		}
	}

	if len(vsGames) >= 3 {
		totalPower := 0.0
		for _, g := range vsGames {
			totalPower += calcPowerFromStats(g)
		}
		avgPower := totalPower / float64(len(vsGames))
		return clamp(avgPower/baseValue, 0.90, 1.10)
	}

	return 1.0
}

func (s *LineupRecommendService) calcTeamContextFactor(player entity.NBAGamePlayer, allPlayers []entity.NBAGamePlayer) float64 {
	// 统计同队球员中 CombatPower=0 的工资占比
	var totalTeamSalary, absentSalary float64
	for _, p := range allPlayers {
		if p.NBATeamID == player.NBATeamID && p.NBAPlayerID != player.NBAPlayerID {
			totalTeamSalary += float64(p.Salary)
			if p.CombatPower == 0 {
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
		factor = 0.94
	case daysRest == 2:
		factor = 0.98
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
		factor -= 0.03
	}

	return clamp(factor, 0.88, 1.00)
}
