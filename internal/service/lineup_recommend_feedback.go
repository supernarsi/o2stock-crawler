package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

// 格式化和打印函数已迁移到 lineup_recommend_format.go

// RunBacktest 基于 player_game_stats 回测推荐结果，并给出真实最优 TopN
func (s *LineupRecommendService) RunBacktest(ctx context.Context, gameDate string, topN int) error {
	if topN <= 0 {
		topN = defaultTopN
	}

	lineupRepo := repositories.NewLineupRecommendationRepository(s.db.DB)
	allRecs, err := lineupRepo.GetAllByDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询推荐阵容失败: %w", err)
	}
	if len(allRecs) == 0 {
		return fmt.Errorf("无推荐阵容数据，请先执行 recommend: %s", gameDate)
	}

	// 仅 AI 推荐用于回测展示，其余类型仅回写实际战力
	recs := filterRecommendationsByType(allRecs, entity.LineupRecommendationTypeAIRecommended)

	gamePlayerRepo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	gamePlayers, err := gamePlayerRepo.GetByGameDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询候选球员失败: %w", err)
	}
	if len(gamePlayers) == 0 {
		return fmt.Errorf("无候选球员数据: %s", gameDate)
	}
	playerMap := make(map[uint]entity.NBAGamePlayer, len(gamePlayers))
	for _, p := range gamePlayers {
		playerMap[p.NBAPlayerID] = p
	}

	actualMap, summary, err := s.buildBacktestActualPowerMap(ctx, gameDate, gamePlayers)
	if err != nil {
		return err
	}
	if len(actualMap) == 0 {
		return fmt.Errorf("无可用真实战力数据: %s（player_game_stats 未命中）", gameDate)
	}
	log.Printf(
		"回测真实战力覆盖(%s): 候选=%d, NBA->TX映射=%d(手工兜底=%d, 冲突=%d), stats命中=%d, 最终覆盖=%d",
		gameDate,
		summary.CandidateCount,
		summary.MappedTxCount,
		summary.ManualMapAppliedCount,
		summary.MappingConflictCount,
		summary.StatsHitCount,
		summary.FinalCoverageCount,
	)

	// A. 推荐结果的实际得分（写回 lineup_recommendation.total_actual_power）
	recRows := make([]entity.LineupBacktestResult, 0, min(topN, len(recs)))
	actualPowerByLineup := make(map[[5]uint]float64, len(allRecs))
	for _, rec := range allRecs {
		playerIDs := pickLineupPlayerIDs(rec)
		actualPowerByLineup[playerIDs] = calcLineupActualTotal(playerIDs, actualMap)
	}
	for _, rec := range recs {
		actualTotal := actualPowerByLineup[pickLineupPlayerIDs(rec)]

		if int(rec.Rank) <= topN {
			row, ok := s.buildBacktestRowFromRecommendation(gameDate, rec, actualTotal, playerMap)
			if ok {
				recRows = append(recRows, row)
			}
		}
	}
	if err := lineupRepo.BatchUpdateActualPowerForAllTypes(ctx, gameDate, allRecs, actualPowerByLineup); err != nil {
		return fmt.Errorf("回写推荐阵容实际战力失败: %w", err)
	}

	// 均值基准部分尽量复用推荐阶段的候选与伤病过滤逻辑。
	injuryMap := map[uint]crawler.InjuryReport{}
	if snapshotMap, ok := s.loadInjurySnapshotMap(ctx, gameDate); ok {
		injuryMap = snapshotMap
		log.Printf("回测伤病快照: 命中 %d 名球员", len(injuryMap))
	} else {
		log.Printf("回测伤病快照缺失: %s，将跳过均值基准的伤病过滤", gameDate)
	}

	benchmarkRowsByType := make(map[uint8][]entity.LineupBacktestResult, 2)
	for _, lookback := range []int{3, 5} {
		benchmarkType := backtestBenchmarkResultType(lookback)
		if benchmarkType == 0 {
			continue
		}

		benchmarkCandidates, benchmarkSummary, err := s.buildAverageRecommendationCandidates(ctx, gameDate, gamePlayers, lookback, injuryMap)
		if err != nil {
			return err
		}
		log.Printf(
			"回测均值基准候选(%s): lookback=%d, 候选=%d, NBA映射=%d(手工兜底=%d), 历史命中=%d, 历史不足=%d, 伤病排除=%d",
			gameDate,
			benchmarkSummary.LookbackGames,
			benchmarkSummary.CandidateCount,
			benchmarkSummary.MappedNBACount,
			benchmarkSummary.ManualMapApplied,
			benchmarkSummary.HistoryHitCount,
			benchmarkSummary.InsufficientHistory,
			benchmarkSummary.InjuryFilteredCount,
		)

		lineups := s.solveOptimalLineupAllowZero(benchmarkCandidates, defaultSalaryCap, defaultPickCount, topN)
		if len(lineups) == 0 {
			log.Printf("回测均值基准未找到可行阵容: %s lookback=%d", gameDate, lookback)
			continue
		}

		rows := make([]entity.LineupBacktestResult, 0, len(lineups))
		for i, lineup := range lineups {
			actualTotal := calcLineupActualFromCandidates(lineup, actualMap)
			predictedTotal := calcLineupPredictedFromCandidates(lineup)
			rows = append(rows, s.buildBacktestRowFromCandidates(
				gameDate,
				uint(i+1),
				benchmarkType,
				lineup,
				fmt.Sprintf("%d_game_average_baseline", lookback),
				predictedTotal,
				actualTotal,
			))
		}
		benchmarkRowsByType[benchmarkType] = rows
	}

	// B. 以真实战力为目标值，重新求解真实最优 TopN（tx 视角，不依赖 NBA->TX 映射完整性）
	actualCandidates, actualCandidateSummary, err := s.buildBacktestActualCandidates(ctx, gameDate, gamePlayers)
	if err != nil {
		return err
	}
	if len(actualCandidates) == 0 {
		return fmt.Errorf("无可用真实最优候选: %s", gameDate)
	}
	log.Printf(
		"回测真实最优候选(%s): stats球员=%d, 映射NBA=%d, tx-only=%d, tx默认工资=%d",
		gameDate,
		actualCandidateSummary.StatsCandidateCount,
		actualCandidateSummary.MappedNBACount,
		actualCandidateSummary.TxOnlyCount,
		actualCandidateSummary.TxOnlyDefaultSalaryCount,
	)

	bestActualLineups := s.solveOptimalLineupAllowZero(actualCandidates, defaultSalaryCap, defaultPickCount, topN)
	if len(bestActualLineups) == 0 {
		return fmt.Errorf("未找到可行真实最优阵容（工资帽=%d）", defaultSalaryCap)
	}

	optRows := make([]entity.LineupBacktestResult, 0, len(bestActualLineups))
	for i, lineup := range bestActualLineups {
		optRows = append(optRows, s.buildBacktestRowFromCandidates(
			gameDate,
			uint(i+1),
			entity.LineupBacktestResultTypeActualOptimal,
			lineup,
			"",
			0,
			-1,
		))
	}

	backtestRepo := repositories.NewLineupBacktestResultRepository(s.db.DB)
	if err := backtestRepo.ReplaceByGameDateAndType(ctx, gameDate, entity.LineupBacktestResultTypeRecommendedActual, recRows); err != nil {
		return fmt.Errorf("保存推荐实得回测结果失败: %w", err)
	}
	for resultType, rows := range benchmarkRowsByType {
		if err := backtestRepo.ReplaceByGameDateAndType(ctx, gameDate, resultType, rows); err != nil {
			return fmt.Errorf("保存均值基准回测结果失败(type=%d): %w", resultType, err)
		}
	}
	if err := backtestRepo.ReplaceByGameDateAndType(ctx, gameDate, entity.LineupBacktestResultTypeActualOptimal, optRows); err != nil {
		return fmt.Errorf("保存真实最优回测结果失败: %w", err)
	}

	s.printBacktestSummary(gameDate, recRows, benchmarkRowsByType, optRows, playerMap)
	log.Printf(">>> 回测完成，结果已保存到 lineup_backtest_result 表 <<<")
	return nil
}

func pickLineupPlayerIDs(rec entity.LineupRecommendation) [5]uint {
	return [5]uint{
		rec.Player1ID,
		rec.Player2ID,
		rec.Player3ID,
		rec.Player4ID,
		rec.Player5ID,
	}
}

func calcLineupActualTotal(playerIDs [5]uint, actualMap map[uint]float64) float64 {
	total := 0.0
	for _, id := range playerIDs {
		total += actualMap[id]
	}
	return roundTo(total, 1)
}

func calcLineupActualFromCandidates(lineup []PlayerCandidate, actualMap map[uint]float64) float64 {
	total := 0.0
	for _, candidate := range lineup {
		total += actualMap[candidate.Player.NBAPlayerID]
	}
	return roundTo(total, 1)
}

func calcLineupPredictedFromCandidates(lineup []PlayerCandidate) float64 {
	total := 0.0
	for _, candidate := range lineup {
		total += candidate.Prediction.PredictedPower
	}
	return roundTo(total, 1)
}

func backtestBenchmarkResultType(lookback int) uint8 {
	switch lookback {
	case 3:
		return entity.LineupBacktestResultTypeAvg3Benchmark
	case 5:
		return entity.LineupBacktestResultTypeAvg5Benchmark
	default:
		return 0
	}
}

type backtestActualPowerSummary struct {
	CandidateCount        int
	MappedTxCount         int
	ManualMapAppliedCount int
	MappingConflictCount  int
	StatsHitCount         int
	FinalCoverageCount    int
}

type backtestActualCandidateSummary struct {
	StatsCandidateCount      int
	MappedNBACount           int
	TxOnlyCount              int
	TxOnlyDefaultSalaryCount int
}

type backtestBenchmarkCandidateSummary struct {
	LookbackGames       int
	CandidateCount      int
	MappedNBACount      int
	ManualMapApplied    int
	HistoryHitCount     int
	InsufficientHistory int
	InjuryFilteredCount int
}

type backtestLineupSlot struct {
	Slot        uint    `json:"slot"`
	TxPlayerID  uint    `json:"tx_player_id"`
	NBAPlayerID uint    `json:"nba_player_id"`
	PlayerName  string  `json:"player_name"`
	Salary      uint    `json:"salary"`
	ActualPower float64 `json:"actual_power"`
	IDSource    string  `json:"id_source"`
}

type backtestDetailPayload struct {
	ResultType          string               `json:"result_type"`
	Note                string               `json:"note,omitempty"`
	PredictedTotalPower float64              `json:"predicted_total_power,omitempty"`
	Lineup              []backtestLineupSlot `json:"lineup,omitempty"`
	LineupTxPlayerIDs   []uint               `json:"lineup_tx_player_ids,omitempty"`
	LineupNBAPlayerIDs  []uint               `json:"lineup_nba_player_ids,omitempty"`
}

const backtestTxOnlyDefaultSalary uint = 5

// nba_player_salary 表映射缺失时的兜底映射（用于 recommend/backtest 的缺失补位）。
// key: nba_player_id, value: tx_player_id
var manualNBATxPlayerIDOverrides = map[uint]uint{
	1631157: 196154, // Ryan Rollins（莱恩.罗林斯）
	1631119: 196152, // Jaylin.Williams（杰林.威廉姆斯）
	1642271: 272768, // Kyle Filipowski（凯尔.菲利波夫斯基）
	1629674: 175332, // Neemias.Queta（内米亚斯.奎塔）
	1630611: 196168, // Gui Santos（古伊.桑托斯）
	1642875: 332258, // Maxime.Raynaud（马克西姆.雷诺）
}

// buildBacktestActualPowerMap 构建回测真实战力映射，仅使用 player_game_stats。
func (s *LineupRecommendService) buildBacktestActualPowerMap(
	ctx context.Context,
	gameDate string,
	gamePlayers []entity.NBAGamePlayer,
) (map[uint]float64, backtestActualPowerSummary, error) {
	summary := backtestActualPowerSummary{
		CandidateCount: len(gamePlayers),
	}
	actualMap := make(map[uint]float64)
	if len(gamePlayers) == 0 {
		return actualMap, summary, nil
	}
	candidateSet := make(map[uint]struct{}, len(gamePlayers))
	for _, candidate := range gamePlayers {
		if candidate.NBAPlayerID == 0 {
			continue
		}
		candidateSet[candidate.NBAPlayerID] = struct{}{}
	}

	nbaPlayerIDs := collectCandidateNBAPlayerIDs(gamePlayers)
	salaryRepo := repositories.NewNBAPlayerSalaryRepository(s.db.DB)
	salaryRows, err := salaryRepo.BatchGetByNBAPlayerIDs(ctx, nbaPlayerIDs)
	if err != nil {
		return nil, summary, fmt.Errorf("查询薪资库 NBA->TX 映射失败: %w", err)
	}

	nbaToTxMap, mappingConflicts := buildNBAToTxPlayerIDMapFromSalary(salaryRows)
	manualApplied := applyManualNBATxPlayerIDOverrides(nbaToTxMap, candidateSet)
	summary.MappedTxCount = len(nbaToTxMap)
	summary.ManualMapAppliedCount = manualApplied
	summary.MappingConflictCount = mappingConflicts

	statsRepo := repositories.NewStatsRepository(s.db.DB)
	txPlayerIDs := collectUniqueTxPlayerIDs(nbaToTxMap)
	statsByTx, err := statsRepo.BatchGetGameStatsByDate(ctx, txPlayerIDs, gameDate)
	if err != nil {
		return nil, summary, fmt.Errorf("查询 player_game_stats 失败: %w", err)
	}

	for _, candidate := range gamePlayers {
		txID, ok := nbaToTxMap[candidate.NBAPlayerID]
		if !ok {
			continue
		}
		stat, ok := statsByTx[txID]
		if !ok {
			continue
		}
		actualMap[candidate.NBAPlayerID] = roundTo(calcPowerFromStats(stat), 1)
		summary.StatsHitCount++
	}

	summary.FinalCoverageCount = len(actualMap)
	return actualMap, summary, nil
}

func (s *LineupRecommendService) buildBacktestActualCandidates(
	ctx context.Context,
	gameDate string,
	gamePlayers []entity.NBAGamePlayer,
) ([]PlayerCandidate, backtestActualCandidateSummary, error) {
	summary := backtestActualCandidateSummary{}
	if len(gamePlayers) == 0 {
		return nil, summary, nil
	}

	salaryByNBA, nameByNBA, candidateSet := buildGamePlayerMetadata(gamePlayers)
	nbaPlayerIDs := collectCandidateNBAPlayerIDs(gamePlayers)

	salaryRepo := repositories.NewNBAPlayerSalaryRepository(s.db.DB)
	salaryRows, err := salaryRepo.BatchGetByNBAPlayerIDs(ctx, nbaPlayerIDs)
	if err != nil {
		return nil, summary, fmt.Errorf("查询薪资库 NBA->TX 映射失败: %w", err)
	}
	nbaToTxMap, _ := buildNBAToTxPlayerIDMapFromSalary(salaryRows)
	applyManualNBATxPlayerIDOverrides(nbaToTxMap, candidateSet)
	txToNBAMap := buildTxToNBAMap(nbaToTxMap, candidateSet)

	statsRepo := repositories.NewStatsRepository(s.db.DB)
	statsByTx, err := statsRepo.GetGameStatsByDate(ctx, gameDate)
	if err != nil {
		return nil, summary, fmt.Errorf("查询 player_game_stats 失败: %w", err)
	}
	summary.StatsCandidateCount = len(statsByTx)

	candidates := make([]PlayerCandidate, 0, len(statsByTx))
	for txPlayerID, stat := range statsByTx {
		nbaPlayerID := txToNBAMap[txPlayerID]
		salary := backtestTxOnlyDefaultSalary
		name := "-"

		if nbaPlayerID > 0 {
			salary = salaryByNBA[nbaPlayerID]
			if salary == 0 {
				salary = backtestTxOnlyDefaultSalary
				summary.TxOnlyDefaultSalaryCount++
			}
			if trimmed := strings.TrimSpace(nameByNBA[nbaPlayerID]); trimmed != "" {
				name = trimmed
			}
			summary.MappedNBACount++
		} else {
			summary.TxOnlyCount++
			summary.TxOnlyDefaultSalaryCount++
		}

		candidates = append(candidates, PlayerCandidate{
			Player: entity.NBAGamePlayer{
				NBAPlayerID: nbaPlayerID,
				Salary:      salary,
			},
			Prediction: PlayerPrediction{
				PredictedPower: roundTo(calcPowerFromStats(stat), 1),
			},
			BacktestTxPlayerID: txPlayerID,
			BacktestName:       name,
		})
	}

	for i := range candidates {
		if candidates[i].Player.Salary == 0 {
			candidates[i].Player.Salary = backtestTxOnlyDefaultSalary
			summary.TxOnlyDefaultSalaryCount++
		}
		if strings.TrimSpace(candidates[i].BacktestName) == "" {
			candidates[i].BacktestName = "-"
		}
	}

	return candidates, summary, nil
}

func (s *LineupRecommendService) buildBacktestAverageBenchmarkCandidates(
	ctx context.Context,
	gameDate string,
	gamePlayers []entity.NBAGamePlayer,
	lookback int,
) ([]PlayerCandidate, backtestBenchmarkCandidateSummary, error) {
	summary := backtestBenchmarkCandidateSummary{
		LookbackGames:  lookback,
		CandidateCount: len(gamePlayers),
	}
	if len(gamePlayers) == 0 || lookback <= 0 {
		return nil, summary, nil
	}

	salaryByNBA, nameByNBA, candidateSet := buildGamePlayerMetadata(gamePlayers)
	nbaPlayerIDs := collectCandidateNBAPlayerIDs(gamePlayers)

	salaryRepo := repositories.NewNBAPlayerSalaryRepository(s.db.DB)
	salaryRows, err := salaryRepo.BatchGetByNBAPlayerIDs(ctx, nbaPlayerIDs)
	if err != nil {
		return nil, summary, fmt.Errorf("查询均值基准薪资库 NBA->TX 映射失败: %w", err)
	}
	nbaToTxMap, _ := buildNBAToTxPlayerIDMapFromSalary(salaryRows)
	summary.ManualMapApplied = applyManualNBATxPlayerIDOverrides(nbaToTxMap, candidateSet)
	summary.MappedNBACount = len(nbaToTxMap)

	statsRepo := repositories.NewStatsRepository(s.db.DB)
	recentStatsByTx, err := statsRepo.BatchGetRecentGameStatsBeforeDate(ctx, collectUniqueTxPlayerIDs(nbaToTxMap), lookback, gameDate)
	if err != nil {
		return nil, summary, fmt.Errorf("查询均值基准历史比赛失败: %w", err)
	}

	candidates := make([]PlayerCandidate, 0, len(gamePlayers))
	for _, player := range gamePlayers {
		if player.NBAPlayerID == 0 {
			continue
		}
		txPlayerID := nbaToTxMap[player.NBAPlayerID]
		if txPlayerID == 0 {
			summary.InsufficientHistory++
			continue
		}

		stats := recentStatsByTx[txPlayerID]
		if len(stats) == 0 {
			summary.InsufficientHistory++
			continue
		}

		avgPower := calcAveragePowerFromStats(stats, lookback)
		if avgPower <= 0 {
			summary.InsufficientHistory++
			continue
		}

		name := strings.TrimSpace(nameByNBA[player.NBAPlayerID])
		if name == "" {
			name = strings.TrimSpace(player.PlayerName)
		}
		if name == "" {
			name = "-"
		}

		candidates = append(candidates, PlayerCandidate{
			Player: entity.NBAGamePlayer{
				NBAPlayerID:  player.NBAPlayerID,
				NBATeamID:    player.NBATeamID,
				MatchID:      player.MatchID,
				PlayerName:   player.PlayerName,
				PlayerEnName: player.PlayerEnName,
				TeamName:     player.TeamName,
				IsHome:       player.IsHome,
				Salary:       salaryByNBA[player.NBAPlayerID],
				Position:     player.Position,
				CombatPower:  player.CombatPower,
			},
			Prediction: PlayerPrediction{
				PredictedPower: avgPower,
			},
			BacktestTxPlayerID: txPlayerID,
			BacktestName:       name,
		})
		summary.HistoryHitCount++
	}

	return candidates, summary, nil
}

func (s *LineupRecommendService) buildAverageRecommendationCandidates(
	ctx context.Context,
	gameDate string,
	gamePlayers []entity.NBAGamePlayer,
	lookback int,
	injuryMap map[uint]crawler.InjuryReport,
) ([]PlayerCandidate, backtestBenchmarkCandidateSummary, error) {
	candidates, summary, err := s.buildBacktestAverageBenchmarkCandidates(ctx, gameDate, gamePlayers, lookback)
	if err != nil {
		return nil, summary, err
	}
	if len(candidates) == 0 {
		return candidates, summary, nil
	}

	filtered, filteredCount := filterBenchmarkCandidatesByInjury(candidates, injuryMap)
	summary.InjuryFilteredCount += filteredCount
	return filtered, summary, nil
}

func filterBenchmarkCandidatesByInjury(
	candidates []PlayerCandidate,
	injuryMap map[uint]crawler.InjuryReport,
) ([]PlayerCandidate, int) {
	if len(candidates) == 0 {
		return candidates, 0
	}

	filtered := make([]PlayerCandidate, 0, len(candidates))
	filteredCount := 0
	for _, candidate := range candidates {
		availabilityScore := resolveAvailabilityScore(candidate.Player, injuryMap)
		if availabilityScore == 0 {
			filteredCount++
			continue
		}

		candidate.Prediction.BaseValue = candidate.Prediction.PredictedPower
		candidate.Prediction.AvailabilityScore = availabilityScore
		candidate.Prediction.PredictedPower = roundTo(candidate.Prediction.PredictedPower*availabilityScore, 1)
		filtered = append(filtered, candidate)
	}

	return filtered, filteredCount
}

func calcAveragePowerFromStats(stats []entity.PlayerGameStats, lookback int) float64 {
	if len(stats) == 0 || lookback <= 0 {
		return 0
	}
	limit := min(len(stats), lookback)
	total := 0.0
	for i := 0; i < limit; i++ {
		total += calcPowerFromStats(stats[i])
	}
	return roundTo(total/float64(limit), 1)
}

func buildGamePlayerMetadata(players []entity.NBAGamePlayer) (map[uint]uint, map[uint]string, map[uint]struct{}) {
	salaryByNBA := make(map[uint]uint, len(players))
	nameByNBA := make(map[uint]string, len(players))
	candidateSet := make(map[uint]struct{}, len(players))
	for _, player := range players {
		if player.NBAPlayerID == 0 {
			continue
		}
		salaryByNBA[player.NBAPlayerID] = player.Salary
		nameByNBA[player.NBAPlayerID] = strings.TrimSpace(player.PlayerName)
		candidateSet[player.NBAPlayerID] = struct{}{}
	}
	return salaryByNBA, nameByNBA, candidateSet
}

func buildTxToNBAMap(nbaToTxMap map[uint]uint, candidateSet map[uint]struct{}) map[uint]uint {
	txToNBAMap := make(map[uint]uint, len(nbaToTxMap))
	if len(nbaToTxMap) == 0 {
		return txToNBAMap
	}

	nbaPlayerIDs := make([]uint, 0, len(nbaToTxMap))
	for nbaPlayerID := range nbaToTxMap {
		nbaPlayerIDs = append(nbaPlayerIDs, nbaPlayerID)
	}
	sort.Slice(nbaPlayerIDs, func(i, j int) bool { return nbaPlayerIDs[i] < nbaPlayerIDs[j] })

	for _, nbaPlayerID := range nbaPlayerIDs {
		txPlayerID := nbaToTxMap[nbaPlayerID]
		if txPlayerID == 0 {
			continue
		}
		if _, ok := candidateSet[nbaPlayerID]; !ok {
			continue
		}
		if _, exists := txToNBAMap[txPlayerID]; exists {
			continue
		}
		txToNBAMap[txPlayerID] = nbaPlayerID
	}
	return txToNBAMap
}

func collectCandidateNBAPlayerIDs(players []entity.NBAGamePlayer) []uint {
	if len(players) == 0 {
		return nil
	}
	set := make(map[uint]struct{}, len(players))
	for _, p := range players {
		if p.NBAPlayerID == 0 {
			continue
		}
		set[p.NBAPlayerID] = struct{}{}
	}
	result := make([]uint, 0, len(set))
	for nbaPlayerID := range set {
		result = append(result, nbaPlayerID)
	}
	return result
}

func buildNBAToTxPlayerIDMapFromSalary(rows []entity.NBAPlayerSalary) (map[uint]uint, int) {
	nbaToTxMap := make(map[uint]uint, len(rows))
	conflictCount := 0
	for _, row := range rows {
		if row.NBAPlayerID == 0 || row.TxPlayerID == 0 {
			continue
		}

		existing, exists := nbaToTxMap[row.NBAPlayerID]
		if exists {
			if existing != row.TxPlayerID {
				conflictCount++
			}
			continue
		}
		nbaToTxMap[row.NBAPlayerID] = row.TxPlayerID
	}
	return nbaToTxMap, conflictCount
}

func applyManualNBATxPlayerIDOverrides(nbaToTxMap map[uint]uint, candidateSet map[uint]struct{}) int {
	if len(manualNBATxPlayerIDOverrides) == 0 || len(candidateSet) == 0 {
		return 0
	}
	appliedCount := 0
	for nbaPlayerID, txPlayerID := range manualNBATxPlayerIDOverrides {
		if txPlayerID == 0 {
			continue
		}
		if _, ok := candidateSet[nbaPlayerID]; !ok {
			continue
		}
		if existing, exists := nbaToTxMap[nbaPlayerID]; exists && existing > 0 {
			continue
		}
		nbaToTxMap[nbaPlayerID] = txPlayerID
		appliedCount++
	}
	return appliedCount
}

func collectUniqueTxPlayerIDs(nbaToTxMap map[uint]uint) []uint {
	if len(nbaToTxMap) == 0 {
		return nil
	}
	txSet := make(map[uint]struct{}, len(nbaToTxMap))
	for _, txPlayerID := range nbaToTxMap {
		if txPlayerID == 0 {
			continue
		}
		txSet[txPlayerID] = struct{}{}
	}
	result := make([]uint, 0, len(txSet))
	for txPlayerID := range txSet {
		result = append(result, txPlayerID)
	}
	return result
}

func (s *LineupRecommendService) buildBacktestRowFromRecommendation(
	gameDate string,
	rec entity.LineupRecommendation,
	actualTotal float64,
	playerMap map[uint]entity.NBAGamePlayer,
) (entity.LineupBacktestResult, bool) {
	ids := pickLineupPlayerIDs(rec)
	totalSalary := rec.TotalSalary
	if totalSalary == 0 {
		for _, id := range ids {
			totalSalary += playerMap[id].Salary
		}
	}

	detail, _ := json.Marshal(map[string]any{
		"result_type":           backtestResultTypeName(entity.LineupBacktestResultTypeRecommendedActual),
		"predicted_total_power": rec.TotalPredictedPower,
		"actual_total_power":    actualTotal,
		"delta_actual_predict":  roundTo(actualTotal-rec.TotalPredictedPower, 1),
	})

	return entity.LineupBacktestResult{
		GameDate:         gameDate,
		ResultType:       entity.LineupBacktestResultTypeRecommendedActual,
		Rank:             rec.Rank,
		TotalActualPower: actualTotal,
		TotalSalary:      totalSalary,
		Player1ID:        ids[0],
		Player2ID:        ids[1],
		Player3ID:        ids[2],
		Player4ID:        ids[3],
		Player5ID:        ids[4],
		DetailJSON:       string(detail),
	}, true
}

func (s *LineupRecommendService) buildBacktestRowFromCandidates(
	gameDate string,
	rank uint,
	resultType uint8,
	lineup []PlayerCandidate,
	note string,
	predictedTotal float64,
	actualTotalOverride float64,
) entity.LineupBacktestResult {
	var totalActual float64
	var totalSalary uint
	var playerIDs [5]uint
	lineupSlots := make([]backtestLineupSlot, 0, len(lineup))
	lineupTxIDs := make([]uint, 0, len(lineup))
	lineupNBAIDs := make([]uint, 0, len(lineup))

	for i, c := range lineup {
		totalActual += c.Prediction.PredictedPower
		totalSalary += c.Player.Salary
		if i < len(playerIDs) {
			playerIDs[i] = c.Player.NBAPlayerID
		}

		playerName := strings.TrimSpace(c.BacktestName)
		if playerName == "" {
			playerName = "-"
		}

		idSource := "nba_mapped"
		if c.Player.NBAPlayerID == 0 && c.BacktestTxPlayerID > 0 {
			idSource = "tx_only"
		}
		if c.Player.NBAPlayerID > 0 && c.BacktestTxPlayerID == 0 {
			idSource = "feedback_only"
		}

		lineupSlots = append(lineupSlots, backtestLineupSlot{
			Slot:        uint(i + 1),
			TxPlayerID:  c.BacktestTxPlayerID,
			NBAPlayerID: c.Player.NBAPlayerID,
			PlayerName:  playerName,
			Salary:      c.Player.Salary,
			ActualPower: roundTo(c.Prediction.PredictedPower, 1),
			IDSource:    idSource,
		})
		lineupTxIDs = append(lineupTxIDs, c.BacktestTxPlayerID)
		lineupNBAIDs = append(lineupNBAIDs, c.Player.NBAPlayerID)
	}

	if actualTotalOverride >= 0 {
		totalActual = actualTotalOverride
	}

	detailData := backtestDetailPayload{
		ResultType:         backtestResultTypeName(resultType),
		Lineup:             lineupSlots,
		LineupTxPlayerIDs:  lineupTxIDs,
		LineupNBAPlayerIDs: lineupNBAIDs,
	}
	if note != "" {
		detailData.Note = note
	}
	if predictedTotal > 0 {
		detailData.PredictedTotalPower = roundTo(predictedTotal, 1)
	}
	detail, _ := json.Marshal(detailData)

	return entity.LineupBacktestResult{
		GameDate:         gameDate,
		ResultType:       resultType,
		Rank:             rank,
		TotalActualPower: roundTo(totalActual, 1),
		TotalSalary:      totalSalary,
		Player1ID:        playerIDs[0],
		Player2ID:        playerIDs[1],
		Player3ID:        playerIDs[2],
		Player4ID:        playerIDs[3],
		Player5ID:        playerIDs[4],
		DetailJSON:       string(detail),
	}
}

// filterRecommendationsByType 按类型过滤推荐阵容
func filterRecommendationsByType(recs []entity.LineupRecommendation, recommendationType uint8) []entity.LineupRecommendation {
	var result []entity.LineupRecommendation
	for _, rec := range recs {
		if rec.RecommendationType == recommendationType {
			result = append(result, rec)
		}
	}
	return result
}

// --- 文件读取 ---
