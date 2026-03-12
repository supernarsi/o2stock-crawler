package service

import (
	"context"
	"encoding/json"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"time"
)

type LineupAPIService struct {
	db            *db.DB
	recommendRepo *repositories.LineupRecommendationRepository
	backtestRepo  *repositories.LineupBacktestResultRepository
	salaryRepo    *repositories.NBAPlayerSalaryRepository
	playerRepo    *repositories.PlayerRepository
}

func NewLineupAPIService(database *db.DB) *LineupAPIService {
	return &LineupAPIService{
		db:            database,
		recommendRepo: repositories.NewLineupRecommendationRepository(database.DB),
		backtestRepo:  repositories.NewLineupBacktestResultRepository(database.DB),
		salaryRepo:    repositories.NewNBAPlayerSalaryRepository(database.DB),
		playerRepo:    repositories.NewPlayerRepository(database.DB),
	}
}

// GetNBALineups 获取指定日期和历史的推荐阵容及真实最佳阵容
func (s *LineupAPIService) GetNBALineups(ctx context.Context, queryDate string) (*api.NBALineupsRes, error) {
	todayStr := time.Now().Format("2006-01-02")

	var latestDate string
	var err error

	if queryDate != "" {
		latestDate = queryDate
	} else {
		latestDate, err = s.recommendRepo.GetLatestGameDate(ctx, todayStr)
		if err != nil {
			latestDate = ""
		}
	}
	// 统一日期格式，去除 T00:00:00Z 后缀
	latestDate = normalizeGameDate(latestDate)

	var res api.NBALineupsRes
	res.History = make([]api.NBALineupDay, 0)

	// 3. 获取历史日期
	benchmarkDate := latestDate
	if benchmarkDate == "" {
		benchmarkDate = todayStr
	}
	historyDates, err := s.recommendRepo.GetRecentGameDates(ctx, benchmarkDate, 3)
	if err != nil {
		historyDates = nil
	}
	historyDates = normalizeGameDateList(historyDates)

	// 2. 收集所有需要查询的日期
	allQueryDates := make([]string, 0)
	if latestDate != "" {
		allQueryDates = append(allQueryDates, latestDate)
	}
	for _, date := range historyDates {
		allQueryDates = append(allQueryDates, normalizeGameDate(date))
	}

	// 3. 批量查询推荐阵容
	historyRecs := make(map[string][]entity.LineupRecommendation)
	todayRecs := make([]entity.LineupRecommendation, 0)
	if len(allQueryDates) > 0 {
		allRecs, _ := s.recommendRepo.GetByDatesAndType(ctx, allQueryDates, entity.LineupRecommendationTypeAIRecommended)
		for _, rec := range allRecs {
			dateKey := normalizeGameDate(rec.GameDate)
			if dateKey == latestDate {
				todayRecs = append(todayRecs, rec)
			} else {
				historyRecs[dateKey] = append(historyRecs[dateKey], rec)
			}
		}
	}

	// 4. 批量查询实际最佳阵容
	bestMap := make(map[string][]entity.LineupBacktestResult)
	todayBests := make([]entity.LineupBacktestResult, 0)
	if len(allQueryDates) > 0 {
		allBests, _ := s.backtestRepo.GetByGameDatesAndType(ctx, allQueryDates, entity.LineupBacktestResultTypeActualOptimal)
		for _, best := range allBests {
			dateKey := normalizeGameDate(best.GameDate)
			if dateKey == latestDate {
				// today 的也同时放入 map 和 切片，向下兼容
				todayBests = append(todayBests, best)
			}
			bestMap[dateKey] = append(bestMap[dateKey], best)
		}
	}

	// 5. 收集所有涉及的 nba_player_id，批量查询 nba_player_salary 和 avatar
	allPlayerIDs := collectAllPlayerIDs(todayRecs, todayBests, historyRecs, bestMap)
	salaryMap, err := s.buildSalaryMap(ctx, allPlayerIDs)
	if err != nil {
		return nil, err
	}
	avatarMap, _ := s.buildAvatarMap(ctx, allPlayerIDs)

	// 6. 组装 today
	if len(todayRecs) > 0 || len(todayBests) > 0 {
		day := api.NBALineupDay{
			GameDate:    latestDate,
			AIRecommend: make([]api.NBALineupItem, 0),
			ActualBest:  make([]api.NBALineupItem, 0),
		}
		for _, rec := range todayRecs {
			predictedMap := parsePredictedDetailMap(rec.DetailJSON)
			day.AIRecommend = append(day.AIRecommend, api.NBALineupItem{
				Rank:                rec.Rank,
				TotalPredictedPower: rec.TotalPredictedPower,
				TotalActualPower:    safeFloat64(rec.TotalActualPower),
				TotalSalary:         rec.TotalSalary,
				Detail:              buildLineupPlayers(recPlayerIDs(rec), salaryMap, predictedMap, nil, avatarMap),
			})
		}
		for _, best := range todayBests {
			actualMap := parseActualPowerMap(best.DetailJSON)
			day.ActualBest = append(day.ActualBest, api.NBALineupItem{
				Rank:                best.Rank,
				TotalPredictedPower: 0,
				TotalActualPower:    best.TotalActualPower,
				TotalSalary:         best.TotalSalary,
				Detail:              buildLineupPlayers(backtestPlayerIDs(best), salaryMap, nil, actualMap, avatarMap),
			})
		}
		res.Today = &day
		res.TodayUpdateAt = latestRecommendationUpdateAt(todayRecs)
	}

	// 7. 组装历史数据
	for _, date := range historyDates {
		day := api.NBALineupDay{
			GameDate:    date,
			AIRecommend: make([]api.NBALineupItem, 0),
			ActualBest:  make([]api.NBALineupItem, 0),
		}

		if recs, ok := historyRecs[date]; ok {
			for _, rec := range recs {
				predictedMap := parsePredictedDetailMap(rec.DetailJSON)
				day.AIRecommend = append(day.AIRecommend, api.NBALineupItem{
					Rank:                rec.Rank,
					TotalPredictedPower: rec.TotalPredictedPower,
					TotalActualPower:    safeFloat64(rec.TotalActualPower),
					TotalSalary:         rec.TotalSalary,
					Detail:              buildLineupPlayers(recPlayerIDs(rec), salaryMap, predictedMap, nil, avatarMap),
				})
			}
		}

		if bests, ok := bestMap[date]; ok {
			for _, best := range bests {
				actualMap := parseActualPowerMap(best.DetailJSON)
				day.ActualBest = append(day.ActualBest, api.NBALineupItem{
					Rank:                best.Rank,
					TotalPredictedPower: 0,
					TotalActualPower:    best.TotalActualPower,
					TotalSalary:         best.TotalSalary,
					Detail:              buildLineupPlayers(backtestPlayerIDs(best), salaryMap, nil, actualMap, avatarMap),
				})
			}
		}

		res.History = append(res.History, day)
	}

	return &res, nil
}

// buildSalaryMap 批量查询 nba_player_salary 并构建 map[nba_player_id]NBAPlayerSalary
func (s *LineupAPIService) buildSalaryMap(ctx context.Context, playerIDs []uint) (map[uint]entity.NBAPlayerSalary, error) {
	result := make(map[uint]entity.NBAPlayerSalary)
	if len(playerIDs) == 0 {
		return result, nil
	}

	rows, err := s.salaryRepo.GetByNBAPlayerIDs(ctx, playerIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.NBAPlayerID] = row
	}
	return result, nil
}

// buildAvatarMap 批量查询玩家信息，并按 nba_player_id 返回 avatar (player_img)
func (s *LineupAPIService) buildAvatarMap(ctx context.Context, playerIDs []uint) (map[uint]string, error) {
	result := make(map[uint]string)
	if len(playerIDs) == 0 {
		return result, nil
	}

	players, err := s.playerRepo.BatchGetByNBAPlayerIDs(ctx, playerIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range players {
		if _, ok := result[p.NBAPlayerID]; !ok {
			result[p.NBAPlayerID] = p.PlayerImg
		}
	}
	return result, nil
}

// buildLineupPlayers 用 player IDs + salaryMap + 运行时映射组装球员详情列表
func buildLineupPlayers(
	playerIDs [5]uint,
	salaryMap map[uint]entity.NBAPlayerSalary,
	predictedMap map[uint]PredictedPlayerDetail,
	actualMap map[uint]float64,
	avatarMap map[uint]string,
) []*api.NBALineupPlayer {
	players := make([]*api.NBALineupPlayer, 0, 5)
	for _, pid := range playerIDs {
		if pid == 0 {
			continue
		}
		p := &api.NBALineupPlayer{
			NBAPlayerID: pid,
		}
		if salary, ok := salaryMap[pid]; ok {
			p.Name = salary.PlayerName
			p.Team = salary.TeamName
			p.Salary = salary.Salary
			p.AvgPower = salary.CombatPower
		}
		if predictedMap != nil {
			p.PredictedPower = predictedMap[pid].PredictedPower
			p.Available = predictedMap[pid].AvailabilityScore
		}
		if actualMap != nil {
			p.ActualPower = actualMap[pid]
		}
		if avatarMap != nil {
			p.Avatar = avatarMap[pid]
		}
		players = append(players, p)
	}
	return players
}

func recPlayerIDs(rec entity.LineupRecommendation) [5]uint {
	return [5]uint{rec.Player1ID, rec.Player2ID, rec.Player3ID, rec.Player4ID, rec.Player5ID}
}

func backtestPlayerIDs(bt entity.LineupBacktestResult) [5]uint {
	return [5]uint{bt.Player1ID, bt.Player2ID, bt.Player3ID, bt.Player4ID, bt.Player5ID}
}

// collectAllPlayerIDs 从所有阵容记录中收集去重后的 nba_player_id
func collectAllPlayerIDs(
	todayRecs []entity.LineupRecommendation,
	todayBests []entity.LineupBacktestResult,
	historyRecs map[string][]entity.LineupRecommendation,
	bestMap map[string][]entity.LineupBacktestResult,
) []uint {
	set := make(map[uint]struct{})
	addRecIDs := func(rec entity.LineupRecommendation) {
		for _, id := range recPlayerIDs(rec) {
			if id > 0 {
				set[id] = struct{}{}
			}
		}
	}
	addBtIDs := func(bt entity.LineupBacktestResult) {
		for _, id := range backtestPlayerIDs(bt) {
			if id > 0 {
				set[id] = struct{}{}
			}
		}
	}

	for _, rec := range todayRecs {
		addRecIDs(rec)
	}
	for _, bt := range todayBests {
		addBtIDs(bt)
	}
	for _, recs := range historyRecs {
		for _, rec := range recs {
			addRecIDs(rec)
		}
	}
	for _, bests := range bestMap {
		for _, bt := range bests {
			addBtIDs(bt)
		}
	}

	result := make([]uint, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	return result
}

type PredictedPlayerDetail struct {
	PredictedPower    float64
	AvailabilityScore float64
}

// parsePredictedDetailMap 从推荐 detail_json 中提取每位球员的 predicted_power 和 availability_score
func parsePredictedDetailMap(detailJSONStr string) map[uint]PredictedPlayerDetail {
	result := make(map[uint]PredictedPlayerDetail)
	if detailJSONStr == "" {
		return result
	}
	var payload struct {
		Players []struct {
			NBAPlayerID    uint    `json:"nba_player_id"`
			PredictedPower float64 `json:"predicted_power"`
			Factors        struct {
				AvailabilityScore float64 `json:"availability_score"`
			} `json:"factors"`
		} `json:"players"`
	}
	if err := json.Unmarshal([]byte(detailJSONStr), &payload); err != nil {
		return result
	}
	for _, p := range payload.Players {
		if p.NBAPlayerID > 0 {
			result[p.NBAPlayerID] = PredictedPlayerDetail{
				PredictedPower:    p.PredictedPower,
				AvailabilityScore: p.Factors.AvailabilityScore,
			}
		}
	}
	return result
}

// parseActualPowerMap 从回测 detail_json 中提取每位球员的 actual_power
func parseActualPowerMap(detailJSONStr string) map[uint]float64 {
	result := make(map[uint]float64)
	if detailJSONStr == "" {
		return result
	}
	var payload struct {
		Lineup []struct {
			NBAPlayerID uint    `json:"nba_player_id"`
			ActualPower float64 `json:"actual_power"`
		} `json:"lineup"`
	}
	if err := json.Unmarshal([]byte(detailJSONStr), &payload); err != nil {
		return result
	}
	for _, p := range payload.Lineup {
		if p.NBAPlayerID > 0 {
			result[p.NBAPlayerID] = p.ActualPower
		}
	}
	return result
}

// normalizeGameDate 将 game_date 截取前 10 位，确保格式为 YYYY-MM-DD
func normalizeGameDate(date string) string {
	if len(date) > 10 {
		return date[:10]
	}
	return date
}

func normalizeGameDateList(dates []string) []string {
	normalized := make([]string, 0, len(dates))
	seen := make(map[string]struct{}, len(dates))
	for _, date := range dates {
		key := normalizeGameDate(date)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
}

func latestRecommendationUpdateAt(recs []entity.LineupRecommendation) int64 {
	var latest time.Time
	for _, rec := range recs {
		if rec.UpdatedAt.After(latest) {
			latest = rec.UpdatedAt
		}
	}
	if latest.IsZero() {
		return 0
	}
	return latest.Unix()
}

// safeFloat64 安全的解引用 float64
func safeFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
