package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

// GenerateRecommendation 生成指定日期的推荐阵容。
func (s *LineupRecommendService) GenerateRecommendation(ctx context.Context, gameDate string) error {
	log.Printf(">>> 开始生成推荐阵容 — %s <<<", gameDate)

	if err := s.ensureGamePlayersForDate(ctx, gameDate); err != nil {
		return err
	}

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
	historicalMode := isHistoricalGameDate(gameDate)
	if historicalMode {
		log.Printf("历史日期推荐模式: %s，将禁用实时伤病并仅使用赛前历史数据", gameDate)
	}

	// 2. 获取伤病报告
	injuryMap := map[uint]crawler.InjuryReport{}
	if !historicalMode {
		var (
			snapshotRows []entity.NBAGameInjurySnapshot
			err          error
		)
		injuryMap, snapshotRows, err = s.fetchInjuryMap(ctx, allPlayers)
		if err != nil {
			log.Printf("获取伤病报告失败（将跳过伤病因素）: %v", err)
		} else {
			if err := s.persistInjurySnapshots(ctx, gameDate, snapshotRows); err != nil {
				log.Printf("保存伤病快照失败: %v", err)
			} else {
				log.Printf("伤病快照已保存: %d 条 (日期=%s)", len(snapshotRows), gameDate)
			}
		}
	} else {
		if snapshotMap, ok := s.loadInjurySnapshotMap(ctx, gameDate); ok {
			injuryMap = snapshotMap
			log.Printf("历史伤病快照: 命中 %d 名球员", len(injuryMap))
		} else {
			log.Printf("历史伤病快照缺失: %s，将继续跳过伤病因素", gameDate)
		}
	}
	log.Printf("伤病报告: 匹配到 %d 名球员", len(injuryMap))

	// 3. 获取 DB 球员数据（用于增强预测）
	rawDBPlayerMap := s.loadDBPlayerMap(ctx, allPlayers)
	dbPlayerMap := rawDBPlayerMap
	if historicalMode {
		dbPlayerMap = stripPredictAnchors(rawDBPlayerMap)
	}
	log.Printf("DB 球员匹配: %d / %d", len(dbPlayerMap), len(allPlayers))
	txPlayerIDMap, txMapSummary := s.buildRecommendTxPlayerIDMap(ctx, allPlayers, rawDBPlayerMap)
	log.Printf(
		"TX 球员映射: %d / %d (DB=%d, 手工=%d, lineup=%d)",
		len(txPlayerIDMap),
		len(allPlayers),
		txMapSummary.DBCount,
		txMapSummary.ManualCount,
		txMapSummary.LineupFallbackCount,
	)

	// 4. 加载历史战绩数据
	statsRepo := repositories.NewStatsRepository(s.db.DB)
	gameStatsMap := s.loadGameStatsMap(ctx, statsRepo, txPlayerIDMap, gameDate, historicalMode)
	log.Printf("历史战绩数据: %d 名球员有记录", len(gameStatsMap))
	seasonStatsMap := s.loadSeasonStatsMap(ctx, statsRepo, txPlayerIDMap, gameDate, historicalMode)
	log.Printf("赛季场均数据: %d 名球员有记录", len(seasonStatsMap))
	teamMatchupMap := s.loadTeamMatchupMetrics(ctx, statsRepo, gameDate, historicalMode)
	log.Printf("对手 DefRating/Pace 数据: %d 支球队", len(teamMatchupMap))
	dvpFactorMap := s.buildDVPFactorMap(allPlayers, txPlayerIDMap, gameStatsMap)
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
			txPlayerIDMap,
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

	type recommendationSet struct {
		recType  uint8
		title    string
		lookback int
		lineups  [][]PlayerCandidate
	}

	recommendationSets := []recommendationSet{
		{
			recType:  entity.LineupRecommendationTypeAIRecommended,
			title:    "AI推荐阵容",
			lookback: 0,
			lineups:  topLineups,
		},
	}

	for _, spec := range []struct {
		recType  uint8
		title    string
		lookback int
	}{
		{recType: entity.LineupRecommendationTypeAvg3Baseline, title: "3日均值推荐", lookback: 3},
		{recType: entity.LineupRecommendationTypeAvg5Baseline, title: "5日均值推荐", lookback: 5},
	} {
		benchmarkCandidates, summary, err := s.buildAverageRecommendationCandidates(
			ctx,
			gameDate,
			allPlayers,
			spec.lookback,
			injuryMap,
		)
		if err != nil {
			log.Printf("构建%s候选失败: %v", spec.title, err)
			continue
		}
		log.Printf(
			"%s候选: lookback=%d, 候选=%d, NBA映射=%d(手工兜底=%d), 历史命中=%d, 历史不足=%d, 伤病排除=%d",
			spec.title,
			summary.LookbackGames,
			summary.CandidateCount,
			summary.MappedNBACount,
			summary.ManualMapApplied,
			summary.HistoryHitCount,
			summary.InsufficientHistory,
			summary.InjuryFilteredCount,
		)

		lineups := s.solveOptimalLineup(benchmarkCandidates, defaultSalaryCap, defaultPickCount, defaultTopN)
		if len(lineups) == 0 {
			log.Printf("%s未找到可行阵容: %s", spec.title, gameDate)
			continue
		}
		recommendationSets = append(recommendationSets, recommendationSet{
			recType:  spec.recType,
			title:    spec.title,
			lookback: spec.lookback,
			lineups:  lineups,
		})
	}

	// 7. 保存推荐结果
	lineupRepo := repositories.NewLineupRecommendationRepository(s.db.DB)
	var recs []entity.LineupRecommendation
	for _, set := range recommendationSets {
		for rank, lineup := range set.lineups {
			rec := s.buildRecommendation(gameDate, uint(rank+1), set.recType, set.lookback, lineup, dbPlayerMap)
			recs = append(recs, rec)
		}
	}

	if err := lineupRepo.BatchSave(ctx, recs); err != nil {
		return fmt.Errorf("保存推荐阵容失败: %w", err)
	}

	// 8. 输出推荐结果
	for _, set := range recommendationSets {
		s.printRecommendations(gameDate, set.title, set.lineups)
	}

	log.Printf(">>> 推荐完成，结果已保存到 lineup_recommendation 表 <<<")
	return nil
}

// --- 球员战力预测（11 维评分） ---

// buildRecommendation 构造推荐阵容数据库记录。
func (s *LineupRecommendService) buildRecommendation(
	gameDate string,
	rank uint,
	recommendationType uint8,
	lookbackGames int,
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
		dp.Factors.ArchetypeFactor = c.Prediction.ArchetypeFactor
		dp.Factors.RoleSecurityFactor = c.Prediction.RoleSecurityFactor
		dp.Factors.DataReliabilityFactor = c.Prediction.DataReliabilityFactor
		dp.Factors.TeamExposureFactor = c.Prediction.TeamExposureFactor
		dp.Factors.FatigueFactor = c.Prediction.FatigueFactor
		dp.Factors.GameRiskFactor = c.Prediction.GameRiskFactor
		dp.Factors.Upside3 = c.Prediction.Upside3
		dp.Factors.Upside5 = c.Prediction.Upside5
		dp.Factors.VersatilityFactor = c.Prediction.VersatilityFactor
		dp.Factors.ExplosivenessFactor = c.Prediction.ExplosivenessFactor
		dp.Factors.StableFloorFactor = c.Prediction.StableFloorFactor

		if dbP, ok := dbPlayerMap[c.Player.NBAPlayerID]; ok {
			dp.Factors.DbPowerPer5 = dbP.PowerPer5
			dp.Factors.DbPowerPer10 = dbP.PowerPer10
		}

		detailPlayers = append(detailPlayers, dp)
	}

	detail := DetailJSON{
		RecommendationType: recommendationTypeName(recommendationType),
		LookbackGames:      lookbackGames,
		Players:            detailPlayers,
	}
	detailBytes, _ := json.Marshal(detail)

	return entity.LineupRecommendation{
		GameDate:            gameDate,
		RecommendationType:  recommendationType,
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

func (s *LineupRecommendService) printRecommendations(gameDate string, title string, lineups [][]PlayerCandidate) {
	fmt.Printf("\n>>> %s — %s <<<\n\n", title, gameDate)

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
			extraSecondPenalty = 0.92
		} else if teamPressureFactor < 0.92 {
			extraSecondPenalty = 0.96
		}

		// 识别低薪高能球员，减少惩罚
		// 使用动态阈值：工资≤15 且预测战力/工资比值≥3.0 视为价值球员
		highValueCount := 0
		for _, idx := range indexes {
			c := candidates[idx]
			valueRatio := 0.0
			if c.Player.Salary > 0 {
				valueRatio = c.Prediction.PredictedPower / float64(c.Player.Salary)
			}
			// 条件 1：工资≤12 且战力≥35（传统低薪高能）
			// 条件 2：工资≤15 且战力/工资比值≥3.5（动态价值比）
			// 条件 3：工资≤20 且战力/工资比值≥4.0（超级价值比）
			isHighValue := (c.Player.Salary <= 12 && c.Prediction.PredictedPower >= 35) ||
				(c.Player.Salary <= 15 && valueRatio >= 3.5) ||
				(c.Player.Salary <= 20 && valueRatio >= 4.0)
			if isHighValue {
				highValueCount++
			}
		}

		for rank, idx := range indexes {
			c := candidates[idx]
			valueRatio := 0.0
			if c.Player.Salary > 0 {
				valueRatio = c.Prediction.PredictedPower / float64(c.Player.Salary)
			}
			// 低薪高能球员减少惩罚（使用动态阈值）
			isHighValue := (c.Player.Salary <= 12 && c.Prediction.PredictedPower >= 35) ||
				(c.Player.Salary <= 15 && valueRatio >= 3.5) ||
				(c.Player.Salary <= 20 && valueRatio >= 4.0)
			// 爆发型球员识别：近期有单场爆发表现（Upside3≥1.4）
			hasUpside := c.Prediction.Upside3 >= 1.4
			isExplosive := isHighValue && hasUpside

			penalty := 1.0
			switch {
			case rank <= 1:
				if rank == 1 {
					// 前两名球员施加轻微惩罚，但爆发型球员保持原价
					if isExplosive {
						penalty = 1.0
					} else {
						penalty = extraSecondPenalty
					}
				}
			case rank == 2:
				current := candidates[idx].Prediction.OptimizedPower
				if current <= 0 {
					current = candidates[idx].Prediction.PredictedPower
				}
				if secondPower > 0 && current/secondPower < 0.75 {
					if isExplosive {
						penalty = 0.98 * extraSecondPenalty // 爆发型球员惩罚更少
					} else if isHighValue {
						penalty = 0.97 * extraSecondPenalty
					} else {
						penalty = 0.96 * extraSecondPenalty
					}
				} else {
					if isExplosive {
						penalty = 1.0 // 爆发型球员不惩罚
					} else if isHighValue {
						penalty = 0.99 * extraSecondPenalty
					} else {
						penalty = 0.98 * extraSecondPenalty
					}
				}
			case rank == 3:
				if isExplosive {
					penalty = 0.96 * extraSecondPenalty
				} else if isHighValue {
					penalty = 0.94 * extraSecondPenalty
				} else {
					penalty = 0.91 * extraSecondPenalty
				}
			default:
				if isExplosive {
					penalty = 0.92 * extraSecondPenalty
				} else if isHighValue {
					penalty = 0.88 * extraSecondPenalty
				} else {
					penalty = 0.85 * extraSecondPenalty
				}
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
