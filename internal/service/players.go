package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/entity"
	"time"
)

// PlayersService 球员服务
type PlayersService struct {
	db *db.DB
}

// NewPlayersService 创建球员服务实例
func NewPlayersService(database *db.DB) *PlayersService {
	return &PlayersService{db: database}
}

// PlayerListOptions 封装球员列表查询参数
type PlayerListOptions struct {
	Page       int
	Limit      int
	OrderBy    string
	OrderAsc   bool
	Period     uint8
	UserID     *uint
	SoldOut    bool
	PlayerName string
	MinPrice   uint
	MaxPrice   uint
	ExFree     bool
}

// ListPlayersWithOwned 获取球员列表，支持分页、排序，并可选地包含用户的拥有信息
func (s *PlayersService) ListPlayersWithOwned(ctx context.Context, opts PlayerListOptions) ([]api.PlayerWithOwned, error) {
	// 参数校验与默认值
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}

	filter := repositories.PlayerFilter{
		Page:       opts.Page,
		Limit:      opts.Limit,
		OrderBy:    s.mapOrderBy(opts.OrderBy),
		OrderAsc:   opts.OrderAsc,
		Period:     opts.Period,
		SoldOut:    opts.SoldOut,
		PlayerName: opts.PlayerName,
		MinPrice:   opts.MinPrice,
		MaxPrice:   opts.MaxPrice,
		ExFree:     opts.ExFree,
	}

	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	players, err := playerRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list players: %w", err)
	}

	var ownedMap map[uint][]dto.OwnInfo
	if opts.UserID != nil && len(players) > 0 {
		pids := make([]uint, len(players))
		for i, p := range players {
			pids[i] = p.PlayerID
		}
		ownRepo := repositories.NewOwnRepository(s.db.DB)
		ownRecords, err := ownRepo.GetByPlayerIDs(ctx, *opts.UserID, pids)
		if err != nil {
			return nil, fmt.Errorf("failed to get owned info: %w", err)
		}
		ownedMap = s.mapOwnRecordsToInfoMap(ownRecords)
	}

	var favMap map[uint]bool
	if opts.UserID != nil && len(players) > 0 {
		pids := make([]uint, len(players))
		for i, p := range players {
			pids[i] = p.PlayerID
		}
		favRepo := repositories.NewFavRepository(s.db.DB)
		favMap, err = favRepo.GetFavMap(ctx, *opts.UserID, pids)
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
	}

	// 构建返回结果
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: s.entityToDTO(p),
			Owned:                 []*dto.OwnInfo{},
			IsFav:                 false,
		}
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				for j := range owned {
					result[i].Owned = append(result[i].Owned, &owned[j])
				}
			}
		}
		if favMap != nil {
			if isFav, ok := favMap[p.PlayerID]; ok {
				result[i].IsFav = isFav
			}
		}
	}

	return result, nil
}

// mapOrderBy maps user-facing order by names to database column names
func (s *PlayersService) mapOrderBy(orderBy string) string {
	mapping := map[string]string{
		"price_change": "price_change_1d",
		"price":        "price_standard",
		"power5":       "power_per5",
		"power10":      "power_per10",
		"overall":      "over_all",
	}
	if mapped, ok := mapping[orderBy]; ok {
		return mapped
	}
	return "player_id"
}

// GetPlayerHistory 获取单个球员历史价格
func (s *PlayersService) GetPlayerHistory(ctx context.Context, playerID uint, period uint8, limit int) ([]*dto.PriceHistoryRow, error) {
	historyRepo := repositories.NewHistoryRepository(s.db.DB)
	startTime := s.calculateStartTime(period)
	rows, err := historyRepo.GetByPlayerID(ctx, playerID, startTime, limit)
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

func (s *PlayersService) calculateStartTime(period uint8) time.Time {
	now := time.Now()
	switch period {
	case 1: // Period1Day
		return now.AddDate(0, 0, -1)
	case 2: // Period3Days
		return now.AddDate(0, 0, -3)
	case 3: // Period1Week
		return now.AddDate(0, 0, -7)
	default:
		return now.AddDate(0, 0, -1)
	}
}

// GetPlayerHistoryRealtime 获取分时数据（当天所有成交记录）
func (s *PlayersService) GetPlayerHistoryRealtime(ctx context.Context, playerID uint32) ([]*dto.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.GetRealtime(ctx, uint(playerID))
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistory5Days 获取五日数据
func (s *PlayersService) GetPlayerHistory5Days(ctx context.Context, playerID uint32) ([]*dto.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.Get5Days(ctx, uint(playerID))
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistoryDailyK 获取日K线数据
func (s *PlayersService) GetPlayerHistoryDailyK(ctx context.Context, playerID uint32) ([]*dto.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.GetDailyK(ctx, uint(playerID))
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistoryDays 获取指定天数的历史数据
func (s *PlayersService) GetPlayerHistoryDays(ctx context.Context, playerID uint32, days int) ([]*dto.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.GetDays(ctx, uint(playerID), days)
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetMultiPlayersHistory 批量获取球员历史价格
func (s *PlayersService) GetMultiPlayersHistory(ctx context.Context, playerIDs []uint32, limit int) ([]api.PlayerHistoryItem, error) {
	if len(playerIDs) == 0 {
		return []api.PlayerHistoryItem{}, nil
	}

	uIDs := make([]uint, len(playerIDs))
	for i, id := range playerIDs {
		uIDs[i] = uint(id)
	}

	repo := repositories.NewHistoryRepository(s.db.DB)
	historyMap, err := repo.GetMultiPlayersHistory(ctx, uIDs, limit)
	if err != nil {
		return nil, err
	}

	result := make([]api.PlayerHistoryItem, 0, len(playerIDs))
	for _, pid := range playerIDs {
		rows := historyMap[uint(pid)]
		result = append(result, api.PlayerHistoryItem{
			PlayerID: pid,
			History:  s.mapToHistoryRows(rows),
		})
	}
	return result, nil
}

func (s *PlayersService) mapToHistoryRows(rows []entity.PlayerPriceHistory) []*dto.PriceHistoryRow {
	res := make([]*dto.PriceHistoryRow, len(rows))
	for i, r := range rows {
		res[i] = &dto.PriceHistoryRow{
			PlayerId:         r.PlayerID,
			AtDate:           r.AtDate,
			AtDateHourStr:    r.AtDateHour,
			PriceStandard:    uint32(r.PriceStandard),
			PriceCurrentSale: int32(r.PriceCurrentSale),
			PriceLower:       uint32(r.PriceLower),
			PriceUpper:       uint32(r.PriceUpper),
		}
	}
	return res
}

// GetPlayerInfo 获取单个球员信息
func (s *PlayersService) GetPlayerInfo(ctx context.Context, playerID uint, userID *uint) (*api.PlayerWithOwned, error) {
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	pp, err := playerRepo.GetByID(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player info: %w", err)
	}

	isFav := false
	owned := []*dto.OwnInfo{}
	if userID != nil {
		ownRepo := repositories.NewOwnRepository(s.db.DB)
		ownRecords, err := ownRepo.GetByPlayerIDs(ctx, *userID, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get owned info: %w", err)
		}
		ownedMap := s.mapOwnRecordsToInfoMap(ownRecords)
		if ownedR, ok := ownedMap[playerID]; ok {
			for i := range ownedR {
				owned = append(owned, &ownedR[i])
			}
		}

		favRepo := repositories.NewFavRepository(s.db.DB)
		favMap, err := favRepo.GetFavMap(ctx, *userID, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
		if isFavR, ok := favMap[playerID]; ok {
			isFav = isFavR
		}
	}

	return &api.PlayerWithOwned{
		PlayerWithPriceChange: s.entityToDTO(*pp),
		Owned:                 owned,
		IsFav:                 isFav,
	}, nil
}

// GetPlayerGameData 获取球员比赛数据和赛季平均数据
func (s *PlayersService) GetPlayerGameData(ctx context.Context, nbaPlayerID uint) (*api.GameDataStandard, []*api.GameDataNbaToday, error) {
	statsRepo := repositories.NewStatsRepository(s.db.DB)

	// 1. 查询赛季平均数据
	seasonStats, err := statsRepo.GetSeasonStats(ctx, nbaPlayerID)
	if err != nil {
		fmt.Printf("GetPlayerSeasonStats error: %v\n", err)
	}

	standard := &api.GameDataStandard{}
	if seasonStats != nil {
		timePerGame := 0.0
		if seasonStats.GamesPlayed > 0 {
			timePerGame = math.Round(seasonStats.Minutes / float64(seasonStats.GamesPlayed))
		}
		standard = &api.GameDataStandard{
			Time:                 timePerGame,
			Points:               seasonStats.Points,
			Rebound:              seasonStats.Rebounds,
			ReboundOffense:       seasonStats.ReboundsOffensive,
			ReboundDefense:       seasonStats.ReboundsDefensive,
			Assists:              seasonStats.Assists,
			Blocks:               seasonStats.Blocks,
			Steals:               seasonStats.Steals,
			Turnovers:            seasonStats.Turnovers,
			Fouls:                seasonStats.Fouls,
			PercentOfThrees:      seasonStats.ThreePointPercentage * 100,
			PercentOfTwoPointers: seasonStats.FieldGoalPercentage * 100,
			PercentOfFreeThrows:  seasonStats.FreeThrowPercentage * 100,
		}
	}

	// 2. 查询最近 5 场比赛
	gameStats, err := statsRepo.GetRecentGameStats(ctx, nbaPlayerID, 5)
	if err != nil {
		fmt.Printf("GetRecentPlayerGameStats error: %v\n", err)
	}

	nbaToday := []*api.GameDataNbaToday{}
	for _, gs := range gameStats {
		nbaToday = append(nbaToday, &api.GameDataNbaToday{
			Date:      gs.GameDate.Format("2006-01-02"),
			VsHome:    gs.PlayerTeamName,
			VsAway:    gs.VsTeamName,
			IsHome:    gs.IsHome,
			Points:    uint(gs.Points),
			Rebound:   uint(gs.Rebounds),
			Assists:   uint(gs.Assists),
			Blocks:    uint(gs.Blocks),
			Steals:    uint(gs.Steals),
			Turnovers: uint(gs.Turnovers),
		})
	}

	return standard, nbaToday, nil
}

// CalculateAndSyncPower 计算并同步所有球员的战力值
func (s *PlayersService) CalculateAndSyncPower(ctx context.Context) error {
	log.Printf(">>> 开始执行球员战力值计算任务 <<<")
	startTime := time.Now()

	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	statsRepo := repositories.NewStatsRepository(s.db.DB)

	targetPlayers, err := playerRepo.GetAllTargetPlayers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get target players: %w", err)
	}

	totalPlayers := len(targetPlayers)
	successCount := 0
	skippedCount := 0
	log.Printf("找到符合条件的球员数量: %d", totalPlayers)

	for _, p := range targetPlayers {
		recentGames, err := statsRepo.GetRecentGameStats(ctx, p.NBAPlayerID, 10)
		if err != nil {
			log.Printf("[PlayerID: %d] 获取比赛记录失败: %v", p.PlayerID, err)
			continue
		}

		if len(recentGames) == 0 {
			skippedCount++
			continue
		}

		powers := make([]float64, len(recentGames))
		for i, gs := range recentGames {
			powers[i] = float64(gs.Points) +
				1.2*float64(gs.Rebounds) +
				1.5*float64(gs.Assists) +
				3.0*float64(gs.Steals) +
				3.0*float64(gs.Blocks) -
				float64(gs.Turnovers)
		}

		num5 := min(len(powers), 5)
		sum5 := 0.0
		for i := 0; i < num5; i++ {
			sum5 += powers[i]
		}
		avg5 := round(sum5/float64(num5), 1)

		num10 := len(powers)
		sum10 := 0.0
		for i := 0; i < num10; i++ {
			sum10 += powers[i]
		}
		avg10 := round(sum10/float64(num10), 1)

		if avg5 == p.PowerPer5 && avg10 == p.PowerPer10 {
			skippedCount++
			continue
		}

		if err := playerRepo.UpdatePower(ctx, p.PlayerID, avg5, avg10); err != nil {
			log.Printf("[PlayerID: %d] 更新战力值失败: %v", p.PlayerID, err)
		} else {
			successCount++
		}
	}

	endTime := time.Now()
	log.Printf(">>> 球员战力值计算任务结束 <<<")
	log.Printf("耗时: %v, 处理球员总数: %d, 成功更新数量: %d, 跳过未变化数量: %d",
		endTime.Sub(startTime), totalPlayers, successCount, skippedCount)

	return nil
}

// SyncAllPlayersPriceChanges 计算并同步所有球员的日涨跌幅
func (s *PlayersService) SyncAllPlayersPriceChanges(ctx context.Context) error {
	log.Printf(">>> 开始同步球员价格涨跌幅 <<<")
	startTime := time.Now()

	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	historyRepo := repositories.NewHistoryRepository(s.db.DB)

	players, err := playerRepo.List(ctx, repositories.PlayerFilter{Limit: 10000})
	if err != nil {
		return fmt.Errorf("failed to list players: %w", err)
	}

	now := time.Now()
	oneDayAgo := now.AddDate(0, 0, -1)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	map1d, err := historyRepo.GetPriceHistoryMap(ctx, oneDayAgo)
	if err != nil {
		log.Printf("获取 1d 历史价格快照失败: %v", err)
	}
	map7d, err := historyRepo.GetPriceHistoryMap(ctx, sevenDaysAgo)
	if err != nil {
		log.Printf("获取 7d 历史价格快照失败: %v", err)
	}

	total := len(players)
	successCount := 0
	log.Printf("找到球员数量: %d", total)

	for _, p := range players {
		pc1d := 0.0
		pc7d := 0.0

		if h1d, ok := map1d[p.PlayerID]; ok && h1d.PriceStandard > 0 {
			pc1d = float64(int(p.PriceStandard)-int(h1d.PriceStandard)) / float64(h1d.PriceStandard)
		}

		if h7d, ok := map7d[p.PlayerID]; ok && h7d.PriceStandard > 0 {
			pc7d = float64(int(p.PriceStandard)-int(h7d.PriceStandard)) / float64(h7d.PriceStandard)
		}

		pc1d = round(pc1d, 2)
		pc7d = round(pc7d, 2)

		if err := playerRepo.UpdatePriceChanges(ctx, p.PlayerID, pc1d, pc7d); err != nil {
			log.Printf("[PlayerID: %d] 更新涨跌幅失败: %v", p.PlayerID, err)
		} else {
			successCount++
		}
	}

	log.Printf(">>> 同步球员价格涨跌幅完成，耗时: %v, 总数: %d, 成功: %d <<<",
		time.Since(startTime), total, successCount)
	return nil
}

// Helper methods
func (s *PlayersService) entityToDTO(p entity.Player) dto.PlayerWithPriceChange {
	return dto.PlayerWithPriceChange{
		Players: dto.Players{
			PlayerID:          p.PlayerID,
			NBAPlayerID:       p.NBAPlayerID,
			ShowName:          p.ShowName,
			EnName:            p.EnName,
			TeamAbbr:          p.TeamAbbr,
			Version:           p.Version,
			CardType:          p.CardType,
			PlayerImg:         p.PlayerImg,
			PriceStandard:     p.PriceStandard,
			PriceCurrentLower: p.PriceCurrentLowest,
			PriceSaleLower:    p.PriceSaleLower,
			PriceSaleUpper:    p.PriceSaleUpper,
			OverAll:           p.OverAll,
			PowerPer5:         p.PowerPer5,
			PowerPer10:        p.PowerPer10,
			PriceChange1d:     p.PriceChange1d,
			PriceChange7d:     p.PriceChange7d,
		},
		PriceChange: p.PriceChange1d,
	}
}

func (s *PlayersService) mapOwnRecordsToInfoMap(records []entity.UserPlayerOwn) map[uint][]dto.OwnInfo {
	result := make(map[uint][]dto.OwnInfo)
	for _, o := range records {
		dtOut := ""
		if o.SellTime != nil {
			dtOut = o.SellTime.Format("2006-01-02 15:04:05")
		}
		info := dto.OwnInfo{
			PlayerID: o.PlayerID,
			PriceIn:  o.BuyPrice,
			PriceOut: o.SellPrice,
			OwnSta:   uint8(o.Sta),
			OwnNum:   o.BuyCount,
			DtIn:     o.BuyTime.Format("2006-01-02 15:04:05"),
			DtOut:    dtOut,
		}
		result[o.PlayerID] = append(result[o.PlayerID], info)
	}
	return result
}

func round(val float64, precision int) float64 {
	p := math.Pow10(precision)
	return math.Round(val*p) / p
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
