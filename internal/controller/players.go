package controller

import (
	"net/http"
	"strconv"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/model"
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
		playerInfo, err := a.playersService.GetPlayerInfo(ctx, uint(playerID), userID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "realtime" // 默认模式为分时数据
		}

		var rows []*model.PriceHistoryRow
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

		mockGameData := &api.GameData{
			PlayerID:       uint32(playerID),
			PlayerNameShow: playerInfo.Players.ShowName,
			Standard: &api.GameDataStandard{
				Time:                 28.6,
				Points:               29.5,
				Rebound:              6.9,
				ReboundOffense:       2.3,
				ReboundDefense:       4.6,
				Assists:              2.8,
				Blocks:               0.3,
				Steals:               0.7,
				Turnovers:            1.2,
				Fouls:                2.1,
				PercentOfThrees:      0.3,
				PercentOfTwoPointers: 0.5,
				PercentOfFreeThrows:  0.8,
			},
			NbaToday: []*api.GameDataNbaToday{
				{
					Date:      "2026-01-23",
					VsHome:    "湖人",
					VsAway:    "勇士",
					IsHome:    true,
					Points:    29,
					Rebound:   7,
					Assists:   6,
					Blocks:    3,
					Steals:    1,
					Turnovers: 2,
				},
			},
		}
		return api.PlayerHistoryRes{PlayerInfo: playerInfo, PlayerHistory: rows, GameData: mockGameData}, nil
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
