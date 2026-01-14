package controller

import (
	"net/http"
	"strconv"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/middleware"
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

		// 解析可选的 user_id
		var userID *uint
		if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
			id, err := strconv.ParseUint(userIDStr, 10, 32)
			if err != nil {
				return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid user_id"}
			}
			uid := uint(id)
			userID = &uid
		}

		players, err := a.playersService.ListPlayersWithOwned(ctx, page, limit, orderBy, orderAsc, uint8(period), userID, soldOut)
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
		playerInfo, err := a.playersService.GetPlayerInfo(ctx, uint(playerID))
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		period := parseIntDefault(r.URL.Query().Get("period"), 1)
		limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
		rows, err := a.playersService.GetPlayerHistory(ctx, uint32(playerID), uint8(period), limit)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return api.PlayerHistoryRes{PlayerInfo: playerInfo, PlayerHistory: rows}, nil
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
