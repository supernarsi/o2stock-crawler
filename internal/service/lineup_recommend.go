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

// PlayerPrediction 预测结果及各因子明细
type PlayerPrediction struct {
	PredictedPower    float64
	BaseValue         float64
	AvailabilityScore float64
	StatusTrend       float64
	MatchupFactor     float64
	HomeAwayFactor    float64
	TeamContextFactor float64
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
	for i := range matches {
		teamToMatch[matches[i].IHomeTeamId] = &matches[i]
		teamToMatch[matches[i].IAwayTeamId] = &matches[i]
	}

	// 确定比赛日期（取第一场的日期）
	gameDate := ""
	if len(matches) > 0 {
		gameDate = matches[0].DtDate
	}
	if gameDate == "" {
		return fmt.Errorf("无法确定比赛日期")
	}
	log.Printf("比赛日期: %s", gameDate)

	// 5. 遍历球员列表，构建 NBAGamePlayer 对象
	var players []entity.NBAGamePlayer
	for _, ps := range playerSalaries {
		match, ok := teamToMatch[ps.ITeamId]
		if !ok {
			log.Printf("警告: 球员 %s (team %s) 未找到对应比赛", ps.SPlayerName, ps.ITeamId)
			continue
		}

		nbaPlayerID, _ := strconv.ParseUint(ps.IPlayerId, 10, 32)
		salary, _ := strconv.ParseUint(ps.ISalary, 10, 32)
		combatPower, _ := strconv.ParseFloat(ps.FCombatPower, 64)
		position, _ := strconv.ParseUint(ps.IPosition, 10, 8)

		isHome := ps.ITeamId == match.IHomeTeamId
		teamName := teamIDMap[ps.ITeamId]

		players = append(players, entity.NBAGamePlayer{
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
		})
	}

	// 6. 批量 Upsert
	repo := repositories.NewNBAGamePlayerRepository(s.db.DB)
	if err := repo.BatchUpsert(ctx, players); err != nil {
		return fmt.Errorf("导入球员数据失败: %w", err)
	}

	log.Printf("成功导入 %d 名球员到 nba_game_player 表 (日期: %s)", len(players), gameDate)
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
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	dbPlayerMap := s.loadDBPlayerMap(ctx, playerRepo, allPlayers)
	log.Printf("DB 球员匹配: %d / %d", len(dbPlayerMap), len(allPlayers))

	// 4. 加载历史战绩数据
	statsRepo := repositories.NewStatsRepository(s.db.DB)
	gameStatsMap := s.loadGameStatsMap(ctx, statsRepo, dbPlayerMap)
	log.Printf("历史战绩数据: %d 名球员有记录", len(gameStatsMap))

	// 5. 对每位球员预测战力
	var candidates []PlayerCandidate
	effectiveCount := 0
	for i := range allPlayers {
		pred := s.predictPower(allPlayers[i], allPlayers, injuryMap, dbPlayerMap, gameStatsMap)

		// 更新 DB 中的预测值
		if pred.PredictedPower > 0 {
			effectiveCount++
			_ = gamePlayerRepo.UpdatePredictedPower(ctx, allPlayers[i].ID, pred.PredictedPower)
		}

		candidates = append(candidates, PlayerCandidate{
			Player:     allPlayers[i],
			Prediction: pred,
		})
	}
	log.Printf("有效球员: %d 人 (战力 > 0)", effectiveCount)

	// 6. DP 求解最优阵容
	topLineups := s.solveOptimalLineup(candidates, 150, 5, 3)
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

// --- 球员战力预测（7 维评分） ---

func (s *LineupRecommendService) predictPower(
	player entity.NBAGamePlayer,
	allPlayers []entity.NBAGamePlayer,
	injuryMap map[uint]crawler.InjuryReport,
	dbPlayerMap map[uint]*entity.Player,
	gameStatsMap map[uint][]entity.PlayerGameStats,
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
			opponentTeam := s.getOpponentTeamName(player, allPlayers)
			matchupFactor = s.calcMatchupFactor(stats, opponentTeam, baseValue)
		}
	}

	// Step 5: 因素5 — 球队阵容上下文 (TeamContextFactor)
	teamContextFactor := s.calcTeamContextFactor(player, allPlayers, dbPlayer)

	// Step 6: 因素6 — 主客场因子 (HomeAwayFactor)
	homeAwayFactor := s.calcHomeAwayFactor(player, txPlayerID, gameStatsMap)

	// Step 7: 因素2 — 比赛取消风险 (GameRiskFactor)
	gameRiskFactor := 1.0 // NBA 室内运动，默认无风险

	// Step 8: 综合计算
	predictedPower := baseValue * availabilityScore * statusTrend * matchupFactor * homeAwayFactor * teamContextFactor * gameRiskFactor

	return PlayerPrediction{
		PredictedPower:    roundTo(predictedPower, 1),
		BaseValue:         roundTo(baseValue, 1),
		AvailabilityScore: availabilityScore,
		StatusTrend:       roundTo(statusTrend, 2),
		MatchupFactor:     roundTo(matchupFactor, 2),
		HomeAwayFactor:    roundTo(homeAwayFactor, 2),
		TeamContextFactor: roundTo(teamContextFactor, 2),
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

	// 预筛：过滤掉没有战力的球员
	var allValid []PlayerCandidate
	for _, c := range candidates {
		if c.Prediction.PredictedPower > 0 && c.Player.Salary > 0 {
			allValid = append(allValid, c)
		}
	}

	// 混合预选策略：保留绝对战力 Top 80 和性价比 Top 80，合并去重
	// 这样可以确保 Luka 这种高战力但高工资（性价比中等）的球员被选中
	topPower := make([]PlayerCandidate, len(allValid))
	copy(topPower, allValid)
	sort.Slice(topPower, func(i, j int) bool {
		return topPower[i].Prediction.PredictedPower > topPower[j].Prediction.PredictedPower
	})

	topRatio := make([]PlayerCandidate, len(allValid))
	copy(topRatio, allValid)
	sort.Slice(topRatio, func(i, j int) bool {
		rI := topRatio[i].Prediction.PredictedPower / float64(topRatio[i].Player.Salary)
		rJ := topRatio[j].Prediction.PredictedPower / float64(topRatio[j].Player.Salary)
		return rI > rJ
	})

	seen := make(map[uint]bool)
	var valid []PlayerCandidate

	// 取 Top 80 战力
	limit := 80
	if len(topPower) < limit {
		limit = len(topPower)
	}
	for i := 0; i < limit; i++ {
		if !seen[topPower[i].Player.NBAPlayerID] {
			valid = append(valid, topPower[i])
			seen[topPower[i].Player.NBAPlayerID] = true
		}
	}

	// 补充 Top 80 性价比
	limit = 80
	if len(topRatio) < limit {
		limit = len(topRatio)
	}
	for i := 0; i < limit; i++ {
		if !seen[topRatio[i].Player.NBAPlayerID] {
			valid = append(valid, topRatio[i])
			seen[topRatio[i].Player.NBAPlayerID] = true
		}
	}

	log.Printf("混合预选完成: 绝对战力候选 + 性价比候选 -> 共 %d 名球员", len(valid))

	// 调试日志：输出特定球员的预测情况
	for _, c := range allValid {
		if strings.Contains(c.Player.PlayerName, "东契奇") ||
			strings.Contains(c.Player.PlayerName, "马克西") ||
			strings.Contains(c.Player.PlayerName, "弗拉格") {
			log.Printf("DEBUG 球员预测: %s, 战力=%.1f, 性价比=%.2f, 状态=%v",
				c.Player.PlayerName, c.Prediction.PredictedPower,
				c.Prediction.PredictedPower/float64(c.Player.Salary),
				c.Prediction.AvailabilityScore)
		}
	}

	log.Printf("DP 求解: %d 名候选球员, 工资帽 %d, 选 %d 人", len(valid), salaryCap, pickCount)

	var results [][]PlayerCandidate
	excluded := make(map[uint]bool)

	for t := 0; t < topN; t++ {
		// 构建本轮可用球员（排除已选过的组合中的部分球员不适用 — 这里改用完整排除重复组合的方式）
		var available []PlayerCandidate
		for _, c := range valid {
			if !excluded[c.Player.NBAPlayerID] {
				available = append(available, c)
			}
		}

		lineup := s.dpSolve(available, salaryCap, pickCount)
		if lineup == nil {
			break
		}

		results = append(results, lineup)

		// 排除最优组合中性价比最低的球员，强制下一轮选不同组合
		if len(lineup) > 0 {
			worstIdx := 0
			worstRatio := math.MaxFloat64
			for i, c := range lineup {
				ratio := c.Prediction.PredictedPower / float64(c.Player.Salary)
				if ratio < worstRatio {
					worstRatio = ratio
					worstIdx = i
				}
			}
			excluded[lineup[worstIdx].Player.NBAPlayerID] = true
		}
	}

	return results
}

// dpSolve 使用 DP 求解单次最优阵容
func (s *LineupRecommendService) dpSolve(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
) []PlayerCandidate {
	n := len(candidates)
	if n < pickCount {
		return nil
	}

	// dp[j][k] = 选 j 人、总工资 ≤ k 时的最大战力值
	dp := make([][]float64, pickCount+1)
	// track[j][k] = 最后选择的球员索引（用于回溯）
	track := make([][]int, pickCount+1)
	for j := 0; j <= pickCount; j++ {
		dp[j] = make([]float64, salaryCap+1)
		track[j] = make([]int, salaryCap+1)
		for k := range track[j] {
			track[j][k] = -1
		}
	}

	// 初始化 prev[j][k] 用于回溯完整路径
	type choice struct {
		playerIdx int
		prevK     int
	}
	history := make([][][]choice, n)

	for i := 0; i < n; i++ {
		salary := int(candidates[i].Player.Salary)
		power := candidates[i].Prediction.PredictedPower

		history[i] = make([][]choice, pickCount+1)
		for j := 0; j <= pickCount; j++ {
			history[i][j] = make([]choice, salaryCap+1)
		}

		// 从后往前遍历，避免重复选择
		for j := min(pickCount, i+1); j >= 1; j-- {
			for k := salaryCap; k >= salary; k-- {
				newPower := dp[j-1][k-salary] + power
				if newPower > dp[j][k] {
					dp[j][k] = newPower
					track[j][k] = i
				}
			}
		}
	}

	// 找到最优解的工资值
	bestK := 0
	for k := 0; k <= salaryCap; k++ {
		if dp[pickCount][k] > dp[pickCount][bestK] {
			bestK = k
		}
	}

	if dp[pickCount][bestK] <= 0 {
		return nil
	}

	// 回溯获取具体球员组合
	result := s.backtrack(candidates, dp, pickCount, bestK)
	if len(result) != pickCount {
		return nil
	}

	return result
}

// backtrack 通过重新扫描回溯获取具体球员组合
func (s *LineupRecommendService) backtrack(
	candidates []PlayerCandidate,
	dp [][]float64,
	targetJ int,
	targetK int,
) []PlayerCandidate {
	n := len(candidates)
	var result []PlayerCandidate

	// 从最大索引开始检查每个球员是否被选择
	j := targetJ
	k := targetK
	selected := make([]bool, n)

	// 重新运行 DP 并记录选择路径
	dpCopy := make([][]float64, j+1)
	for jj := 0; jj <= j; jj++ {
		dpCopy[jj] = make([]float64, k+1)
	}

	type decision struct {
		chosen bool
	}
	decisions := make([][][]decision, n)

	for i := 0; i < n; i++ {
		salary := int(candidates[i].Player.Salary)
		power := candidates[i].Prediction.PredictedPower
		decisions[i] = make([][]decision, j+1)
		for jj := 0; jj <= j; jj++ {
			decisions[i][jj] = make([]decision, k+1)
		}

		for jj := min(j, i+1); jj >= 1; jj-- {
			for kk := k; kk >= salary; kk-- {
				newPower := dpCopy[jj-1][kk-salary] + power
				if newPower > dpCopy[jj][kk] {
					dpCopy[jj][kk] = newPower
					decisions[i][jj][kk] = decision{chosen: true}
				}
			}
		}
	}

	// 从后往前回溯
	remJ, remK := j, k
	for i := n - 1; i >= 0 && remJ > 0; i-- {
		if decisions[i][remJ][remK].chosen {
			selected[i] = true
			remK -= int(candidates[i].Player.Salary)
			remJ--
		}
	}

	for i, sel := range selected {
		if sel {
			result = append(result, candidates[i])
		}
	}

	return result
}

// --- 辅助函数 ---

func (s *LineupRecommendService) fetchInjuryMap(ctx context.Context, players []entity.NBAGamePlayer) map[uint]crawler.InjuryReport {
	result := make(map[uint]crawler.InjuryReport)

	reports, err := s.injuryClient.GetInjuryReports(ctx)
	if err != nil {
		log.Printf("获取伤病报告失败（将跳过伤病因素）: %v", err)
		return result
	}

	for _, report := range reports {
		for _, player := range players {
			if crawler.MatchInjuryToPlayer(report.PlayerName, player.PlayerEnName) {
				result[player.NBAPlayerID] = report
				break
			}
		}
	}

	return result
}

func (s *LineupRecommendService) loadDBPlayerMap(ctx context.Context, repo *repositories.PlayerRepository, gamePlayers []entity.NBAGamePlayer) map[uint]*entity.Player {
	result := make(map[uint]*entity.Player)

	// 收集所有 NBAPlayerID
	var nbaIDs []uint
	for _, p := range gamePlayers {
		nbaIDs = append(nbaIDs, p.NBAPlayerID)
	}

	// 从 players 表批量查询
	var dbPlayers []entity.Player
	if err := s.db.Where("nba_player_id IN ?", nbaIDs).Find(&dbPlayers).Error; err != nil {
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
	var txPlayerIDs []uint
	for _, p := range dbPlayerMap {
		if p.TxPlayerID > 0 {
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

	return statsMap
}

func (s *LineupRecommendService) getOpponentTeamName(player entity.NBAGamePlayer, allPlayers []entity.NBAGamePlayer) string {
	for _, p := range allPlayers {
		if p.MatchID == player.MatchID && p.NBATeamID != player.NBATeamID {
			return p.TeamName
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
	for _, g := range stats {
		if g.VsTeamName == opponentTeam {
			vsGames = append(vsGames, g)
		}
	}

	if len(vsGames) >= 2 {
		totalPower := 0.0
		for _, g := range vsGames {
			totalPower += calcPowerFromStats(g)
		}
		avgPower := totalPower / float64(len(vsGames))
		return clamp(avgPower/baseValue, 0.90, 1.10)
	}

	return 1.0
}

func (s *LineupRecommendService) calcTeamContextFactor(player entity.NBAGamePlayer, allPlayers []entity.NBAGamePlayer, dbPlayer *entity.Player) float64 {
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

		medal := medals[i]
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

func calcPowerFromStats(g entity.PlayerGameStats) float64 {
	return float64(g.Points) + 1.2*float64(g.Rebounds) + 1.5*float64(g.Assists) +
		3*float64(g.Steals) + 3*float64(g.Blocks) - float64(g.Turnovers)
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

func tomorrow() string {
	return time.Now().AddDate(0, 0, 1).Format("2006-01-02")
}
