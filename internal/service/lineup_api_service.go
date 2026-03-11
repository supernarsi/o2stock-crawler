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
}

func NewLineupAPIService(database *db.DB) *LineupAPIService {
	return &LineupAPIService{
		db:            database,
		recommendRepo: repositories.NewLineupRecommendationRepository(database.DB),
		backtestRepo:  repositories.NewLineupBacktestResultRepository(database.DB),
		salaryRepo:    repositories.NewNBAPlayerSalaryRepository(database.DB),
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

	// 2. 获取 today 的推荐数据和回测数据
	var todayRecs []entity.LineupRecommendation
	var todayBests []entity.LineupBacktestResult
	if latestDate != "" {
		todayRecs, _ = s.recommendRepo.GetByDateAndType(ctx, latestDate, entity.LineupRecommendationTypeAIRecommended)
		// 同时查询当日的回测真实最优数据（如果已回测则有数据）
		todayBestsAll, _ := s.backtestRepo.GetByGameDatesAndType(ctx, []string{latestDate}, entity.LineupBacktestResultTypeActualOptimal)
		todayBests = todayBestsAll
	}

	// 3. 获取历史日期
	benchmarkDate := latestDate
	if benchmarkDate == "" {
		benchmarkDate = todayStr
	}
	historyDates, err := s.recommendRepo.GetRecentGameDates(ctx, benchmarkDate, 3)
	if err != nil {
		historyDates = nil
	}
	// 统一日期格式
	for i := range historyDates {
		historyDates[i] = normalizeGameDate(historyDates[i])
	}

	// 4. 批量查询历史日期的推荐阵容和实际最佳阵容
	historyRecs := make(map[string][]entity.LineupRecommendation)
	for _, date := range historyDates {
		recs, _ := s.recommendRepo.GetByDateAndType(ctx, date, entity.LineupRecommendationTypeAIRecommended)
		historyRecs[date] = recs
	}

	bestMap := make(map[string][]entity.LineupBacktestResult)
	if len(historyDates) > 0 {
		historyBests, _ := s.backtestRepo.GetByGameDatesAndType(ctx, historyDates, entity.LineupBacktestResultTypeActualOptimal)
		for _, best := range historyBests {
			key := normalizeGameDate(best.GameDate)
			bestMap[key] = append(bestMap[key], best)
		}
	}

	// 5. 收集所有涉及的 nba_player_id，批量查询 nba_player_salary
	allPlayerIDs := collectAllPlayerIDs(todayRecs, todayBests, historyRecs, bestMap)
	salaryMap, err := s.buildSalaryMap(ctx, allPlayerIDs)
	if err != nil {
		return nil, err
	}

	// 6. 组装 today
	if len(todayRecs) > 0 || len(todayBests) > 0 {
		day := api.NBALineupDay{
			GameDate:    latestDate,
			AIRecommend: make([]api.NBALineupItem, 0),
			ActualBest:  make([]api.NBALineupItem, 0),
		}
		for _, rec := range todayRecs {
			predictedMap := parsePredictedPowerMap(rec.DetailJSON)
			day.AIRecommend = append(day.AIRecommend, api.NBALineupItem{
				Rank:                rec.Rank,
				TotalPredictedPower: rec.TotalPredictedPower,
				TotalActualPower:    rec.TotalActualPower,
				TotalSalary:         rec.TotalSalary,
				Detail:              buildLineupPlayers(recPlayerIDs(rec), salaryMap, predictedMap, nil),
			})
		}
		for _, best := range todayBests {
			actualMap := parseActualPowerMap(best.DetailJSON)
			day.ActualBest = append(day.ActualBest, api.NBALineupItem{
				Rank:                best.Rank,
				TotalPredictedPower: 0,
				TotalActualPower:    &best.TotalActualPower,
				TotalSalary:         best.TotalSalary,
				Detail:              buildLineupPlayers(backtestPlayerIDs(best), salaryMap, nil, actualMap),
			})
		}
		res.Today = &day
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
				predictedMap := parsePredictedPowerMap(rec.DetailJSON)
				day.AIRecommend = append(day.AIRecommend, api.NBALineupItem{
					Rank:                rec.Rank,
					TotalPredictedPower: rec.TotalPredictedPower,
					TotalActualPower:    rec.TotalActualPower,
					TotalSalary:         rec.TotalSalary,
					Detail:              buildLineupPlayers(recPlayerIDs(rec), salaryMap, predictedMap, nil),
				})
			}
		}

		if bests, ok := bestMap[date]; ok {
			for _, best := range bests {
				actualMap := parseActualPowerMap(best.DetailJSON)
				day.ActualBest = append(day.ActualBest, api.NBALineupItem{
					Rank:                best.Rank,
					TotalPredictedPower: 0,
					TotalActualPower:    &best.TotalActualPower,
					TotalSalary:         best.TotalSalary,
					Detail:              buildLineupPlayers(backtestPlayerIDs(best), salaryMap, nil, actualMap),
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

// buildLineupPlayers 用 player IDs + salaryMap + 运行时映射组装球员详情列表
func buildLineupPlayers(
	playerIDs [5]uint,
	salaryMap map[uint]entity.NBAPlayerSalary,
	predictedMap map[uint]float64,
	actualMap map[uint]float64,
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
			p.PredictedPower = predictedMap[pid]
		}
		if actualMap != nil {
			p.ActualPower = actualMap[pid]
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

// parsePredictedPowerMap 从推荐 detail_json 中提取每位球员的 predicted_power
func parsePredictedPowerMap(detailJSONStr string) map[uint]float64 {
	result := make(map[uint]float64)
	if detailJSONStr == "" {
		return result
	}
	var payload struct {
		Players []struct {
			NBAPlayerID    uint    `json:"nba_player_id"`
			PredictedPower float64 `json:"predicted_power"`
		} `json:"players"`
	}
	if err := json.Unmarshal([]byte(detailJSONStr), &payload); err != nil {
		return result
	}
	for _, p := range payload.Players {
		if p.NBAPlayerID > 0 {
			result[p.NBAPlayerID] = p.PredictedPower
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
