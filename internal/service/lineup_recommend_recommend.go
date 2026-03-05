package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

// GenerateRecommendation 生成指定日期的推荐阵容。
func (s *LineupRecommendService) GenerateRecommendation(ctx context.Context, gameDate string) error {
	log.Printf(">>> 开始生成推荐阵容 — %s <<<", gameDate)

	// 1. 查询候选球员
	gamePlayerRepo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	allPlayers, err := gamePlayerRepo.GetByGameDate(ctx, gameDate)
	if err != nil {
		return fmt.Errorf("查询候选球员失败: %w", err)
	}
	if len(allPlayers) == 0 {
		log.Printf("该日期无比赛数据: %s", gameDate)
		return nil
	}
	log.Printf("候选球员池: %d 人", len(allPlayers))

	// 2. 获取伤病报告
	injuryMap := s.fetchInjuryMap(ctx, allPlayers)
	log.Printf("伤病报告: 匹配到 %d 名球员", len(injuryMap))

	// 3. 获取 DB 球员数据（用于增强预测）
	dbPlayerMap := s.loadDBPlayerMap(ctx, allPlayers)
	log.Printf("DB 球员匹配: %d / %d", len(dbPlayerMap), len(allPlayers))

	// 4. 加载历史战绩数据
	statsRepo := repositories.NewStatsRepository(s.db.DB)
	gameStatsMap := s.loadGameStatsMap(ctx, statsRepo, dbPlayerMap)
	log.Printf("历史战绩数据: %d 名球员有记录", len(gameStatsMap))
	seasonStatsMap := s.loadSeasonStatsMap(ctx, statsRepo, dbPlayerMap, gameDate)
	log.Printf("赛季场均数据: %d 名球员有记录", len(seasonStatsMap))
	teamMatchupMap := s.loadTeamMatchupMetrics(ctx, statsRepo)
	log.Printf("对手 DefRating/Pace 数据: %d 支球队", len(teamMatchupMap))
	dvpFactorMap := s.buildDVPFactorMap(allPlayers, dbPlayerMap, gameStatsMap)
	log.Printf("对手 DvP 数据: %d 支球队", len(dvpFactorMap))

	// 5. 对每位球员预测战力
	var candidates []PlayerCandidate
	effectiveCount := 0
	for i := range allPlayers {
		pred := s.predictPower(
			allPlayers[i],
			allPlayers,
			injuryMap,
			dbPlayerMap,
			gameStatsMap,
			seasonStatsMap,
			teamMatchupMap,
			dvpFactorMap,
		)

		// 始终覆盖预测值，避免旧值残留
		writePower := pred.PredictedPower
		if writePower < 0 {
			writePower = 0
		}
		if err := gamePlayerRepo.UpdatePredictedPower(ctx, allPlayers[i].ID, writePower); err != nil {
			log.Printf("更新 predicted_power 失败: player_id=%d err=%v", allPlayers[i].NBAPlayerID, err)
		}

		if pred.PredictedPower > 0 {
			effectiveCount++
		}

		candidates = append(candidates, PlayerCandidate{
			Player:     allPlayers[i],
			Prediction: pred,
		})
	}
	log.Printf("有效球员: %d 人 (战力 > 0)", effectiveCount)
	candidates = applyTeamExposurePenalty(candidates)

	// 6. DP 求解最优阵容
	topLineups := s.solveOptimalLineup(candidates, defaultSalaryCap, defaultPickCount, defaultTopN)
	if len(topLineups) == 0 {
		log.Println("未找到可行阵容")
		return nil
	}

	// 7. 保存推荐结果
	lineupRepo := repositories.NewLineupRecommendationRepository(s.db.DB)
	var recs []entity.LineupRecommendation
	for rank, lineup := range topLineups {
		rec := s.buildRecommendation(gameDate, uint(rank+1), lineup, dbPlayerMap)
		recs = append(recs, rec)
	}

	if err := lineupRepo.BatchSave(ctx, recs); err != nil {
		return fmt.Errorf("保存推荐阵容失败: %w", err)
	}

	// 8. 输出推荐结果
	s.printRecommendations(gameDate, topLineups)

	log.Printf(">>> 推荐完成，结果已保存到 lineup_recommendation 表 <<<")
	return nil
}

// --- 球员战力预测（11 维评分） ---

// buildRecommendation 构造推荐阵容数据库记录。
func (s *LineupRecommendService) buildRecommendation(
	gameDate string,
	rank uint,
	lineup []PlayerCandidate,
	dbPlayerMap map[uint]*entity.Player,
) entity.LineupRecommendation {
	var totalPower float64
	var totalSalary uint
	var playerIDs [5]uint
	var detailPlayers []DetailPlayer

	for i, c := range lineup {
		totalPower += c.Prediction.PredictedPower
		totalSalary += c.Player.Salary
		if i < 5 {
			playerIDs[i] = c.Player.NBAPlayerID
		}

		dp := DetailPlayer{
			NBAPlayerID:    c.Player.NBAPlayerID,
			Name:           c.Player.PlayerName,
			Team:           c.Player.TeamName,
			Salary:         c.Player.Salary,
			CombatPower:    c.Player.CombatPower,
			PredictedPower: c.Prediction.PredictedPower,
			OptimizedPower: c.Prediction.OptimizedPower,
		}
		dp.Factors.BaseValue = c.Prediction.BaseValue
		dp.Factors.AvailabilityScore = c.Prediction.AvailabilityScore
		dp.Factors.StatusTrend = c.Prediction.StatusTrend
		dp.Factors.MatchupFactor = c.Prediction.MatchupFactor
		dp.Factors.DefRatingFactor = c.Prediction.DefRatingFactor
		dp.Factors.PaceFactor = c.Prediction.PaceFactor
		dp.Factors.DvPFactor = c.Prediction.DvPFactor
		dp.Factors.HistoryFactor = c.Prediction.HistoryFactor
		dp.Factors.OpponentFormFactor = c.Prediction.OpponentFormFactor
		dp.Factors.RimDeterrenceFactor = c.Prediction.RimDeterrenceFactor
		dp.Factors.DefenseAnchorFactor = c.Prediction.DefenseAnchorFactor
		dp.Factors.HomeAwayFactor = c.Prediction.HomeAwayFactor
		dp.Factors.TeamContextFactor = c.Prediction.TeamContextFactor
		dp.Factors.MinutesFactor = c.Prediction.MinutesFactor
		dp.Factors.UsageFactor = c.Prediction.UsageFactor
		dp.Factors.StabilityFactor = c.Prediction.StabilityFactor
		dp.Factors.DefenseUpsideFactor = c.Prediction.DefenseUpsideFactor
		dp.Factors.RoleSecurityFactor = c.Prediction.RoleSecurityFactor
		dp.Factors.DataReliabilityFactor = c.Prediction.DataReliabilityFactor
		dp.Factors.TeamExposureFactor = c.Prediction.TeamExposureFactor
		dp.Factors.FatigueFactor = c.Prediction.FatigueFactor
		dp.Factors.GameRiskFactor = c.Prediction.GameRiskFactor

		if dbP, ok := dbPlayerMap[c.Player.NBAPlayerID]; ok {
			dp.Factors.DbPowerPer5 = dbP.PowerPer5
			dp.Factors.DbPowerPer10 = dbP.PowerPer10
		}

		detailPlayers = append(detailPlayers, dp)
	}

	detail := DetailJSON{Players: detailPlayers}
	detailBytes, _ := json.Marshal(detail)

	return entity.LineupRecommendation{
		GameDate:            gameDate,
		Rank:                rank,
		TotalPredictedPower: roundTo(totalPower, 1),
		TotalSalary:         totalSalary,
		Player1ID:           playerIDs[0],
		Player2ID:           playerIDs[1],
		Player3ID:           playerIDs[2],
		Player4ID:           playerIDs[3],
		Player5ID:           playerIDs[4],
		DetailJSON:          string(detailBytes),
	}
}

func (s *LineupRecommendService) printRecommendations(gameDate string, lineups [][]PlayerCandidate) {
	fmt.Printf("\n>>> 今日NBA推荐阵容 — %s <<<\n\n", gameDate)

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
		fmt.Printf("%s 推荐阵容 #%d (总预测战力: %.1f, 总工资: %d)\n", medal, i+1, totalPower, totalSalary)
		fmt.Println("┌──────────────────────┬──────┬──────┬───────┬──────────┐")
		fmt.Println("│ 球员                 │ 球队 │ 工资 │ 预测  │ 可用性   │")
		fmt.Println("├──────────────────────┼──────┼──────┼───────┼──────────┤")
		for _, c := range lineup {
			name := padRight(c.Player.PlayerName, 20)
			team := padRight(c.Player.TeamName, 4)
			fmt.Printf("│ %s │ %s │  %2d  │ %5.1f │  %.2f    │\n",
				name, team, c.Player.Salary, c.Prediction.PredictedPower, c.Prediction.AvailabilityScore)
		}
		fmt.Println("└──────────────────────┴──────┴──────┴───────┴──────────┘")
		fmt.Println()
	}
}

// applyTeamExposurePenalty 对同队第 3 名及之后的候选球员施加惩罚，避免推荐阵容过度堆叠单队风险。
func applyTeamExposurePenalty(candidates []PlayerCandidate) []PlayerCandidate {
	if len(candidates) == 0 {
		return candidates
	}

	teamToIndexes := make(map[string][]int)
	for idx := range candidates {
		teamCode := normalizeTeamCode(candidates[idx].Player.TeamName)
		if teamCode == "" {
			teamCode = candidates[idx].Player.NBATeamID
		}
		teamToIndexes[teamCode] = append(teamToIndexes[teamCode], idx)
		candidates[idx].Prediction.TeamExposureFactor = 1.0
	}

	for _, indexes := range teamToIndexes {
		sort.Slice(indexes, func(i, j int) bool {
			left := candidates[indexes[i]].Prediction.OptimizedPower
			if left <= 0 {
				left = candidates[indexes[i]].Prediction.PredictedPower
			}
			right := candidates[indexes[j]].Prediction.OptimizedPower
			if right <= 0 {
				right = candidates[indexes[j]].Prediction.PredictedPower
			}
			if left == right {
				return candidates[indexes[i]].Player.Salary < candidates[indexes[j]].Player.Salary
			}
			return left > right
		})

		secondPower := 0.0
		if len(indexes) >= 2 {
			secondPower = candidates[indexes[1]].Prediction.OptimizedPower
			if secondPower <= 0 {
				secondPower = candidates[indexes[1]].Prediction.PredictedPower
			}
		}
		teamPressureFactor := estimateTeamPressureFactor(candidates, indexes)
		extraSecondPenalty := 1.0
		if teamPressureFactor < 0.88 {
			extraSecondPenalty = 0.90
		} else if teamPressureFactor < 0.92 {
			extraSecondPenalty = 0.94
		}

		for rank, idx := range indexes {
			penalty := 1.0
			switch {
			case rank <= 1:
				if rank == 1 {
					penalty = extraSecondPenalty
				}
			case rank == 2:
				current := candidates[idx].Prediction.OptimizedPower
				if current <= 0 {
					current = candidates[idx].Prediction.PredictedPower
				}
				if secondPower > 0 && current/secondPower < 0.75 {
					penalty = 0.95 * extraSecondPenalty
				} else {
					penalty = 0.98 * extraSecondPenalty
				}
			case rank == 3:
				penalty = 0.90 * extraSecondPenalty
			default:
				penalty = 0.84 * extraSecondPenalty
			}

			base := candidates[idx].Prediction.OptimizedPower
			if base <= 0 {
				base = candidates[idx].Prediction.PredictedPower
			}
			candidates[idx].Prediction.TeamExposureFactor = penalty
			candidates[idx].Prediction.OptimizedPower = base * penalty
		}
	}

	return candidates
}

func estimateTeamPressureFactor(candidates []PlayerCandidate, indexes []int) float64 {
	if len(indexes) == 0 {
		return 1.0
	}

	limit := min(2, len(indexes))
	total := 0.0
	count := 0
	for i := 0; i < limit; i++ {
		pred := candidates[indexes[i]].Prediction
		matchup := pred.MatchupFactor
		if matchup <= 0 {
			matchup = 1.0
		}
		anchor := pred.DefenseAnchorFactor
		if anchor <= 0 {
			anchor = 1.0
		}
		rim := pred.RimDeterrenceFactor
		if rim <= 0 {
			rim = 1.0
		}
		form := pred.OpponentFormFactor
		if form <= 0 {
			form = 1.0
		}

		total += matchup * anchor * rim * form
		count++
	}
	if count == 0 {
		return 1.0
	}
	return clamp(total/float64(count), 0.75, 1.05)
}
