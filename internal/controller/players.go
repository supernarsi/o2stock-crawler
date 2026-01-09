package controller

import (
	"net/http"
	"strconv"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/model"
)

// Players 获取球员列表
func (a *API) Players() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
		rows, err := db.ListPlayers(ctx, a.db, limit, offset)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		// 构建返回结果，总是包含 owned 字段
		result := make([]api.PlayerWithOwned, len(rows))
		playerIDs := make([]uint, len(rows))
		for i, p := range rows {
			playerIDs[i] = p.PlayerID
			result[i] = api.PlayerWithOwned{
				Players: *p,
				Owned:   []*model.OwnInfo{}, // 默认为空数组
			}
		}

		// 如果提供了 user_id，查询用户的拥有信息
		userIDStr := r.URL.Query().Get("user_id")
		if userIDStr != "" {
			userID, err := strconv.ParseUint(userIDStr, 10, 32)
			if err != nil {
				return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid user_id"}
			}

			ownedMap, err := db.GetOwnedInfoByPlayerIDs(ctx, a.db, uint(userID), playerIDs)
			if err != nil {
				return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
			}

			// 更新拥有信息
			for i := range result {
				if owned, ok := ownedMap[result[i].PlayerID]; ok {
					result[i].Owned = owned
				}
			}
		}

		return api.PlayersWithOwnedRes{Players: result}, nil
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
		id64, err := strconv.ParseUint(playerIDStr, 10, 32)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_id"}
		}
		limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
		rows, err := db.GetPlayerHistory(ctx, a.db, uint32(id64), limit)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return api.PlayerHistoryRes{PlayerHistory: rows}, nil
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

		// 限制最多查询的球员数量
		if len(playerIDStrs) > 30 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "too many player_ids, maximum 50"}
		}

		playerIDs := make([]uint32, 0, len(playerIDStrs))
		for _, idStr := range playerIDStrs {
			id64, err := strconv.ParseUint(idStr, 10, 32)
			if err != nil {
				return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid player_id: " + idStr}
			}
			playerIDs = append(playerIDs, uint32(id64))
		}

		limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
		historyMap, err := db.GetMultiPlayersHistory(ctx, a.db, playerIDs, limit)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
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

		return api.MultiPlayersHistoryRes{History: historyList}, nil
	})
}
