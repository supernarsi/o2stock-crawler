package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
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

	filter := db.PlayerFilter{
		Page:       opts.Page,
		Limit:      opts.Limit,
		OrderBy:    opts.OrderBy,
		OrderAsc:   opts.OrderAsc,
		Period:     opts.Period,
		SoldOut:    opts.SoldOut,
		PlayerName: opts.PlayerName,
	}

	query := db.NewPlayersQuery(filter)
	players, ownedMap, err := query.ListPlayersWithOwned(ctx, s.db, opts.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to list players with owned: %w", err)
	}

	var favMap map[uint]bool
	if opts.UserID != nil && len(players) > 0 {
		pids := make([]uint, len(players))
		for i, p := range players {
			pids[i] = p.PlayerID
		}
		favMap, err = db.GetFavMapByPlayerIDs(ctx, s.db, *opts.UserID, pids)
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
	}

	// 构建返回结果，总是包含 owned 字段
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: *p,
			Owned:                 []*model.OwnInfo{}, // 默认为空数组
			IsFav:                 false,
		}
		// 如果有拥有信息，填充到结果中
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				result[i].Owned = owned
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
func (s *PlayersService) GetPlayerHistory(ctx context.Context, playerID uint32, period uint8, limit int) ([]*model.PriceHistoryRow, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := db.NewPlayerHistoryQuery(playerID, limit)
	rows, err := query.GetPlayerHistory(ctx, s.db, period)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history: %w", err)
	}
	return rows, nil
}

// GetMultiPlayersHistory 批量获取球员历史价格
func (s *PlayersService) GetMultiPlayersHistory(ctx context.Context, playerIDs []uint32, limit int) ([]api.PlayerHistoryItem, error) {
	if len(playerIDs) == 0 {
		return []api.PlayerHistoryItem{}, nil
	}

	// 限制最多查询的球员数量
	if len(playerIDs) > 30 {
		return nil, fmt.Errorf("too many player_ids, maximum 30")
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := db.NewMultiPlayersHistoryQuery(playerIDs, limit)
	historyMap, err := query.GetMultiPlayersHistory(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get multi players history: %w", err)
	}

	// 将 map 转换为列表形式，保持请求的 player_ids 顺序
	historyList := make([]api.PlayerHistoryItem, 0, len(playerIDs))
	for _, pid := range playerIDs {
		history, ok := historyMap[pid]
		if !ok {
			history = []*model.PriceHistoryRow{} // 如果没有数据，返回空数组
		}
		historyList = append(historyList, api.PlayerHistoryItem{
			PlayerID: pid,
			History:  history,
		})
	}

	return historyList, nil
}

// GetPlayerInfo 获取单个球员信息
func (s *PlayersService) GetPlayerInfo(ctx context.Context, playerID uint, userID *uint) (*api.PlayerWithOwned, error) {
	query := db.NewPlayersQuery(db.PlayerFilter{Page: 1, Limit: 1, OrderAsc: true})
	pp, err := query.GetPlayerInfo(ctx, s.db, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player info: %w", err)
	}

	isFav := false
	owned := []*model.OwnInfo{}
	if userID != nil {
		// 查询已拥有的球员
		ownedMap, err := db.NewUserPlayerOwnQuery(*userID).GetOwnedInfoByPlayerIDs(ctx, s.db, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get owned info: %w", err)
		}
		if ownedR, ok := ownedMap[playerID]; ok {
			owned = ownedR
		}

		// 查询已收藏的球员
		favMap, err := db.GetFavMapByPlayerIDs(ctx, s.db, *userID, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
		if isFavR, ok := favMap[playerID]; ok {
			isFav = isFavR
		}
	}

	return &api.PlayerWithOwned{
		PlayerWithPriceChange: model.PlayerWithPriceChange{Players: *pp},
		Owned:                 owned,
		IsFav:                 isFav,
	}, nil
}

// GetPlayerHistoryRealtime 获取分时数据（当天所有成交记录）
func (s *PlayersService) GetPlayerHistoryRealtime(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	rows, err := db.NewPlayerHistoryQuery(playerID, 0).GetRealtime(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history realtime: %w", err)
	}
	return rows, nil
}

// GetPlayerHistory5Days 获取五日数据（最近5个自然日的所有成交记录）
func (s *PlayersService) GetPlayerHistory5Days(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	rows, err := db.NewPlayerHistoryQuery(playerID, 0).Get5Days(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history 5days: %w", err)
	}
	return rows, nil
}

// GetPlayerHistoryDailyK 获取日K线数据（最近30个自然日的K线数据）
func (s *PlayersService) GetPlayerHistoryDailyK(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	rows, err := db.NewPlayerHistoryQuery(playerID, 0).GetDailyK(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history dailyk: %w", err)
	}
	return rows, nil
}

// GetPlayerHistoryDays 获取指定天数的历史数据（每天一条）
func (s *PlayersService) GetPlayerHistoryDays(ctx context.Context, playerID uint32, days int) ([]*model.PriceHistoryRow, error) {
	rows, err := db.NewPlayerHistoryQuery(playerID, 0).GetDays(ctx, s.db, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history days: %w", err)
	}
	return rows, nil
}

// GetPlayerGameData 获取球员比赛数据和赛季平均数据
func (s *PlayersService) GetPlayerGameData(ctx context.Context, nbaPlayerID uint) (*api.GameDataStandard, []*api.GameDataNbaToday, error) {
	statsQuery := db.NewPlayerStatsQuery(nbaPlayerID)

	// 1. 查询赛季平均数据
	seasonStats, err := statsQuery.GetSeasonStats(ctx, s.db)
	if err != nil && err != db.ErrNoRows {
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
	gameStats, err := statsQuery.GetRecentGameStats(ctx, s.db, 5)
	if err != nil && err != db.ErrNoRows {
		fmt.Printf("GetRecentPlayerGameStats error: %v\n", err)
	}

	nbaToday := []*api.GameDataNbaToday{} // 默认空数组
	for _, gs := range gameStats {
		nbaToday = append(nbaToday, &api.GameDataNbaToday{
			Date:      gs.GameDate.Format("2006-01-02"),
			VsHome:    gs.PlayerTeamName,
			VsAway:    gs.VsTeamName,
			IsHome:    gs.IsHome,
			Points:    gs.Points,
			Rebound:   gs.Rebounds,
			Assists:   gs.Assists,
			Blocks:    gs.Blocks,
			Steals:    gs.Steals,
			Turnovers: gs.Turnovers,
		})
	}

	return standard, nbaToday, nil
}

// CalculateAndSyncPower 计算并同步所有球员的战力值（近5场和近10场平均值）
func (s *PlayersService) CalculateAndSyncPower(ctx context.Context) error {
	log.Printf(">>> 开始执行球员战力值计算任务 <<<")
	startTime := time.Now()

	playersQuery := db.NewPlayersQuery(db.PlayerFilter{})
	targetPlayers, err := playersQuery.GetAllTargetPlayers(ctx, s.db)
	if err != nil {
		return fmt.Errorf("failed to get target players: %w", err)
	}

	totalPlayers := len(targetPlayers)
	successCount := 0
	log.Printf("找到符合条件的球员数量: %d", totalPlayers)

	for _, p := range targetPlayers {
		statsQuery := db.NewPlayerStatsQuery(p.NBAPlayerID)
		recentGames, err := statsQuery.GetRecentGameStats(ctx, s.db, 10)
		if err != nil {
			log.Printf("[PlayerID: %d] 获取比赛记录失败: %v", p.PlayerID, err)
			continue
		}

		if len(recentGames) == 0 {
			// 如果没有记录，设置为 0
			// if err := playersQuery.UpdatePlayerPower(ctx, s.db, p.PlayerID, 0, 0); err != nil {
			// 	log.Printf("[PlayerID: %d] 更新战力值为0失败: %v", p.PlayerID, err)
			// } else {
			// 	successCount++
			// }
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

		if err := playersQuery.UpdatePlayerPower(ctx, s.db, p.PlayerID, avg5, avg10); err != nil {
			log.Printf("[PlayerID: %d] 更新战力值失败: %v", p.PlayerID, err)
		} else {
			successCount++
		}
	}

	endTime := time.Now()
	log.Printf(">>> 球员战力值计算任务结束 <<<")
	log.Printf("耗时: %v, 处理球员总数: %d, 成功更新数量: %d", endTime.Sub(startTime), totalPlayers, successCount)

	return nil
}

func round(val float64, precision int) float64 {
	p := math.Pow10(precision)
	return math.Round(val*p) / p
}
