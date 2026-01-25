package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/model"
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
		OrderBy:    opts.OrderBy,
		OrderAsc:   opts.OrderAsc,
		Period:     opts.Period,
		SoldOut:    opts.SoldOut,
		PlayerName: opts.PlayerName,
		MinPrice:   opts.MinPrice,
		MaxPrice:   opts.MaxPrice,
		ExFree:     opts.ExFree,
	}

	query := db.NewPlayersQuery(s.db, filter)
	players, err := query.ListPlayers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list players: %w", err)
	}

	var ownedMap map[uint][]model.OwnInfo
	if opts.UserID != nil && len(players) > 0 {
		pids := make([]uint, len(players))
		for i, p := range players {
			pids[i] = p.PlayerID
		}
		ownedQuery := db.NewUserPlayerOwnQuery(s.db, *opts.UserID)
		ownedMap, err = ownedQuery.GetOwnedInfoByPlayerIDs(ctx, s.db, pids)
		if err != nil {
			return nil, fmt.Errorf("failed to get owned info: %w", err)
		}
	}

	var favMap map[uint]bool
	if opts.UserID != nil && len(players) > 0 {
		pids := make([]uint, len(players))
		for i, p := range players {
			pids[i] = p.PlayerID
		}
		favQuery := db.NewFavQuery(s.db)
		favMap, err = favQuery.GetFavMapByPlayerIDs(ctx, *opts.UserID, pids)
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
	}

	// 构建返回结果
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: p,
			Owned:                 []*model.OwnInfo{}, // 默认为空数组
			IsFav:                 false,
		}
		// 如果有拥有信息，填充到结果中
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				for j := range owned {
					result[i].Owned = append(result[i].Owned, &owned[j])
				}
			}
		}
		// 如果有收藏信息，填充到结果中
		if favMap != nil {
			if isFav, ok := favMap[p.PlayerID]; ok {
				result[i].IsFav = isFav
			}
		}
	}

	return result, nil
}

// GetPlayerHistory 获取单个球员历史价格
func (s *PlayersService) GetPlayerHistory(ctx context.Context, playerID uint, period uint8, limit int) ([]*model.PriceHistoryRow, error) {
	query := db.NewHistoryQuery(s.db, playerID)
	rows, err := query.GetPlayerHistory(ctx, period)
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistoryRealtime 获取分时数据（当天所有成交记录）
func (s *PlayersService) GetPlayerHistoryRealtime(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.GetRealtime(ctx, uint(playerID))
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistory5Days 获取五日数据（最近5个自然日的所有成交记录）
func (s *PlayersService) GetPlayerHistory5Days(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.Get5Days(ctx, uint(playerID))
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistoryDailyK 获取日K线数据（最近30个自然日的K线数据）
func (s *PlayersService) GetPlayerHistoryDailyK(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	repo := repositories.NewHistoryRepository(s.db.DB)
	rows, err := repo.GetDailyK(ctx, uint(playerID))
	if err != nil {
		return nil, err
	}
	return s.mapToHistoryRows(rows), nil
}

// GetPlayerHistoryDays 获取指定天数的历史数据（每天一条）
func (s *PlayersService) GetPlayerHistoryDays(ctx context.Context, playerID uint32, days int) ([]*model.PriceHistoryRow, error) {
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

func (s *PlayersService) mapToHistoryRows(rows []models.PlayerPriceHistory) []*model.PriceHistoryRow {
	res := make([]*model.PriceHistoryRow, len(rows))
	for i, r := range rows {
		res[i] = &model.PriceHistoryRow{
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
	query := db.NewPlayersQuery(s.db, repositories.PlayerFilter{Page: 1, Limit: 1, OrderAsc: true})
	pp, err := query.GetPlayerInfo(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player info: %w", err)
	}

	isFav := false
	owned := []*model.OwnInfo{}
	if userID != nil {
		// 查询已拥有的球员
		ownedMap, err := db.NewUserPlayerOwnQuery(s.db, *userID).GetOwnedInfoByPlayerIDs(ctx, s.db, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get owned info: %w", err)
		}
		if ownedR, ok := ownedMap[playerID]; ok {
			for i := range ownedR {
				owned = append(owned, &ownedR[i])
			}
		}

		// 查询已收藏的球员
		favQuery := db.NewFavQuery(s.db)
		favMap, err := favQuery.GetFavMapByPlayerIDs(ctx, *userID, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
		if isFavR, ok := favMap[playerID]; ok {
			isFav = isFavR
		}
	}

	return &api.PlayerWithOwned{
		PlayerWithPriceChange: model.PlayerWithPriceChange{
			Players: model.Players{
				PlayerID:          pp.PlayerID,
				NBAPlayerID:       pp.NBAPlayerID,
				ShowName:          pp.ShowName,
				EnName:            pp.EnName,
				TeamAbbr:          pp.TeamAbbr,
				Version:           pp.Version,
				CardType:          pp.CardType,
				PlayerImg:         pp.PlayerImg,
				PriceStandard:     pp.PriceStandard,
				PriceCurrentLower: pp.PriceCurrentLowest,
				PriceSaleLower:    pp.PriceSaleLower,
				PriceSaleUpper:    pp.PriceSaleUpper,
				OverAll:           pp.OverAll,
				PowerPer5:         pp.PowerPer5,
				PowerPer10:        pp.PowerPer10,
				PriceChange1d:     pp.PriceChange1d,
				PriceChange7d:     pp.PriceChange7d,
			},
			PriceChange: pp.PriceChange1d,
		},
		Owned: owned,
		IsFav: isFav,
	}, nil
}

// GetPlayerGameData 获取球员比赛数据和赛季平均数据
func (s *PlayersService) GetPlayerGameData(ctx context.Context, nbaPlayerID uint) (*api.GameDataStandard, []*api.GameDataNbaToday, error) {
	statsQuery := db.NewPlayerStatsQuery(s.db, nbaPlayerID)

	// 1. 查询赛季平均数据
	seasonStats, err := statsQuery.GetSeasonStats(ctx)
	if err != nil {
		fmt.Printf("GetPlayerSeasonStats error: %v\n", err)
	}

	standard := &api.GameDataStandard{} // 默认全 0
	if seasonStats != nil {
		timePerGame := 0.0
		if seasonStats.GamesPlayed > 0 {
			// 保留1位小数
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
	gameStats, err := statsQuery.GetRecentGameStats(ctx, 5)
	if err != nil {
		fmt.Printf("GetRecentPlayerGameStats error: %v\n", err)
	}

	nbaToday := []*api.GameDataNbaToday{} // 默认空数组
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

// CalculateAndSyncPower 计算并同步所有球员的战力值（近5场和近10场平均值）
func (s *PlayersService) CalculateAndSyncPower(ctx context.Context) error {
	log.Printf(">>> 开始执行球员战力值计算任务 <<<")
	startTime := time.Now()

	playersQuery := db.NewPlayersQuery(s.db, repositories.PlayerFilter{})
	targetPlayers, err := playersQuery.GetAllTargetPlayers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get target players: %w", err)
	}

	totalPlayers := len(targetPlayers)
	successCount := 0
	skippedCount := 0
	log.Printf("找到符合条件的球员数量: %d", totalPlayers)

	for _, p := range targetPlayers {
		statsQuery := db.NewPlayerStatsQuery(s.db, p.NBAPlayerID)
		recentGames, err := statsQuery.GetRecentGameStats(ctx, 10)
		if err != nil {
			log.Printf("[PlayerID: %d] 获取比赛记录失败: %v", p.PlayerID, err)
			continue
		}

		if len(recentGames) == 0 {
			skippedCount++
			continue
		}

		// 计算每一场的战力值
		powers := make([]float64, len(recentGames))
		for i, gs := range recentGames {
			// 单场战力值 = 得分 + (1.2 * 篮板) + (1.5 * 助攻) + (3 * 抢断) + (3 * 盖帽) - 失误
			powers[i] = float64(gs.Points) +
				1.2*float64(gs.Rebounds) +
				1.5*float64(gs.Assists) +
				3.0*float64(gs.Steals) +
				3.0*float64(gs.Blocks) -
				float64(gs.Turnovers)
		}

		// 计算近 5 场平均
		num5 := min(len(powers), 5)
		sum5 := 0.0
		for i := 0; i < num5; i++ {
			sum5 += powers[i]
		}
		avg5 := round(sum5/float64(num5), 1)

		// 计算近 10 场平均
		num10 := len(powers)
		sum10 := 0.0
		for i := 0; i < num10; i++ {
			sum10 += powers[i]
		}
		avg10 := round(sum10/float64(num10), 1)

		// 检查是否有变化
		if avg5 == p.PowerPer5 && avg10 == p.PowerPer10 {
			skippedCount++
			continue
		}

		if err := playersQuery.UpdatePlayerPower(ctx, p.PlayerID, avg5, avg10); err != nil {
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

// SyncAllPlayersPriceChanges 计算并同步所有球员的日涨跌幅（1天）和周涨跌幅（7天）
func (s *PlayersService) SyncAllPlayersPriceChanges(ctx context.Context) error {
	log.Printf(">>> 开始同步球员价格涨跌幅 <<<")
	startTime := time.Now()

	// 1. 获取所有球员列表
	playersQuery := db.NewPlayersQuery(s.db, repositories.PlayerFilter{Limit: 10000}) // 设置一个足够大的 Limit 以获取所有球员
	players, err := playersQuery.ListPlayers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list players: %w", err)
	}

	// 2. 获取 1 天前和 7 天前的价格快照
	now := time.Now()
	oneDayAgo := now.AddDate(0, 0, -1)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	historyRepo := repositories.NewHistoryRepository(s.db.DB)
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

		// 计算 1d 涨跌幅
		if h1d, ok := map1d[p.PlayerID]; ok && h1d.PriceStandard > 0 {
			pc1d = float64(int(p.PriceStandard)-int(h1d.PriceStandard)) / float64(h1d.PriceStandard)
		}

		// 计算 7d 涨跌幅
		if h7d, ok := map7d[p.PlayerID]; ok && h7d.PriceStandard > 0 {
			pc7d = float64(int(p.PriceStandard)-int(h7d.PriceStandard)) / float64(h7d.PriceStandard)
		}

		// 保留两位小数
		pc1d = round(pc1d, 2)
		pc7d = round(pc7d, 2)

		// 更新到数据库
		if err := playersQuery.UpdatePlayerPriceChanges(ctx, p.PlayerID, pc1d, pc7d); err != nil {
			log.Printf("[PlayerID: %d] 更新涨跌幅失败: %v", p.PlayerID, err)
		} else {
			successCount++
		}
	}

	log.Printf(">>> 同步球员价格涨跌幅完成，耗时: %v, 总数: %d, 成功: %d <<<",
		time.Since(startTime), total, successCount)
	return nil
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
