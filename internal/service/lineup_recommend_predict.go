package service

import (
	"context"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

const (
	matchupLookbackDays    = 120
	teamMetricMaxGames     = 24
	teamMetricMinGames     = 6
	teamTrendRecentGames   = 6
	dvpMetricMinSampleSize = 8
	recentBaseMinGames     = 3
)

type teamMatchupMetric struct {
	DefRatingFactor       float64
	PaceFactor            float64
	OpponentFormFactor    float64
	RimDeterrenceFactor   float64
	PerimeterImpactFactor float64
	SampleCount           int
}

type positionDVPMetric struct {
	Factor      float64
	SampleCount int
}

type txLineupPlayer struct {
	ID     uint
	CnName string
	EnName string
}

type matchupFactorDetail struct {
	MatchupFactor         float64
	DefRatingFactor       float64
	PaceFactor            float64
	DvPFactor             float64
	HistoryFactor         float64
	OpponentFormFactor    float64
	RimDeterrenceFactor   float64
	PerimeterImpactFactor float64
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
	availabilityScore := 1.0
	if player.CombatPower == 0 {
		return PlayerPrediction{AvailabilityScore: 0.0}
	}
	if injury, ok := injuryMap[player.NBAPlayerID]; ok {
		availabilityScore = crawler.StatusToAvailabilityScore(injury.Status)
		availabilityScore = rebalanceAvailabilityScore(injury.Status, availabilityScore)
		if availabilityScore == 0 {
			return PlayerPrediction{AvailabilityScore: 0.0}
		}
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
	baseValue = stabilizeBaseValue(baseValue, gamePower, recentPower5, recentPower10, len(stats))

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
	teamContextFactor := s.calcTeamContextFactor(player, allPlayers)

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
	)

	// Step 8: 因素2 — 比赛取消风险 (GameRiskFactor)
	gameRiskFactor := 1.0 // NBA 室内运动，默认无风险

	// Step 9: 综合计算
	dynamicMultiplier := statusTrend * matchupFactor * homeAwayFactor * teamContextFactor *
		minutesFactor * usageFactor * stabilityFactor * defenseUpsideFactor *
		archetypeFactor * roleSecurityFactor * dataReliabilityFactor *
		defenseAnchorFactor * fatigueFactor
	dynamicMultiplier = clampDynamicMultiplier(dynamicMultiplier, len(stats))

	predictedPower := baseValue * availabilityScore * dynamicMultiplier * gameRiskFactor
	predictedPower = calibratePredictedPower(predictedPower, baseValue, recentPower10, len(stats))
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

func rebalanceAvailabilityScore(status string, availabilityScore float64) float64 {
	if availabilityScore <= 0 || availabilityScore >= 1 {
		return availabilityScore
	}

	normalized := strings.ToLower(strings.TrimSpace(status))
	switch {
	case normalized == "day-to-day":
		return clamp(math.Max(availabilityScore, 0.78), 0.0, 1.0)
	case strings.Contains(normalized, "questionable"):
		return clamp(math.Max(availabilityScore, 0.68), 0.0, 1.0)
	default:
		return availabilityScore
	}
}

func matchNBAGamePlayerToTxLineupPlayer(
	player entity.NBAGamePlayer,
	lineupPlayers []struct {
		ID       string `json:"id"`
		CnName   string `json:"cnName"`
		EnName   string `json:"enName"`
		Logo     string `json:"logo"`
		Position string `json:"position"`
	},
	usedTxIDs map[uint]struct{},
) (uint, bool) {
	normalizedEn := normalizePlayerName(player.PlayerEnName)
	normalizedCN := normalizeLocalizedPlayerName(player.PlayerName)

	exactCandidates := make([]txLineupPlayer, 0)
	fuzzyCandidates := make([]txLineupPlayer, 0)
	for _, raw := range lineupPlayers {
		txPlayerID := parseUintOrZero(raw.ID)
		if txPlayerID == 0 {
			continue
		}
		if _, used := usedTxIDs[txPlayerID]; used {
			continue
		}

		lineupPlayer := txLineupPlayer{
			ID:     txPlayerID,
			CnName: raw.CnName,
			EnName: raw.EnName,
		}
		if normalizedEn != "" && normalizePlayerName(raw.EnName) == normalizedEn {
			exactCandidates = append(exactCandidates, lineupPlayer)
			continue
		}
		if normalizedCN != "" && normalizeLocalizedPlayerName(raw.CnName) == normalizedCN {
			exactCandidates = append(exactCandidates, lineupPlayer)
			continue
		}
		if normalizedEn != "" && crawler.MatchInjuryToPlayer(raw.EnName, player.PlayerEnName) {
			fuzzyCandidates = append(fuzzyCandidates, lineupPlayer)
		}
	}

	if len(exactCandidates) == 1 {
		return exactCandidates[0].ID, true
	}
	if len(fuzzyCandidates) == 1 {
		return fuzzyCandidates[0].ID, true
	}
	return 0, false
}

func normalizeLocalizedPlayerName(name string) string {
	replacer := strings.NewReplacer(
		".", "",
		"-", "",
		"·", "",
		" ", "",
	)
	return strings.ToLower(strings.TrimSpace(replacer.Replace(name)))
}

func parseUintOrZero(value string) uint {
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
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

func stripPredictAnchors(dbPlayerMap map[uint]*entity.Player) map[uint]*entity.Player {
	if len(dbPlayerMap) == 0 {
		return dbPlayerMap
	}
	result := make(map[uint]*entity.Player, len(dbPlayerMap))
	for nbaPlayerID, player := range dbPlayerMap {
		if player == nil {
			continue
		}
		cloned := *player
		cloned.PowerPer5 = 0
		cloned.PowerPer10 = 0
		result[nbaPlayerID] = &cloned
	}
	return result
}

type recommendTxMapSummary struct {
	DBCount             int
	ManualCount         int
	LineupFallbackCount int
}

func (s *LineupRecommendService) buildRecommendTxPlayerIDMap(
	ctx context.Context,
	gamePlayers []entity.NBAGamePlayer,
	dbPlayerMap map[uint]*entity.Player,
) (map[uint]uint, recommendTxMapSummary) {
	result := make(map[uint]uint, len(gamePlayers))
	summary := recommendTxMapSummary{}

	for _, player := range gamePlayers {
		dbPlayer := dbPlayerMap[player.NBAPlayerID]
		if dbPlayer == nil || dbPlayer.TxPlayerID == 0 {
			continue
		}
		if _, exists := result[player.NBAPlayerID]; exists {
			continue
		}
		result[player.NBAPlayerID] = dbPlayer.TxPlayerID
		summary.DBCount++
	}

	missingSet := make(map[uint]struct{})
	for _, player := range gamePlayers {
		if player.NBAPlayerID == 0 {
			continue
		}
		if result[player.NBAPlayerID] > 0 {
			continue
		}
		missingSet[player.NBAPlayerID] = struct{}{}
	}
	summary.ManualCount = applyManualNBATxPlayerIDOverrides(result, missingSet)

	missingByTeam := make(map[string][]entity.NBAGamePlayer)
	for _, player := range gamePlayers {
		if player.NBAPlayerID == 0 || result[player.NBAPlayerID] > 0 || strings.TrimSpace(player.NBATeamID) == "" {
			continue
		}
		missingByTeam[player.NBATeamID] = append(missingByTeam[player.NBATeamID], player)
	}
	if len(missingByTeam) == 0 || s.txNBAClient == nil {
		return result, summary
	}

	for teamID, teamPlayers := range missingByTeam {
		resp, err := s.txNBAClient.GetTeamLineup(ctx, teamID)
		if err != nil {
			log.Printf("获取腾讯球队阵容失败: team_id=%s err=%v", teamID, err)
			continue
		}
		if resp == nil || len(resp.Data.LineUp.Players) == 0 {
			continue
		}

		usedTxIDs := make(map[uint]struct{})
		for _, player := range teamPlayers {
			txPlayerID, ok := matchNBAGamePlayerToTxLineupPlayer(player, resp.Data.LineUp.Players, usedTxIDs)
			if !ok {
				continue
			}
			result[player.NBAPlayerID] = txPlayerID
			usedTxIDs[txPlayerID] = struct{}{}
			summary.LineupFallbackCount++
		}
	}

	return result, summary
}

func (s *LineupRecommendService) loadGameStatsMap(
	ctx context.Context,
	repo *repositories.StatsRepository,
	txPlayerIDMap map[uint]uint,
	gameDate string,
	historical bool,
) map[uint][]entity.PlayerGameStats {
	result := make(map[uint][]entity.PlayerGameStats)

	seenTxIDs := make(map[uint]struct{})
	var txPlayerIDs []uint
	for _, txPlayerID := range txPlayerIDMap {
		if txPlayerID == 0 {
			continue
		}
		if _, ok := seenTxIDs[txPlayerID]; ok {
			continue
		}
		seenTxIDs[txPlayerID] = struct{}{}
		txPlayerIDs = append(txPlayerIDs, txPlayerID)
	}

	if len(txPlayerIDs) == 0 {
		return result
	}

	// 批量获取近 10 场数据
	var (
		statsMap map[uint][]entity.PlayerGameStats
		err      error
	)
	if historical {
		statsMap, err = repo.BatchGetRecentGameStatsBeforeDate(ctx, txPlayerIDs, 10, gameDate)
	} else {
		statsMap, err = repo.BatchGetRecentGameStats(ctx, txPlayerIDs, 10)
	}
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
	txPlayerIDMap map[uint]uint,
	gameDate string,
	historical bool,
) map[uint]*entity.PlayerSeasonStats {
	result := make(map[uint]*entity.PlayerSeasonStats)
	if historical {
		return result
	}

	seenTxIDs := make(map[uint]struct{})
	var txPlayerIDs []uint
	for _, txPlayerID := range txPlayerIDMap {
		if txPlayerID == 0 {
			continue
		}
		if _, ok := seenTxIDs[txPlayerID]; ok {
			continue
		}
		seenTxIDs[txPlayerID] = struct{}{}
		txPlayerIDs = append(txPlayerIDs, txPlayerID)
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
	gameDate string,
	historical bool,
) map[string]teamMatchupMetric {
	var (
		rows []repositories.TeamGameAggregate
		err  error
	)
	if historical {
		beforeDate, ok := parseISODate(gameDate)
		if !ok {
			return map[string]teamMatchupMetric{}
		}
		rows, err = repo.GetRecentTeamGameAggregatesBeforeDate(ctx, matchupLookbackDays, beforeDate)
	} else {
		rows, err = repo.GetRecentTeamGameAggregates(ctx, matchupLookbackDays)
	}
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
	type teamDefenseSkillSample struct {
		Date       time.Time
		TeamBlocks float64
		TeamSteals float64
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
	defenseSkillMap := make(map[string][]teamDefenseSkillSample)
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

		defenseSkillMap[offenseTeam] = append(defenseSkillMap[offenseTeam], teamDefenseSkillSample{
			Date:       row.GameDate,
			TeamBlocks: row.TeamBlocks,
			TeamSteals: row.TeamSteals,
		})
	}
	if len(teamSampleMap) == 0 && len(defenseSkillMap) == 0 {
		return map[string]teamMatchupMetric{}
	}

	trimmedByTeam := make(map[string][]teamGameSample, len(teamSampleMap))
	trimmedSkillByTeam := make(map[string][]teamDefenseSkillSample, len(defenseSkillMap))
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

	leagueBlocksTotal := 0.0
	leagueStealsTotal := 0.0
	leagueSkillCount := 0
	for team, samples := range defenseSkillMap {
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Date.After(samples[j].Date)
		})
		if len(samples) > teamMetricMaxGames {
			samples = samples[:teamMetricMaxGames]
		}
		trimmedSkillByTeam[team] = samples
		for _, sample := range samples {
			leagueBlocksTotal += sample.TeamBlocks
			leagueStealsTotal += sample.TeamSteals
			leagueSkillCount++
		}
	}

	leagueAvgAllowed := 112.0
	leagueAvgTotal := 224.0
	if leagueSampleCount > 0 {
		leagueAvgAllowed = leagueAllowedTotal / float64(leagueSampleCount)
		leagueAvgTotal = leagueTotalPoints / float64(leagueSampleCount)
	}
	if leagueAvgAllowed <= 0 {
		leagueAvgAllowed = 112.0
	}
	if leagueAvgTotal <= 0 {
		leagueAvgTotal = 224.0
	}
	leagueAvgBlocks := 5.2
	leagueAvgSteals := 7.5
	if leagueSkillCount > 0 {
		leagueAvgBlocks = leagueBlocksTotal / float64(leagueSkillCount)
		leagueAvgSteals = leagueStealsTotal / float64(leagueSkillCount)
	}

	result := make(map[string]teamMatchupMetric, max(len(trimmedByTeam), len(trimmedSkillByTeam)))
	for team, samples := range trimmedByTeam {
		metric := teamMatchupMetric{
			DefRatingFactor:       1.0,
			PaceFactor:            1.0,
			OpponentFormFactor:    1.0,
			RimDeterrenceFactor:   1.0,
			PerimeterImpactFactor: 1.0,
			SampleCount:           len(samples),
		}
		if len(samples) >= teamMetricMinGames {
			teamAllowed := 0.0
			teamTotal := 0.0
			for _, sample := range samples {
				teamAllowed += sample.PointsAllowed
				teamTotal += sample.GameTotal
			}
			avgAllowed := teamAllowed / float64(len(samples))
			avgTotal := teamTotal / float64(len(samples))
			metric.DefRatingFactor = clamp(avgAllowed/leagueAvgAllowed, 0.88, 1.12)
			metric.PaceFactor = clamp(avgTotal/leagueAvgTotal, 0.94, 1.08)

			recentCount := min(teamTrendRecentGames, len(samples))
			recentAllowed := 0.0
			for i := 0; i < recentCount; i++ {
				recentAllowed += samples[i].PointsAllowed
			}
			if recentCount > 0 && avgAllowed > 0 {
				recentAvgAllowed := recentAllowed / float64(recentCount)
				metric.OpponentFormFactor = clamp(recentAvgAllowed/avgAllowed, 0.90, 1.08)
			}
		}

		skillSamples := trimmedSkillByTeam[team]
		if len(skillSamples) >= teamMetricMinGames {
			blocksTotal := 0.0
			stealsTotal := 0.0
			for _, sample := range skillSamples {
				blocksTotal += sample.TeamBlocks
				stealsTotal += sample.TeamSteals
			}
			avgBlocks := blocksTotal / float64(len(skillSamples))
			avgSteals := stealsTotal / float64(len(skillSamples))
			if avgBlocks > 0 && leagueAvgBlocks > 0 {
				metric.RimDeterrenceFactor = clamp(leagueAvgBlocks/avgBlocks, 0.88, 1.08)
			}
			if avgSteals > 0 && leagueAvgSteals > 0 {
				metric.PerimeterImpactFactor = clamp(leagueAvgSteals/avgSteals, 0.92, 1.08)
			}
		}

		result[team] = metric
	}

	for team, skillSamples := range trimmedSkillByTeam {
		if _, exists := result[team]; exists {
			continue
		}
		metric := teamMatchupMetric{
			DefRatingFactor:       1.0,
			PaceFactor:            1.0,
			OpponentFormFactor:    1.0,
			RimDeterrenceFactor:   1.0,
			PerimeterImpactFactor: 1.0,
			SampleCount:           len(skillSamples),
		}
		if len(skillSamples) >= teamMetricMinGames {
			blocksTotal := 0.0
			stealsTotal := 0.0
			for _, sample := range skillSamples {
				blocksTotal += sample.TeamBlocks
				stealsTotal += sample.TeamSteals
			}
			avgBlocks := blocksTotal / float64(len(skillSamples))
			avgSteals := stealsTotal / float64(len(skillSamples))
			if avgBlocks > 0 && leagueAvgBlocks > 0 {
				metric.RimDeterrenceFactor = clamp(leagueAvgBlocks/avgBlocks, 0.88, 1.08)
			}
			if avgSteals > 0 && leagueAvgSteals > 0 {
				metric.PerimeterImpactFactor = clamp(leagueAvgSteals/avgSteals, 0.92, 1.08)
			}
		}
		result[team] = metric
	}

	return result
}

func (s *LineupRecommendService) buildDVPFactorMap(
	allPlayers []entity.NBAGamePlayer,
	txPlayerIDMap map[uint]uint,
	gameStatsMap map[uint][]entity.PlayerGameStats,
) map[string]map[uint]positionDVPMetric {
	opponentPositionPowers := make(map[string]map[uint][]float64)
	leaguePositionPowers := make(map[uint][]float64)

	for _, player := range allPlayers {
		txPlayerID := txPlayerIDMap[player.NBAPlayerID]
		if txPlayerID == 0 {
			continue
		}
		stats := gameStatsMap[txPlayerID]
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

func calcRecentPowerAverages(stats []entity.PlayerGameStats) (float64, float64) {
	if len(stats) == 0 {
		return 0, 0
	}
	count5 := min(5, len(stats))
	count10 := min(10, len(stats))

	sum5 := 0.0
	sum10 := 0.0
	for i := 0; i < count10; i++ {
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

func calcStatusTrend(dbPlayer *entity.Player, stats []entity.PlayerGameStats) float64 {
	dbTrend := 1.0
	if dbPlayer != nil && dbPlayer.PowerPer10 > 0 && dbPlayer.PowerPer5 > 0 {
		dbTrend = clamp(dbPlayer.PowerPer5/dbPlayer.PowerPer10, 0.88, 1.12)
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
				recentTrend = clamp(avg3/avg10, 0.90, 1.10)
			}
		}
	}

	recentWeight := 0.0
	if len(stats) >= 5 {
		recentWeight = 0.40
	}
	return clamp(dbTrend*(1-recentWeight)+recentTrend*recentWeight, 0.88, 1.12)
}

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

func calibratePredictedPower(predicted, baseValue, recentPower10 float64, statCount int) float64 {
	if predicted <= 0 {
		return predicted
	}
	if recentPower10 <= 0 || statCount < 5 {
		// 无足够近期样本时，避免过拟合因素把结果拉太离谱。
		modelWeight := 0.85
		if statCount == 0 {
			modelWeight = 0.72
		} else if statCount < 3 {
			modelWeight = 0.78
		}
		return modelWeight*predicted + (1-modelWeight)*baseValue
	}

	reliability := clamp(float64(min(statCount, 10))/10.0, 0.40, 1.0)
	modelWeight := 0.58 + 0.20*reliability
	anchored := modelWeight*predicted + (1-modelWeight)*recentPower10
	return anchored
}

func shrinkTowardsOne(value, confidence float64) float64 {
	conf := clamp(confidence, 0.0, 1.0)
	return 1.0 + (value-1.0)*conf
}

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
		confidence = clamp(confidence, 0.35, 1.0)
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
		disruptionFactor = 0.72*rimDeterrenceFactor + 0.28*perimeterImpactFactor
	} else {
		disruptionFactor = 0.35*rimDeterrenceFactor + 0.65*perimeterImpactFactor
	}
	disruptionFactor = clamp(disruptionFactor, 0.90, 1.08)

	baseMatchup := 0.43*defRatingFactor + 0.23*paceFactor + 0.16*historyFactor + 0.18*opponentFormFactor
	matchupFactor := clamp(baseMatchup*dvpFactor*disruptionFactor, 0.86, 1.14)
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
		return clamp(avgPower/baseValue, 0.90, 1.10)
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
		for i := 0; i < recentCount; i++ {
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
			impact = 1.25*blocksAvg + 0.45*stealsAvg
		} else {
			impact = 0.45*blocksAvg + 0.95*stealsAvg
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
	if baseValue < 45 || minutesFactor < 1.03 || usageFactor < 1.03 {
		return matchupFactor, defenseAnchorFactor
	}

	resilience := 0.30
	if baseValue >= 50 {
		resilience += 0.18
	}
	if player.Salary >= 40 {
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
) float64 {
	factor := 1.0
	positionGroup := normalizePositionGroup(player.Position)

	if positionGroup == 0 {
		if player.Salary <= 20 && player.CombatPower >= 18 {
			factor += clamp((minutesFactor-1.0)*0.40, 0.0, 0.05)
			factor += clamp((defenseUpsideFactor-1.0)*0.45, 0.0, 0.06)
			factor += clamp((teamContextFactor-1.0)*0.80, 0.0, 0.04)
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
		return clamp(factor, 0.94, 1.12)
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

	return clamp(factor, 0.90, 1.05)
}

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
	if positionGroup == 0 && player.Salary <= 20 && archetypeFactor > 1.02 && defenseAnchorFactor >= 0.94 {
		optimizedPower += upsideGap * clamp((archetypeFactor-1.0)*4.0, 0.0, 0.42)
	}

	return clamp(optimizedPower, predictedPower*0.62, predictedPower)
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

func (s *LineupRecommendService) calcConservativePower(
	predicted float64,
	stats []entity.PlayerGameStats,
	availabilityScore float64,
	roleSecurityFactor float64,
	dataReliabilityFactor float64,
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
	availabilityRisk := clamp(0.85+0.15*availabilityScore, 0.75, 1.0)
	conservative := blended * clamp(0.90+0.10*riskAnchor, 0.80, 1.02) * availabilityRisk
	return clamp(conservative, predicted*0.60, predicted*1.02)
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
