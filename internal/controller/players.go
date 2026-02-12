package controller

import (
	"net/http"
	"strconv"
	"strings"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/service"
)

// Players 获取球员列表
func (a *API) Players() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
		page := parseIntDefault(r.URL.Query().Get("page"), 1)
		orderBy := r.URL.Query().Get("order_by")
		orderAsc := r.URL.Query().Get("order_asc") == "true"
		soldOut := r.URL.Query().Get("sold_out") == "true"
		period := parseIntDefault(r.URL.Query().Get("period"), 1)
		pName := r.URL.Query().Get("player_name")
		minPrice := parseIntDefault(r.URL.Query().Get("min_price"), 0)
		maxPrice := parseIntDefault(r.URL.Query().Get("max_price"), 0)
		exFree := r.URL.Query().Get("ex_free") == "true"

		// 可选球队筛选：team=OKC/MIN/...；特殊值 FA 表示自由球员
		teamParam := strings.TrimSpace(r.URL.Query().Get("team"))

		// 解析可选的 user_id (从 Token 获取)
		var userID *uint
		if uid, ok := GetUserIDFromContext(ctx); ok {
			userID = &uid
		}

		opts := service.PlayerListOptions{
			Page:       page,
			Limit:      limit,
			OrderBy:    orderBy,
			OrderAsc:   orderAsc,
			Period:     uint8(period),
			UserID:     userID,
			SoldOut:    soldOut,
			PlayerName: pName,
			MinPrice:   uint(minPrice),
			MaxPrice:   uint(maxPrice),
			ExFree:     exFree,
			TeamAbbr:   teamParam,
		}

		players, err := a.playersService.ListPlayersWithOwned(ctx, opts)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return api.PlayersWithOwnedRes{Players: players}, nil
	})
}

// PlayerHistory 获取单个球员历史价格
func (a *API) PlayerHistory() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		playerIDStr := r.URL.Query().Get("player_id")
		if playerIDStr == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}
		playerID, err := strconv.Atoi(playerIDStr)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_id"}
		}

		var userID *uint
		if uid, ok := GetUserIDFromContext(ctx); ok {
			userID = &uid
		}
		appVersion := ""
		if c := middleware.MustGetClient(ctx); c != nil {
			appVersion = c.AppVersion
		}
		needBbr := ApiVersionGE(appVersion, "1.2.2")
		playerInfo, err := a.playersService.GetPlayerInfo(ctx, uint(playerID), userID, needBbr)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "realtime" // 默认模式为分时数据
		}

		var rows []*dto.PriceHistoryRow
		switch mode {
		case "realtime":
			rows, err = a.playersService.GetPlayerHistoryRealtime(ctx, uint32(playerID))
		case "5d":
			rows, err = a.playersService.GetPlayerHistory5Days(ctx, uint32(playerID))
		case "10d":
			rows, err = a.playersService.GetPlayerHistoryDays(ctx, uint32(playerID), 10)
		case "30d":
			rows, err = a.playersService.GetPlayerHistoryDays(ctx, uint32(playerID), 30)
		case "dailyk":
			rows, err = a.playersService.GetPlayerHistoryDailyK(ctx, uint32(playerID))
		default:
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid mode, must be one of: realtime, 5days, 10d, 30d, dailyk"}
		}

		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		// 初始化默认值，确保不会返回 null
		standard := &api.GameDataStandard{}   // 默认全 0
		nbaToday := []*api.GameDataNbaToday{} // 默认空数组

		if playerInfo.Players.TxPlayerID > 0 {
			standardResult, nbaTodayResult, err := a.playersService.GetPlayerGameData(ctx, playerInfo.Players.TxPlayerID)
			if err != nil {
				// 记录错误但不阻塞返回，使用默认值
				// log.Printf("failed to get player game data for player %d: %v", playerID, err)
			} else {
				// 如果查询成功，使用查询结果
				if standardResult != nil {
					standard = standardResult
				}
				if nbaTodayResult != nil {
					nbaToday = nbaTodayResult
				}
			}
		}

		gameData := &api.GameData{
			PlayerID:       uint32(playerID),
			PlayerNameShow: playerInfo.Players.ShowName,
			Standard:       standard,
			NbaToday:       nbaToday,
		}
		return api.PlayerHistoryRes{PlayerInfo: playerInfo, PlayerHistory: rows, GameData: gameData}, nil
	})
}

// MultiPlayersHistory 批量获取球员历史价格
func (a *API) MultiPlayersHistory() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()

		// 解析 player_ids 参数（支持多个，用逗号分隔）
		playerIDsStr := r.URL.Query().Get("player_ids")
		if playerIDsStr == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_ids"}
		}

		// 解析多个 player_id
		playerIDStrs := splitCommaSeparated(playerIDsStr)
		if len(playerIDStrs) == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_ids"}
		}

		playerIDs := make([]uint32, 0, len(playerIDStrs))
		for _, idStr := range playerIDStrs {
			id64, err := strconv.ParseUint(idStr, 10, 32)
			if err != nil {
				return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_id: " + idStr}
			}
			playerIDs = append(playerIDs, uint32(id64))
		}

		historyList, err := a.playersService.GetMultiPlayersHistory(ctx, playerIDs, 200)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: err.Error()}
		}

		return api.MultiPlayersHistoryRes{History: historyList}, nil
	})
}

// PlayerInvestmentStats 获取球员投资盈亏统计（全平台 u_p_own + players），可选 player_ids 过滤
func (a *API) PlayerInvestmentStats() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		var playerIDs []uint
		if s := r.URL.Query().Get("player_ids"); s != "" {
			parts := splitCommaSeparated(s)
			for _, p := range parts {
				id, err := strconv.ParseUint(p, 10, 32)
				if err != nil {
					return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_ids"}
				}
				playerIDs = append(playerIDs, uint(id))
			}
		}
		statsMap, err := a.playersService.GetPlayerInvestmentStats(ctx, playerIDs)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		list := make([]dto.PlayerInvestmentStats, 0, len(statsMap))
		for _, v := range statsMap {
			list = append(list, v)
		}
		return api.PlayerInvestmentStatsRes{List: list}, nil
	})
}
