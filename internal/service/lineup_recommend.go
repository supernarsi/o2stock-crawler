package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LineupRecommendService 推荐引擎核心服务
type LineupRecommendService struct {
	db           *db.DB
	injuryClient *crawler.InjuryClient
}

const (
	defaultSalaryCap = 150
	defaultPickCount = 5
	defaultTopN      = 3
)

// NewLineupRecommendService 创建推荐引擎服务
func NewLineupRecommendService(database *db.DB) *LineupRecommendService {
	return &LineupRecommendService{
		db:           database,
		injuryClient: crawler.NewInjuryClient(),
	}
}

// --- JSON 数据结构 ---

// MatchData 比赛数据 JSON 结构
type MatchData struct {
	IMatchId    string `json:"iMatchId"`
	IHomeTeamId string `json:"iHomeTeamId"`
	IAwayTeamId string `json:"iAwayTeamId"`
	DtDate      string `json:"dtDate"`
	DtTime      string `json:"dtTime"`
}

// PlayerSalary 球员工资 JSON 结构
type PlayerSalary struct {
	ID            string `json:"id"`
	IPlayerId     string `json:"iPlayerId"`
	ITeamId       string `json:"iTeamId"`
	SPlayerName   string `json:"sPlayerName"`
	SPlayerEnName string `json:"sPlayerEnName"`
	IPosition     string `json:"iPosition"`
	FCombatPower  string `json:"fCombatPower"`
	ISalary       string `json:"iSalary"`
}

// ActualFeedbackItem 真实战力反馈项（仅支持阵容 list 格式）
type ActualFeedbackItem struct {
	Rank        uint     `json:"-"`
	NBAPlayerID uint     `json:"nba_player_id"`
	Salary      *uint    `json:"salary"`
	ActualPower *float64 `json:"actual_power"`
	Source      string   `json:"source"`
}

// ActualFeedbackLineupPayload 阵容反馈 JSON 结构（list -> players）
type ActualFeedbackLineupPayload struct {
	GameDate string                 `json:"game_date"`
	Source   string                 `json:"source"`
	List     []ActualFeedbackLineup `json:"list"`
}

type ActualFeedbackLineup struct {
	Rank    uint                 `json:"rank"`
	Players []ActualFeedbackItem `json:"players"`
}

// PlayerPrediction 预测结果及各因子明细
type PlayerPrediction struct {
	PredictedPower    float64
	BaseValue         float64
	AvailabilityScore float64
	StatusTrend       float64
	MatchupFactor     float64
	HomeAwayFactor    float64
	TeamContextFactor float64
	MinutesFactor     float64
	UsageFactor       float64
	StabilityFactor   float64
	FatigueFactor     float64
	GameRiskFactor    float64
}

// PlayerCandidate 候选球员（含预测值）
type PlayerCandidate struct {
	Player     entity.NBAGamePlayer
	Prediction PlayerPrediction
}

// DetailPlayer detail_json 中的球员信息
type DetailPlayer struct {
	NBAPlayerID    uint    `json:"nba_player_id"`
	Name           string  `json:"name"`
	Team           string  `json:"team"`
	Salary         uint    `json:"salary"`
	CombatPower    float64 `json:"combat_power"`
	PredictedPower float64 `json:"predicted_power"`
	Factors        struct {
		BaseValue         float64 `json:"base_value"`
		AvailabilityScore float64 `json:"availability_score"`
		StatusTrend       float64 `json:"status_trend"`
		MatchupFactor     float64 `json:"matchup_factor"`
		HomeAwayFactor    float64 `json:"home_away_factor"`
		TeamContextFactor float64 `json:"team_context_factor"`
		MinutesFactor     float64 `json:"minutes_factor"`
		UsageFactor       float64 `json:"usage_factor"`
		StabilityFactor   float64 `json:"stability_factor"`
		FatigueFactor     float64 `json:"fatigue_factor"`
		GameRiskFactor    float64 `json:"game_risk_factor"`
		DbPowerPer5       float64 `json:"db_power_per5,omitempty"`
		DbPowerPer10      float64 `json:"db_power_per10,omitempty"`
	} `json:"factors"`
}

// DetailJSON detail_json 结构
type DetailJSON struct {
	Players []DetailPlayer `json:"players"`
}

// --- 数据导入 ---

// ImportGameData 导入游戏数据 JSON 到数据库
func (s *LineupRecommendService) ImportGameData(ctx context.Context, dataDir string) error {
	// 1. 读取 team_id.json → 构建 teamId → teamName 映射
	teamIDMap, err := s.loadTeamIDMap(dataDir + "/team_id.json")
	if err != nil {
		return fmt.Errorf("读取 team_id.json 失败: %w", err)
	}
	log.Printf("加载球队映射: %d 支球队", len(teamIDMap))

	// 2. 读取 match_data.json → 获取比赛列表
	matches, err := s.loadMatchData(dataDir + "/match_data.json")
	if err != nil {
		return fmt.Errorf("读取 match_data.json 失败: %w", err)
	}
	log.Printf("加载比赛数据: %d 场比赛", len(matches))

	// 3. 读取 player_salary.json → 获取球员列表
	playerSalaries, err := s.loadPlayerSalary(dataDir + "/player_salary.json")
	if err != nil {
		return fmt.Errorf("读取 player_salary.json 失败: %w", err)
	}
	log.Printf("加载球员工资: %d 名球员", len(playerSalaries))

	// 4. 构建 teamId → matchId 映射 和 match 信息映射
	teamToMatch := make(map[string]*MatchData)
	gameDate := ""
	for i := range matches {
		match := &matches[i]
		teamToMatch[match.IHomeTeamId] = match
		teamToMatch[match.IAwayTeamId] = match

		if gameDate == "" {
			gameDate = match.DtDate
			continue
		}
		if match.DtDate != gameDate {
			return fmt.Errorf("match_data.json 包含多个比赛日期: %s / %s", gameDate, match.DtDate)
		}
	}
	if gameDate == "" {
		return fmt.Errorf("无法确定比赛日期")
	}
	log.Printf("比赛日期: %s", gameDate)

	// 5. 遍历球员列表，构建 NBAGamePlayer 对象
	playerMap := make(map[uint]entity.NBAGamePlayer)
	invalidCount := 0

	for _, ps := range playerSalaries {
		match, ok := teamToMatch[ps.ITeamId]
		if !ok {
			log.Printf("警告: 球员 %s (team %s) 未找到对应比赛", ps.SPlayerName, ps.ITeamId)
			continue
		}

		nbaPlayerID, err := strconv.ParseUint(strings.TrimSpace(ps.IPlayerId), 10, 32)
		if err != nil || nbaPlayerID == 0 {
			invalidCount++
			log.Printf("警告: 跳过非法 iPlayerId=%q (name=%s)", ps.IPlayerId, ps.SPlayerName)
			continue
		}

		salary, err := strconv.ParseUint(strings.TrimSpace(ps.ISalary), 10, 32)
		if err != nil {
			invalidCount++
			log.Printf("警告: 跳过非法 iSalary=%q (name=%s)", ps.ISalary, ps.SPlayerName)
			continue
		}

		combatPower, err := strconv.ParseFloat(strings.TrimSpace(ps.FCombatPower), 64)
		if err != nil {
			invalidCount++
			log.Printf("警告: 跳过非法 fCombatPower=%q (name=%s)", ps.FCombatPower, ps.SPlayerName)
			continue
		}

		position, err := strconv.ParseUint(strings.TrimSpace(ps.IPosition), 10, 8)
		if err != nil {
			invalidCount++
			log.Printf("警告: 跳过非法 iPosition=%q (name=%s)", ps.IPosition, ps.SPlayerName)
			continue
		}

		isHome := ps.ITeamId == match.IHomeTeamId
		teamName := teamIDMap[ps.ITeamId]
		if teamName == "" {
			teamName = ps.ITeamId
		}

		playerMap[uint(nbaPlayerID)] = entity.NBAGamePlayer{
			GameDate:     gameDate,
			MatchID:      match.IMatchId,
			NBAPlayerID:  uint(nbaPlayerID),
			NBATeamID:    ps.ITeamId,
			PlayerName:   ps.SPlayerName,
			PlayerEnName: ps.SPlayerEnName,
			TeamName:     teamName,
			IsHome:       isHome,
			Salary:       uint(salary),
			CombatPower:  combatPower,
			Position:     uint(position),
		}
	}

	players := make([]entity.NBAGamePlayer, 0, len(playerMap))
	for _, p := range playerMap {
		players = append(players, p)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].NBAPlayerID < players[j].NBAPlayerID
	})
	if len(players) == 0 {
		return fmt.Errorf("没有可导入的候选球员")
	}

	// 6. 按日期全量替换，避免旧候选残留
	repo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	if err := repo.ReplaceByGameDate(ctx, gameDate, players); err != nil {
		return fmt.Errorf("导入球员数据失败: %w", err)
	}

	log.Printf("成功导入 %d 名球员到 nba_game_player 表 (日期: %s, 非法记录: %d)", len(players), gameDate, invalidCount)
	return nil
}

// ImportActualFeedback 导入赛后真实战力反馈（按比赛日全量替换）
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

	// 5. 对每位球员预测战力
	var candidates []PlayerCandidate
	effectiveCount := 0
	for i := range allPlayers {
		pred := s.predictPower(allPlayers[i], allPlayers, injuryMap, dbPlayerMap, gameStatsMap, seasonStatsMap)

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

func (s *LineupRecommendService) predictPower(
	player entity.NBAGamePlayer,
	allPlayers []entity.NBAGamePlayer,
	injuryMap map[uint]crawler.InjuryReport,
	dbPlayerMap map[uint]*entity.Player,
	gameStatsMap map[uint][]entity.PlayerGameStats,
	seasonStatsMap map[uint]*entity.PlayerSeasonStats,
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
	txPlayerID := uint(0)
	if dbPlayer != nil {
		txPlayerID = dbPlayer.TxPlayerID
	}
	if txPlayerID > 0 {
		stats := gameStatsMap[txPlayerID]
		if len(stats) >= 3 {
			// 计算该球员历史对阵情况
			opponentTeam := s.getOpponentTeamCode(player, allPlayers)
			matchupFactor = s.calcMatchupFactor(stats, opponentTeam, baseValue)
		}
	}

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

func (s *LineupRecommendService) solveOptimalLineup(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
) [][]PlayerCandidate {
	return s.solveOptimalLineupInternal(candidates, salaryCap, pickCount, topN, false)
}

func (s *LineupRecommendService) solveOptimalLineupAllowZero(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
) [][]PlayerCandidate {
	return s.solveOptimalLineupInternal(candidates, salaryCap, pickCount, topN, true)
}

func (s *LineupRecommendService) solveOptimalLineupInternal(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
	allowNonPositive bool,
) [][]PlayerCandidate {
	if salaryCap <= 0 || pickCount <= 0 || topN <= 0 {
		return nil
	}

	// 过滤：推荐场景只保留 >0，回测场景允许 0/负分
	var allValid []PlayerCandidate
	for _, c := range candidates {
		if c.Player.Salary == 0 {
			continue
		}
		if allowNonPositive || c.Prediction.PredictedPower > 0 {
			allValid = append(allValid, c)
		}
	}
	if len(allValid) < pickCount {
		return nil
	}

	valid := allValid

	log.Printf("DP 求解: 候选球员 %d 人, 工资帽 %d, 选 %d 人, 输出 Top %d", len(valid), salaryCap, pickCount, topN)

	// dp[j][k] = 选 j 人，工资恰好为 k 时的 TopN 阵容
	dp := make([][][]lineupState, pickCount+1)
	for j := 0; j <= pickCount; j++ {
		dp[j] = make([][]lineupState, salaryCap+1)
	}
	dp[0][0] = []lineupState{{score: 0, salary: 0, indices: []int{}}}

	for i, c := range valid {
		salary := int(c.Player.Salary)
		power := c.Prediction.PredictedPower
		if salary > salaryCap {
			continue
		}

		for j := pickCount; j >= 1; j-- {
			for k := salaryCap; k >= salary; k-- {
				prevStates := dp[j-1][k-salary]
				if len(prevStates) == 0 {
					continue
				}

				nextStates := dp[j][k]
				for _, prev := range prevStates {
					nextIdx := append([]int{}, prev.indices...)
					nextIdx = append(nextIdx, i)
					nextStates = insertLineupState(nextStates, lineupState{
						score:   prev.score + power,
						salary:  k,
						indices: nextIdx,
					}, topN)
				}
				dp[j][k] = nextStates
			}
		}
	}

	bestStates := make([]lineupState, 0, topN)
	for k := 0; k <= salaryCap; k++ {
		for _, st := range dp[pickCount][k] {
			bestStates = insertLineupState(bestStates, st, topN)
		}
	}
	if len(bestStates) == 0 {
		return nil
	}

	results := make([][]PlayerCandidate, 0, len(bestStates))
	for _, st := range bestStates {
		lineup := make([]PlayerCandidate, 0, len(st.indices))
		for _, idx := range st.indices {
			lineup = append(lineup, valid[idx])
		}
		sort.Slice(lineup, func(i, j int) bool {
			if lineup[i].Prediction.PredictedPower == lineup[j].Prediction.PredictedPower {
				if lineup[i].Player.Salary == lineup[j].Player.Salary {
					return lineup[i].Player.NBAPlayerID < lineup[j].Player.NBAPlayerID
				}
				return lineup[i].Player.Salary < lineup[j].Player.Salary
			}
			return lineup[i].Prediction.PredictedPower > lineup[j].Prediction.PredictedPower
		})
		results = append(results, lineup)
	}
	return results
}

type lineupState struct {
	score   float64
	salary  int
	indices []int
}

func insertLineupState(states []lineupState, candidate lineupState, limit int) []lineupState {
	for i := range states {
		if sameLineupIndices(states[i].indices, candidate.indices) {
			if lineupStateLess(candidate, states[i]) {
				states[i] = candidate
			}
			sort.Slice(states, func(a, b int) bool {
				return lineupStateLess(states[a], states[b])
			})
			if len(states) > limit {
				states = states[:limit]
			}
			return states
		}
	}

	states = append(states, candidate)
	sort.Slice(states, func(i, j int) bool {
		return lineupStateLess(states[i], states[j])
	})
	if len(states) > limit {
		states = states[:limit]
	}
	return states
}

func lineupStateLess(a, b lineupState) bool {
	if math.Abs(a.score-b.score) > 1e-9 {
		return a.score > b.score
	}
	if a.salary != b.salary {
		return a.salary < b.salary
	}
	return lexicographicallyLess(a.indices, b.indices)
}

func sameLineupIndices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func lexicographicallyLess(a, b []int) bool {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] == b[i] {
			continue
		}
		return a[i] < b[i]
	}
	return len(a) < len(b)
}

// --- 辅助函数 ---

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

func (s *LineupRecommendService) getOpponentTeamCode(player entity.NBAGamePlayer, allPlayers []entity.NBAGamePlayer) string {
	for _, p := range allPlayers {
		if p.MatchID == player.MatchID && p.NBATeamID != player.NBATeamID {
			return normalizeTeamCode(p.TeamName)
		}
	}
	return ""
}

func (s *LineupRecommendService) calcMatchupFactor(stats []entity.PlayerGameStats, opponentTeam string, baseValue float64) float64 {
	if len(stats) == 0 || baseValue <= 0 {
		return 1.0
	}

	// 计算对手场均失分（使用近期数据粗略估算）
	// 这里简化为：如果有该球员对阵该对手的历史数据，计算平均战力
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
		}
		dp.Factors.BaseValue = c.Prediction.BaseValue
		dp.Factors.AvailabilityScore = c.Prediction.AvailabilityScore
		dp.Factors.StatusTrend = c.Prediction.StatusTrend
		dp.Factors.MatchupFactor = c.Prediction.MatchupFactor
		dp.Factors.HomeAwayFactor = c.Prediction.HomeAwayFactor
		dp.Factors.TeamContextFactor = c.Prediction.TeamContextFactor
		dp.Factors.MinutesFactor = c.Prediction.MinutesFactor
		dp.Factors.UsageFactor = c.Prediction.UsageFactor
		dp.Factors.StabilityFactor = c.Prediction.StabilityFactor
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

func (s *LineupRecommendService) loadTeamIDMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// 反转：teamName → teamId 变成 teamId → teamName
	result := make(map[string]string)
	for name, id := range raw {
		result[id] = name
	}
	return result, nil
}

func (s *LineupRecommendService) loadMatchData(path string) ([]MatchData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var matches []MatchData
	if err := json.Unmarshal(data, &matches); err != nil {
		return nil, err
	}
	return matches, nil
}

func (s *LineupRecommendService) loadPlayerSalary(path string) ([]PlayerSalary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var players []PlayerSalary
	if err := json.Unmarshal(data, &players); err != nil {
		return nil, err
	}
	return players, nil
}

// --- 通用工具函数 ---

type teamAlias struct {
	alias string
	code  string
}

var englishTeamAliases = []teamAlias{
	{alias: "los angeles lakers", code: "LAL"},
	{alias: "los angeles clippers", code: "LAC"},
	{alias: "oklahoma city thunder", code: "OKC"},
	{alias: "new orleans pelicans", code: "NOP"},
	{alias: "san antonio spurs", code: "SAS"},
	{alias: "golden state warriors", code: "GSW"},
	{alias: "minnesota timberwolves", code: "MIN"},
	{alias: "portland trail blazers", code: "POR"},
	{alias: "philadelphia 76ers", code: "PHI"},
	{alias: "indiana pacers", code: "IND"},
	{alias: "washington wizards", code: "WAS"},
	{alias: "orlando magic", code: "ORL"},
	{alias: "new york knicks", code: "NYK"},
	{alias: "brooklyn nets", code: "BKN"},
	{alias: "charlotte hornets", code: "CHA"},
	{alias: "cleveland cavaliers", code: "CLE"},
	{alias: "dallas mavericks", code: "DAL"},
	{alias: "denver nuggets", code: "DEN"},
	{alias: "detroit pistons", code: "DET"},
	{alias: "houston rockets", code: "HOU"},
	{alias: "memphis grizzlies", code: "MEM"},
	{alias: "miami heat", code: "MIA"},
	{alias: "milwaukee bucks", code: "MIL"},
	{alias: "phoenix suns", code: "PHX"},
	{alias: "sacramento kings", code: "SAC"},
	{alias: "toronto raptors", code: "TOR"},
	{alias: "utah jazz", code: "UTA"},
	{alias: "atlanta hawks", code: "ATL"},
	{alias: "boston celtics", code: "BOS"},
	{alias: "chicago bulls", code: "CHI"},
	{alias: "trail blazers", code: "POR"},
	{alias: "thunder", code: "OKC"},
	{alias: "lakers", code: "LAL"},
	{alias: "clippers", code: "LAC"},
	{alias: "hawks", code: "ATL"},
	{alias: "nets", code: "BKN"},
	{alias: "celtics", code: "BOS"},
	{alias: "hornets", code: "CHA"},
	{alias: "bulls", code: "CHI"},
	{alias: "cavaliers", code: "CLE"},
	{alias: "cavs", code: "CLE"},
	{alias: "mavericks", code: "DAL"},
	{alias: "nuggets", code: "DEN"},
	{alias: "pistons", code: "DET"},
	{alias: "warriors", code: "GSW"},
	{alias: "rockets", code: "HOU"},
	{alias: "pacers", code: "IND"},
	{alias: "grizzlies", code: "MEM"},
	{alias: "heat", code: "MIA"},
	{alias: "bucks", code: "MIL"},
	{alias: "timberwolves", code: "MIN"},
	{alias: "wolves", code: "MIN"},
	{alias: "pelicans", code: "NOP"},
	{alias: "knicks", code: "NYK"},
	{alias: "magic", code: "ORL"},
	{alias: "sixers", code: "PHI"},
	{alias: "76ers", code: "PHI"},
	{alias: "suns", code: "PHX"},
	{alias: "blazers", code: "POR"},
	{alias: "kings", code: "SAC"},
	{alias: "spurs", code: "SAS"},
	{alias: "raptors", code: "TOR"},
	{alias: "jazz", code: "UTA"},
	{alias: "wizards", code: "WAS"},
}

func normalizeTeamCode(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}

	// 中国队名 -> 英文缩写
	if abbr := convertTeamName(trimmed); abbr != trimmed {
		return strings.ToUpper(strings.TrimSpace(abbr))
	}

	upper := strings.ToUpper(trimmed)
	if len(upper) >= 2 && len(upper) <= 4 {
		return upper
	}

	lower := strings.ToLower(trimmed)
	for _, item := range englishTeamAliases {
		if strings.Contains(lower, item.alias) {
			return item.code
		}
	}

	return upper
}

func normalizePlayerName(name string) string {
	replacer := strings.NewReplacer(
		".", " ",
		"-", " ",
		",", " ",
		"'", "",
		"’", "",
		"(", " ",
		")", " ",
	)
	clean := replacer.Replace(strings.ToLower(strings.TrimSpace(name)))
	return strings.Join(strings.Fields(clean), " ")
}

func calcPowerFromStats(g entity.PlayerGameStats) float64 {
	return float64(g.Points) + 1.2*float64(g.Rebounds) + 1.5*float64(g.Assists) +
		3*float64(g.Steals) + 3*float64(g.Blocks) - float64(g.Turnovers)
}

func calcUsageProxyFromStats(g entity.PlayerGameStats) float64 {
	proxy := float64(g.FieldGoalsAttempted) + 0.44*float64(g.FreeThrowsAttempted) + float64(g.Turnovers)
	if proxy > 0 {
		return proxy
	}
	// 没有命中/出手字段时回退到基础持球代理
	return float64(g.Points) + 0.7*float64(g.Assists) + 0.4*float64(g.Turnovers)
}

func inferSeasonByGameDate(gameDate string) string {
	dt, ok := parseISODate(gameDate)
	if !ok {
		return ""
	}
	startYear := dt.Year()
	if dt.Month() < 10 {
		startYear--
	}
	return fmt.Sprintf("%d-%02d", startYear, (startYear+1)%100)
}

func parseISODate(value string) (time.Time, bool) {
	dt, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return normalizeDateOnly(dt), true
}

func normalizeDateOnly(dt time.Time) time.Time {
	y, m, d := dt.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func clamp(val, minVal, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func roundTo(val float64, precision int) float64 {
	p := math.Pow10(precision)
	return math.Round(val*p) / p
}

func padRight(s string, length int) string {
	runeStr := []rune(s)
	// CJK 字符占 2 个宽度
	width := 0
	for _, r := range runeStr {
		if r > 127 {
			width += 2
		} else {
			width++
		}
	}
	if width >= length {
		return s
	}
	return s + strings.Repeat(" ", length-width)
}
