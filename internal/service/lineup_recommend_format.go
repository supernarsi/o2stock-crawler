// lineup_recommend_format.go 负责推荐和回测结果的格式化输出，包括：
// - 推荐阵容打印（printRecommendations）
// - 回测结果打印（printBacktestSummary）
// - 阵容排序（sortLineupsByPowerDesc）
// - 球员名称解析和格式化辅助函数
package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"o2stock-crawler/internal/entity"
)

// --- 推荐结果输出 ---

// printRecommendations 以表格形式打印推荐阵容列表。
func (s *LineupRecommendService) printRecommendations(gameDate string, title string, lineups [][]PlayerCandidate) {
	fmt.Printf("\n>>> %s — %s <<<\n\n", title, gameDate)

	// 排序已在保存前执行（sortLineupsByPowerDesc），此处无需再排

	medals := []string{"🏆", "🥈", "🥉"}
	for i, lineup := range lineups {
		var totalPower float64
		var totalSalary uint
		for _, c := range lineup {
			totalPower += c.Prediction.PredictedPower
			totalSalary += c.Player.Salary
		}

		medal := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			medal = medals[i]
		}
		fmt.Printf("%s %s #%d (总预测战力: %.1f, 总工资: %d)\n", medal, title, i+1, totalPower, totalSalary)
		fmt.Println("┌──────────────────────┬────────┬──────┬───────┬──────────┐")
		fmt.Println("│ 球员                 │ 球队   │ 工资 │  预测 │ 可用性   │")
		fmt.Println("├──────────────────────┼────────┼──────┼───────┼──────────┤")
		for _, c := range lineup {
			name := padRight(c.Player.PlayerName, 20)
			team := padRight(c.Player.TeamName, 6)
			fmt.Printf("│ %s │ %s │  %2d  │ %5.1f │  %.2f    │\n",
				name, team, c.Player.Salary, c.Prediction.PredictedPower, c.Prediction.AvailabilityScore)
		}
		fmt.Println("└──────────────────────┴────────┴──────┴───────┴──────────┘")
		fmt.Println()
	}
}

func recommendationTypeName(recommendationType uint8) string {
	switch recommendationType {
	case entity.LineupRecommendationTypeAvg3Baseline:
		return "avg3_recommendation"
	case entity.LineupRecommendationTypeAvg5Baseline:
		return "avg5_recommendation"
	default:
		return "ai_recommendation"
	}
}

// --- 回测结果输出 ---

func backtestResultTypeName(resultType uint8) string {
	switch resultType {
	case entity.LineupBacktestResultTypeRecommendedActual:
		return "recommended_actual"
	case entity.LineupBacktestResultTypeActualOptimal:
		return "actual_optimal"
	case entity.LineupBacktestResultTypeAvg3Benchmark:
		return "avg3_baseline"
	case entity.LineupBacktestResultTypeAvg5Benchmark:
		return "avg5_baseline"
	default:
		return "unknown"
	}
}

func backtestResultTypeLabel(resultType uint8) string {
	switch resultType {
	case entity.LineupBacktestResultTypeRecommendedActual:
		return "推荐实得"
	case entity.LineupBacktestResultTypeActualOptimal:
		return "真实最优"
	case entity.LineupBacktestResultTypeAvg3Benchmark:
		return "3日均值基准"
	case entity.LineupBacktestResultTypeAvg5Benchmark:
		return "5日均值基准"
	default:
		return fmt.Sprintf("结果类型%d", resultType)
	}
}

// printBacktestSummary 打印回测结果摘要，包括推荐实得、基准对比和真实最优。
func (s *LineupRecommendService) printBacktestSummary(
	gameDate string,
	recRows []entity.LineupBacktestResult,
	benchmarkRowsByType map[uint8][]entity.LineupBacktestResult,
	optRows []entity.LineupBacktestResult,
	playerMap map[uint]entity.NBAGamePlayer,
) {
	fmt.Printf("\n>>> 今日NBA回测结果 — %s <<<\n\n", gameDate)

	if len(recRows) == 0 {
		fmt.Println("无推荐回测结果")
		return
	}

	sort.Slice(recRows, func(i, j int) bool { return recRows[i].Rank < recRows[j].Rank })
	sort.Slice(optRows, func(i, j int) bool { return optRows[i].Rank < optRows[j].Rank })
	for resultType := range benchmarkRowsByType {
		rows := benchmarkRowsByType[resultType]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Rank < rows[j].Rank })
		benchmarkRowsByType[resultType] = rows
	}

	for i := 0; i < len(recRows); i++ {
		rec := recRows[i]
		fmt.Printf("#%d %s %.1f", i+1, backtestResultTypeLabel(rec.ResultType), rec.TotalActualPower)

		for _, benchmarkType := range []uint8{
			entity.LineupBacktestResultTypeAvg3Benchmark,
			entity.LineupBacktestResultTypeAvg5Benchmark,
		} {
			rows := benchmarkRowsByType[benchmarkType]
			if i >= len(rows) {
				continue
			}
			gap := roundTo(rec.TotalActualPower-rows[i].TotalActualPower, 1)
			fmt.Printf(" | %s %.1f (%+.1f)", backtestResultTypeLabel(benchmarkType), rows[i].TotalActualPower, gap)
		}

		if i < len(optRows) {
			opt := optRows[i]
			gap := roundTo(opt.TotalActualPower-rec.TotalActualPower, 1)
			fmt.Printf(" | %s %.1f (差距 %.1f)", backtestResultTypeLabel(opt.ResultType), opt.TotalActualPower, gap)
			fmt.Printf("\n   %s: %s", backtestResultTypeLabel(opt.ResultType), formatBacktestPlayersFromRow(opt, playerMap))
		}
		fmt.Println()
		for _, benchmarkType := range []uint8{
			entity.LineupBacktestResultTypeAvg3Benchmark,
			entity.LineupBacktestResultTypeAvg5Benchmark,
		} {
			rows := benchmarkRowsByType[benchmarkType]
			if i >= len(rows) {
				continue
			}
			fmt.Printf("   %s: %s\n", backtestResultTypeLabel(benchmarkType), formatBacktestPlayersFromRow(rows[i], playerMap))
		}
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

// sortLineupsByPowerDesc 按总预测战力降序、总工资升序排列阵容。
func sortLineupsByPowerDesc(lineups [][]PlayerCandidate) {
	sort.Slice(lineups, func(i, j int) bool {
		var powerI, powerJ float64
		var salaryI, salaryJ uint
		for _, c := range lineups[i] {
			powerI += c.Prediction.PredictedPower
			salaryI += c.Player.Salary
		}
		for _, c := range lineups[j] {
			powerJ += c.Prediction.PredictedPower
			salaryJ += c.Player.Salary
		}
		if powerI != powerJ {
			return powerI > powerJ
		}
		return salaryI < salaryJ
	})
}
