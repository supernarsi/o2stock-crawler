package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

// ImportActualFeedback 导入赛后真实战力反馈（按比赛日全量替换）。
func (s *LineupRecommendService) ImportActualFeedback(ctx context.Context, gameDate string, feedbackFile string) error {
	raw, err := os.ReadFile(feedbackFile)
	if err != nil {
		return fmt.Errorf("读取反馈文件失败: %w", err)
	}

	resolvedDate, items, err := resolveActualFeedbackItems(raw)
	if err != nil {
		return fmt.Errorf("解析反馈文件失败: %w", err)
	}
	if gameDate == "" {
		gameDate = resolvedDate
	}
	if gameDate == "" {
		return fmt.Errorf("缺少比赛日期，请传入参数或在 JSON 中提供 game_date")
	}
	if resolvedDate != "" && resolvedDate != gameDate {
		return fmt.Errorf("反馈日期不一致: 参数=%s, JSON=%s", gameDate, resolvedDate)
	}
	if len(items) == 0 {
		return fmt.Errorf("反馈文件中无有效球员数据")
	}

	salaryMap := make(map[uint]uint)
	gamePlayerRepo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	gamePlayers, err := gamePlayerRepo.GetByGameDate(ctx, gameDate)
	if err != nil {
		log.Printf("加载候选球员工资失败，将仅使用反馈中的 salary: %v", err)
	} else {
		for _, p := range gamePlayers {
			salaryMap[p.NBAPlayerID] = p.Salary
		}
	}

	defaultSource := "manual_json"
	dedup := make(map[string]entity.NBAGamePlayerActual)
	for _, item := range items {
		if item.Rank == 0 || item.NBAPlayerID == 0 || item.ActualPower == nil {
			continue
		}

		source := strings.TrimSpace(item.Source)
		if source == "" {
			source = defaultSource
		}

		salary := salaryMap[item.NBAPlayerID]
		if item.Salary != nil {
			salary = *item.Salary
		}

		key := fmt.Sprintf("%d:%d", item.Rank, item.NBAPlayerID)
		dedup[key] = entity.NBAGamePlayerActual{
			GameDate:    gameDate,
			Rank:        item.Rank,
			NBAPlayerID: item.NBAPlayerID,
			Salary:      salary,
			ActualPower: roundTo(*item.ActualPower, 1),
			Source:      source,
		}
	}
	if len(dedup) == 0 {
		return fmt.Errorf("反馈文件中没有可导入的有效 actual_power")
	}

	rows := make([]entity.NBAGamePlayerActual, 0, len(dedup))
	for _, row := range dedup {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Rank != rows[j].Rank {
			return rows[i].Rank < rows[j].Rank
		}
		return rows[i].NBAPlayerID < rows[j].NBAPlayerID
	})

	actualRepo := repositories.NewNBAGamePlayerActualRepository(s.db.DB)
	if err := actualRepo.ReplaceByGameDate(ctx, gameDate, rows); err != nil {
		return fmt.Errorf("写入反馈失败: %w", err)
	}

	log.Printf("已导入真实战力反馈: %d 名球员 (日期: %s)", len(rows), gameDate)
	return nil
}

// RunBacktest 基于比赛数据(主) + 用户反馈(补充覆盖)回测推荐结果，并给出真实最优 TopN
func (s *LineupRecommendService) RunBacktest(ctx context.Context, gameDate string, topN int) error {
	if topN <= 0 {
		topN = defaultTopN
	}

	lineupRepo := repositories.NewLineupRecommendationRepository(s.db.DB)
	recs, err := lineupRepo.GetByDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询推荐阵容失败: %w", err)
	}
	if len(recs) == 0 {
		return fmt.Errorf("无推荐阵容数据，请先执行 recommend: %s", gameDate)
	}

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

	actualRepo := repositories.NewNBAGamePlayerActualRepository(s.db.DB)
	actualRows, err := actualRepo.GetByGameDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询真实战力反馈失败: %w", err)
	}

	actualMap, summary, err := s.buildBacktestActualPowerMap(ctx, gameDate, gamePlayers, actualRows)
	if err != nil {
		return err
	}
	if len(actualMap) == 0 {
		return fmt.Errorf("无可用真实战力数据: %s（player_game_stats 与 feedback 均未命中）", gameDate)
	}
	log.Printf(
		"回测真实战力覆盖(%s): 候选=%d, NBA->TX映射=%d(手工兜底=%d, 冲突=%d), stats命中=%d, feedback去重=%d(重复=%d, 覆盖替换=%d), 最终覆盖=%d",
		gameDate,
		summary.CandidateCount,
		summary.MappedTxCount,
		summary.ManualMapAppliedCount,
		summary.MappingConflictCount,
		summary.StatsHitCount,
		summary.FeedbackCount,
		summary.FeedbackDuplicateCount,
		summary.FeedbackOverrideCount,
		summary.FinalCoverageCount,
	)

	// A. 推荐结果的实际得分（写回 lineup_recommendation.total_actual_power）
	recRows := make([]entity.LineupBacktestResult, 0, min(topN, len(recs)))
	rankPowerMap := make(map[uint]float64, len(recs))
	for _, rec := range recs {
		actualTotal := calcLineupActualTotal(pickLineupPlayerIDs(rec), actualMap)
		rankPowerMap[rec.Rank] = actualTotal

		if int(rec.Rank) <= topN {
			row, ok := s.buildBacktestRowFromRecommendation(gameDate, rec, actualTotal, playerMap)
			if ok {
				recRows = append(recRows, row)
			}
		}
	}
	if err := lineupRepo.BatchUpdateActualPower(ctx, gameDate, rankPowerMap); err != nil {
		return fmt.Errorf("回写推荐阵容实际战力失败: %w", err)
	}

	// B. 以真实战力为目标值，重新求解真实最优 TopN（tx 视角，不依赖 NBA->TX 映射完整性）
	actualCandidates, actualCandidateSummary, err := s.buildBacktestActualCandidates(ctx, gameDate, gamePlayers, actualRows)
	if err != nil {
		return err
	}
	if len(actualCandidates) == 0 {
		return fmt.Errorf("无可用真实最优候选: %s", gameDate)
	}
	log.Printf(
		"回测真实最优候选(%s): stats球员=%d, 映射NBA=%d, tx-only=%d, feedback补充=%d, tx默认工资=%d",
		gameDate,
		actualCandidateSummary.StatsCandidateCount,
		actualCandidateSummary.MappedNBACount,
		actualCandidateSummary.TxOnlyCount,
		actualCandidateSummary.FeedbackOnlyCount,
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
		))
	}

	backtestRepo := repositories.NewLineupBacktestResultRepository(s.db.DB)
	if err := backtestRepo.ReplaceByGameDateAndType(ctx, gameDate, entity.LineupBacktestResultTypeRecommendedActual, recRows); err != nil {
		return fmt.Errorf("保存推荐实得回测结果失败: %w", err)
	}
	if err := backtestRepo.ReplaceByGameDateAndType(ctx, gameDate, entity.LineupBacktestResultTypeActualOptimal, optRows); err != nil {
		return fmt.Errorf("保存真实最优回测结果失败: %w", err)
	}

	s.printBacktestSummary(gameDate, recRows, optRows, playerMap)
	log.Printf(">>> 回测完成，结果已保存到 lineup_backtest_result 表 <<<")
	return nil
}

// --- 生成推荐 ---

// GenerateRecommendation 生成指定日期的推荐阵容

// resolveActualFeedbackItems 解析用户回传的反馈 JSON。
// 当前仅支持 list/rank/players 的阵容格式。
func resolveActualFeedbackItems(raw []byte) (string, []ActualFeedbackItem, error) {
	var lineupPayload ActualFeedbackLineupPayload
	if err := json.Unmarshal(raw, &lineupPayload); err != nil {
		return "", nil, fmt.Errorf("反馈 JSON 解析失败: %w", err)
	}
	if len(lineupPayload.List) == 0 {
		return "", nil, fmt.Errorf("仅支持 list 阵容格式，且 list 不能为空")
	}

	date := strings.TrimSpace(lineupPayload.GameDate)
	source := strings.TrimSpace(lineupPayload.Source)
	items := make([]ActualFeedbackItem, 0)
	for _, lineup := range lineupPayload.List {
		if lineup.Rank == 0 {
			return "", nil, fmt.Errorf("list.rank 不能为空")
		}
		if lineup.Rank > 3 {
			return "", nil, fmt.Errorf("list.rank 超出范围: %d（最多 3）", lineup.Rank)
		}
		for _, player := range lineup.Players {
			if player.NBAPlayerID == 0 {
				return "", nil, fmt.Errorf("list.rank=%d 存在空 nba_player_id", lineup.Rank)
			}
			if player.ActualPower == nil {
				return "", nil, fmt.Errorf("list.rank=%d player=%d 缺少 actual_power", lineup.Rank, player.NBAPlayerID)
			}
			player.Rank = lineup.Rank
			if source != "" && strings.TrimSpace(player.Source) == "" {
				player.Source = source
			}
			items = append(items, player)
		}
	}
	if len(items) == 0 {
		return "", nil, fmt.Errorf("list.players 不能为空")
	}
	return date, items, nil
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

type backtestActualPowerSummary struct {
	CandidateCount         int
	MappedTxCount          int
	ManualMapAppliedCount  int
	MappingConflictCount   int
	StatsHitCount          int
	FeedbackCount          int
	FeedbackDuplicateCount int
	FeedbackOverrideCount  int
	FinalCoverageCount     int
}

type backtestActualCandidateSummary struct {
	StatsCandidateCount      int
	MappedNBACount           int
	TxOnlyCount              int
	FeedbackOnlyCount        int
	TxOnlyDefaultSalaryCount int
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

// players 表映射缺失时的回测兜底映射（仅用于 backtest，不影响线上推荐）。
// key: nba_player_id, value: tx_player_id
var manualNBATxPlayerIDOverrides = map[uint]uint{
	1631157: 196154, // Ryan Rollins（莱恩.罗林斯）
	1631119: 196152, // Jaylin.Williams（杰林.威廉姆斯）
}

// buildBacktestActualPowerMap 构建回测真实战力映射，优先来源顺序如下：
// 1) player_game_stats（通过 players 表建立 nba_player_id -> tx_player_id 映射）
// 2) nba_game_player_actual（用户反馈，覆盖同球员的 stats 推导值）
func (s *LineupRecommendService) buildBacktestActualPowerMap(
	ctx context.Context,
	gameDate string,
	gamePlayers []entity.NBAGamePlayer,
	feedbackRows []entity.NBAGamePlayerActual,
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
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	dbPlayers, err := playerRepo.BatchGetByNBAPlayerIDs(ctx, nbaPlayerIDs)
	if err != nil {
		return nil, summary, fmt.Errorf("查询 NBA->TX 映射失败: %w", err)
	}

	nbaToTxMap, mappingConflicts := buildNBAToTxPlayerIDMap(dbPlayers)
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

	feedbackMap, feedbackDup := dedupeFeedbackActualMap(feedbackRows)
	summary.FeedbackDuplicateCount = feedbackDup
	for nbaPlayerID, power := range feedbackMap {
		if _, ok := candidateSet[nbaPlayerID]; !ok {
			continue
		}
		summary.FeedbackCount++
		if existing, exists := actualMap[nbaPlayerID]; exists && existing != power {
			summary.FeedbackOverrideCount++
		}
		actualMap[nbaPlayerID] = power
	}
	summary.FinalCoverageCount = len(actualMap)
	return actualMap, summary, nil
}

func (s *LineupRecommendService) buildBacktestActualCandidates(
	ctx context.Context,
	gameDate string,
	gamePlayers []entity.NBAGamePlayer,
	feedbackRows []entity.NBAGamePlayerActual,
) ([]PlayerCandidate, backtestActualCandidateSummary, error) {
	summary := backtestActualCandidateSummary{}
	if len(gamePlayers) == 0 {
		return nil, summary, nil
	}

	salaryByNBA, nameByNBA, candidateSet := buildGamePlayerMetadata(gamePlayers)
	nbaPlayerIDs := collectCandidateNBAPlayerIDs(gamePlayers)

	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	dbPlayers, err := playerRepo.BatchGetByNBAPlayerIDs(ctx, nbaPlayerIDs)
	if err != nil {
		return nil, summary, fmt.Errorf("查询 NBA->TX 映射失败: %w", err)
	}
	nbaToTxMap, _ := buildNBAToTxPlayerIDMap(dbPlayers)
	applyManualNBATxPlayerIDOverrides(nbaToTxMap, candidateSet)
	txToNBAMap := buildTxToNBAMap(nbaToTxMap, candidateSet)

	statsRepo := repositories.NewStatsRepository(s.db.DB)
	statsByTx, err := statsRepo.GetGameStatsByDate(ctx, gameDate)
	if err != nil {
		return nil, summary, fmt.Errorf("查询 player_game_stats 失败: %w", err)
	}
	summary.StatsCandidateCount = len(statsByTx)

	candidates := make([]PlayerCandidate, 0, len(statsByTx)+len(feedbackRows))
	txCandidateIndex := make(map[uint]int, len(statsByTx))
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
		txCandidateIndex[txPlayerID] = len(candidates) - 1
	}

	feedbackMap, _ := dedupeFeedbackActualMap(feedbackRows)
	feedbackSalaryMap := dedupeFeedbackSalaryMap(feedbackRows)
	for nbaPlayerID, power := range feedbackMap {
		salary := salaryByNBA[nbaPlayerID]
		if feedbackSalary, ok := feedbackSalaryMap[nbaPlayerID]; ok && feedbackSalary > 0 {
			salary = feedbackSalary
		}
		if salary == 0 {
			salary = backtestTxOnlyDefaultSalary
		}

		txPlayerID := nbaToTxMap[nbaPlayerID]
		if txPlayerID > 0 {
			if idx, exists := txCandidateIndex[txPlayerID]; exists {
				candidates[idx].Prediction.PredictedPower = power
				if candidates[idx].Player.NBAPlayerID == 0 {
					candidates[idx].Player.NBAPlayerID = nbaPlayerID
				}
				if candidates[idx].Player.Salary == 0 {
					candidates[idx].Player.Salary = salary
				}
				if trimmed := strings.TrimSpace(nameByNBA[nbaPlayerID]); trimmed != "" {
					candidates[idx].BacktestName = trimmed
				}
				continue
			}
		}

		name := strings.TrimSpace(nameByNBA[nbaPlayerID])
		if name == "" {
			name = "-"
		}
		candidates = append(candidates, PlayerCandidate{
			Player: entity.NBAGamePlayer{
				NBAPlayerID: nbaPlayerID,
				Salary:      salary,
			},
			Prediction: PlayerPrediction{
				PredictedPower: power,
			},
			BacktestTxPlayerID: txPlayerID,
			BacktestName:       name,
		})
		summary.FeedbackOnlyCount++
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

func dedupeFeedbackSalaryMap(rows []entity.NBAGamePlayerActual) map[uint]uint {
	feedbackSalary := make(map[uint]uint, len(rows))
	for _, row := range rows {
		if row.NBAPlayerID == 0 {
			continue
		}
		if _, exists := feedbackSalary[row.NBAPlayerID]; exists {
			continue
		}
		feedbackSalary[row.NBAPlayerID] = row.Salary
	}
	return feedbackSalary
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

func buildNBAToTxPlayerIDMap(players []entity.Player) (map[uint]uint, int) {
	nbaToTxMap := make(map[uint]uint, len(players))
	conflictCount := 0
	for _, player := range players {
		if player.NBAPlayerID == 0 || player.TxPlayerID == 0 {
			continue
		}

		existing, exists := nbaToTxMap[player.NBAPlayerID]
		if exists {
			if existing != player.TxPlayerID {
				conflictCount++
			}
			continue
		}
		nbaToTxMap[player.NBAPlayerID] = player.TxPlayerID
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

func dedupeFeedbackActualMap(rows []entity.NBAGamePlayerActual) (map[uint]float64, int) {
	feedbackMap := make(map[uint]float64, len(rows))
	dupCount := 0
	for _, row := range rows {
		if row.NBAPlayerID == 0 {
			continue
		}
		if _, exists := feedbackMap[row.NBAPlayerID]; exists {
			dupCount++
			continue
		}
		feedbackMap[row.NBAPlayerID] = roundTo(row.ActualPower, 1)
	}
	return feedbackMap, dupCount
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
		"result_type":           "recommended_actual",
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

	detailData := backtestDetailPayload{
		ResultType:         "actual_optimal",
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

func (s *LineupRecommendService) printBacktestSummary(
	gameDate string,
	recRows []entity.LineupBacktestResult,
	optRows []entity.LineupBacktestResult,
	playerMap map[uint]entity.NBAGamePlayer,
) {
	fmt.Printf("\n>>> 今日NBA回测结果 — %s <<<\n\n", gameDate)

	limit := min(len(recRows), len(optRows))
	if limit == 0 {
		fmt.Println("无可对比结果")
		return
	}

	sort.Slice(recRows, func(i, j int) bool { return recRows[i].Rank < recRows[j].Rank })
	sort.Slice(optRows, func(i, j int) bool { return optRows[i].Rank < optRows[j].Rank })

	for i := 0; i < limit; i++ {
		rec := recRows[i]
		opt := optRows[i]
		gap := roundTo(opt.TotalActualPower-rec.TotalActualPower, 1)
		fmt.Printf("#%d 推荐实得 %.1f vs 真实最优 %.1f (差距 %.1f)\n",
			i+1, rec.TotalActualPower, opt.TotalActualPower, gap)
		fmt.Printf("   真实最优阵容: %s\n", formatBacktestPlayersFromRow(opt, playerMap))
	}
	fmt.Println()
}

func formatBacktestPlayersFromRow(row entity.LineupBacktestResult, playerMap map[uint]entity.NBAGamePlayer) string {
	if slots := parseBacktestLineupSlots(row.DetailJSON); len(slots) > 0 {
		return formatBacktestPlayersBySlots(slots, playerMap)
	}
	return formatBacktestPlayers(
		[5]uint{row.Player1ID, row.Player2ID, row.Player3ID, row.Player4ID, row.Player5ID},
		playerMap,
	)
}

func parseBacktestLineupSlots(detailJSON string) []backtestLineupSlot {
	if strings.TrimSpace(detailJSON) == "" {
		return nil
	}
	var payload backtestDetailPayload
	if err := json.Unmarshal([]byte(detailJSON), &payload); err != nil {
		return nil
	}
	return payload.Lineup
}

func formatBacktestPlayersBySlots(slots []backtestLineupSlot, playerMap map[uint]entity.NBAGamePlayer) string {
	parts := make([]string, 0, len(slots))
	for _, slot := range slots {
		if slot.NBAPlayerID == 0 && slot.TxPlayerID == 0 {
			continue
		}

		if slot.NBAPlayerID > 0 {
			name := resolveBacktestPlayerName(slot.NBAPlayerID, slot.PlayerName, playerMap)
			parts = append(parts, fmt.Sprintf("%d:%s", slot.NBAPlayerID, name))
			continue
		}

		name := strings.TrimSpace(slot.PlayerName)
		if name == "" {
			name = "-"
		}
		parts = append(parts, fmt.Sprintf("%d(tx):%s", slot.TxPlayerID, name))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func resolveBacktestPlayerName(nbaPlayerID uint, fallback string, playerMap map[uint]entity.NBAGamePlayer) string {
	if player, ok := playerMap[nbaPlayerID]; ok {
		if trimmed := strings.TrimSpace(player.PlayerName); trimmed != "" {
			return trimmed
		}
	}
	trimmed := strings.TrimSpace(fallback)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

func formatBacktestPlayers(ids [5]uint, playerMap map[uint]entity.NBAGamePlayer) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		name := "-"
		if player, ok := playerMap[id]; ok {
			trimmed := strings.TrimSpace(player.PlayerName)
			if trimmed != "" {
				name = trimmed
			}
		}
		parts = append(parts, fmt.Sprintf("%d:%s", id, name))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// --- 文件读取 ---
