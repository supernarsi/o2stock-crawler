// lineup_recommend_data.go 负责推荐流程所需的数据加载层，包括：
// - 伤病数据获取与快照持久化（fetchInjuryMap, persistInjurySnapshots）
// - DB 球员信息加载（loadDBPlayerMap）
// - NBA↔TX 薪资表球员 ID 映射（loadSalaryTxPlayerIDMap）
// - 历史比赛数据批量加载（loadGameStatsMap, loadSeasonStatsMap）
// - 球队防守指标聚合（loadTeamMatchupMetrics, buildTeamMatchupMetricsFromAggregates）
package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

// fetchInjuryMap 从 ESPN 获取实时伤病报告，并与候选球员列表匹配构建 NBAPlayerID → InjuryReport 映射。
// 同时返回快照行用于后续持久化。
func (s *LineupRecommendService) fetchInjuryMap(
	ctx context.Context,
	players []entity.NBAGamePlayer,
) (map[uint]crawler.InjuryReport, []entity.NBAGameInjurySnapshot, error) {
	result := make(map[uint]crawler.InjuryReport)
	snapshots := make([]entity.NBAGameInjurySnapshot, 0)

	reports, err := s.injuryClient.GetInjuryReports(ctx)
	if err != nil {
		return result, nil, fmt.Errorf("获取伤病报告失败: %w", err)
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
		snapshots = append(snapshots, entity.NBAGameInjurySnapshot{
			NBAPlayerID: nbaPlayerID,
			PlayerName:  report.PlayerName,
			TeamName:    report.TeamName,
			Status:      report.Status,
			Description: report.Description,
			ReportDate:  report.Date,
			Source:      "espn",
		})
	}

	return result, snapshots, nil
}

// loadInjurySnapshotMap 从数据库加载历史伤病快照（用于历史日期推荐模式）。
func (s *LineupRecommendService) loadInjurySnapshotMap(
	ctx context.Context,
	gameDate string,
) (map[uint]crawler.InjuryReport, bool) {
	repo := repositories.NewNBAGameInjurySnapshotRepository(s.db.DB)
	rows, err := repo.GetByGameDate(ctx, gameDate)
	if err != nil {
		log.Printf("读取伤病快照失败: game_date=%s err=%v", gameDate, err)
		return map[uint]crawler.InjuryReport{}, false
	}
	if len(rows) == 0 {
		return map[uint]crawler.InjuryReport{}, false
	}

	result := make(map[uint]crawler.InjuryReport, len(rows))
	for _, row := range rows {
		if row.NBAPlayerID == 0 {
			continue
		}
		result[row.NBAPlayerID] = crawler.InjuryReport{
			PlayerName:  row.PlayerName,
			TeamName:    row.TeamName,
			Status:      row.Status,
			Description: row.Description,
			Date:        row.ReportDate,
		}
	}
	return result, true
}

// persistInjurySnapshots 将当日伤病快照替换保存到数据库。
func (s *LineupRecommendService) persistInjurySnapshots(
	ctx context.Context,
	gameDate string,
	rows []entity.NBAGameInjurySnapshot,
) error {
	repo := repositories.NewNBAGameInjurySnapshotRepository(s.db.DB)
	now := time.Now()
	for i := range rows {
		rows[i].GameDate = gameDate
		rows[i].FetchedAt = now
	}
	return repo.ReplaceByGameDate(ctx, gameDate, rows)
}

// pickInjuryMatchedPlayer 将伤病报告与候选球员匹配，优先精确匹配英文名，其次模糊匹配。
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

// selectPlayerByTeamCode 从候选球员中按球队代码选择第一个匹配的球员。
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

// rebalanceAvailabilityScore 根据伤病状态微调可用性分数，避免某些状态被过度惩罚。
func rebalanceAvailabilityScore(status string, availabilityScore float64) float64 {
	if availabilityScore <= 0 || availabilityScore >= 1 {
		return availabilityScore
	}

	normalized := strings.ToLower(strings.TrimSpace(status))
	switch {
	case normalized == "day-to-day":
		// 上调下限，避免过度悲观
		return clamp(math.Max(availabilityScore, 0.85), 0.0, 1.0)
	case strings.Contains(normalized, "questionable"):
		// 上调下限，减少过度折扣
		return clamp(math.Max(availabilityScore, 0.78), 0.0, 1.0)
	case strings.Contains(normalized, "probable"):
		// probable 状态给予更高下限
		return clamp(math.Max(availabilityScore, 0.92), 0.0, 1.0)
	default:
		return availabilityScore
	}
}

// loadDBPlayerMap 从 players 表批量加载 DB 球员记录（含历史场均战力等锚定数据）。
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

// loadSalaryTxPlayerIDMap 从 nba_player_salary 表加载 NBAPlayerID → TxPlayerID 映射。
func (s *LineupRecommendService) loadSalaryTxPlayerIDMap(ctx context.Context, gamePlayers []entity.NBAGamePlayer) map[uint]uint {
	result := make(map[uint]uint)
	nbaIDs := collectCandidateNBAPlayerIDs(gamePlayers)
	if len(nbaIDs) == 0 {
		return result
	}

	salaryRepo := repositories.NewNBAPlayerSalaryRepository(s.db.DB)
	rows, err := salaryRepo.BatchGetByNBAPlayerIDs(ctx, nbaIDs)
	if err != nil {
		log.Printf("查询薪资库映射失败: %v", err)
		return result
	}

	for _, row := range rows {
		if row.NBAPlayerID == 0 || row.TxPlayerID == 0 {
			continue
		}
		result[row.NBAPlayerID] = row.TxPlayerID
	}
	return result
}

// stripPredictAnchors 清除 DB 球员的 PowerPer5/PowerPer10 锚定数据（用于回测场景禁用锚定）。
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

// loadGameStatsMap 批量加载 TX 球员近 10 场比赛数据，按比赛日期倒序排列。
// 支持历史模式（仅查询 gameDate 之前的数据）。
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

// loadSeasonStatsMap 批量加载 TX 球员本赛季汇总数据。历史模式下跳过。
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

// loadTeamMatchupMetrics 加载球队防守效率/节奏/近期状态/篮下威慑/外线干扰指标。
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

// buildTeamMatchupMetricsFromAggregates 从聚合数据构建球队防守指标映射。
// 计算每支球队的场均失分、节奏、近期趋势、盖帽威慑和抢断干扰与联盟均值的比率。
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
