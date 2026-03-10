package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

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
	salaryTxMap := s.loadSalaryTxPlayerIDMap(ctx, allPlayers)
	txPlayerIDMap, txMapSummary := s.buildRecommendTxPlayerIDMap(ctx, allPlayers, salaryTxMap)
	log.Printf(
		"TX 球员映射: %d / %d (salary=%d, 手工=%d, lineup=%d)",
		len(txPlayerIDMap),
		len(allPlayers),
		txMapSummary.SalaryCount,
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
	pctx := PredictionContext{
		AllPlayers:     allPlayers,
		InjuryMap:      injuryMap,
		DBPlayerMap:    dbPlayerMap,
		TxPlayerIDMap:  txPlayerIDMap,
		GameStatsMap:   gameStatsMap,
		SeasonStatsMap: seasonStatsMap,
		TeamMatchupMap: teamMatchupMap,
		DVPFactorMap:   dvpFactorMap,
	}
	var candidates []PlayerCandidate
	effectiveCount := 0
	for i := range allPlayers {
		pred := s.predictPower(allPlayers[i], pctx)

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

	// 7. 按总预测战力降序、总工资升序排列后保存推荐结果
	lineupRepo := repositories.NewLineupRecommendationRepository(s.db.DB)
	var recs []entity.LineupRecommendation
	for _, set := range recommendationSets {
		sortLineupsByPowerDesc(set.lineups)
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
			Factors:        c.Prediction.toDetailFactors(),
		}
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

// printRecommendations 和 recommendationTypeName 已迁移到 lineup_recommend_format.go
// applyTeamExposurePenalty 和 estimateTeamPressureFactor 已迁移到 lineup_recommend_solver.go
