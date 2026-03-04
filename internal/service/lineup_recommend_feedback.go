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

// RunBacktest 基于反馈数据回测推荐结果，并给出真实最优 TopN
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

	actualRepo := repositories.NewNBAGamePlayerActualRepository(s.db.DB)
	actualRows, err := actualRepo.GetByGameDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询真实战力反馈失败: %w", err)
	}
	if len(actualRows) == 0 {
		return fmt.Errorf("无真实战力反馈数据，请先执行 import-actual/feedback: %s", gameDate)
	}
	actualMap := make(map[uint]float64, len(actualRows))
	for _, row := range actualRows {
		if _, exists := actualMap[row.NBAPlayerID]; exists {
			continue
		}
		actualMap[row.NBAPlayerID] = row.ActualPower
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

	// B. 以真实战力为目标值，重新求解真实最优 TopN
	actualCandidates := make([]PlayerCandidate, 0, len(gamePlayers))
	for _, p := range gamePlayers {
		actualCandidates = append(actualCandidates, PlayerCandidate{
			Player: p,
			Prediction: PlayerPrediction{
				PredictedPower: actualMap[p.NBAPlayerID],
			},
		})
	}
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

	s.printBacktestSummary(gameDate, recRows, optRows)
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

	for i, c := range lineup {
		totalActual += c.Prediction.PredictedPower
		totalSalary += c.Player.Salary
		if i < len(playerIDs) {
			playerIDs[i] = c.Player.NBAPlayerID
		}
	}

	detailData := map[string]any{
		"result_type": "actual_optimal",
	}
	if note != "" {
		detailData["note"] = note
	}
	if predictedTotal > 0 {
		detailData["predicted_total_power"] = roundTo(predictedTotal, 1)
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
	}
	fmt.Println()
}

// --- 文件读取 ---
