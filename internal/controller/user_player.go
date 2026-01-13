package controller

import (
	"net/http"
	"strconv"
	"time"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/model"
)

// PlayerIn 标记购买接口
func (a *API) PlayerIn() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		var req api.PlayerInReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 解析时间
		dt, err := time.Parse("2006-01-02 15:04:05", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02 15:04:05"}
		}

		// 获取 user_id（这里简化处理，实际应该从认证中间件获取）
		// 临时从请求体中获取
		userID := req.UserID
		if userID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing user_id"}
		}

		ctx := r.Context()

		// 检查是否已拥有超过 2 条
		count, err := db.CountOwnedPlayers(ctx, a.db, userID, req.PlayerID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if count >= 2 {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: "already owned more than 2 players"}
		}

		// 插入购买记录
		if err := db.InsertPlayerOwn(ctx, a.db, uint(userID), req.PlayerID, req.Num, req.Cost, dt); err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}

// PlayerOut 标记出售接口
func (a *API) PlayerOut() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		var req api.PlayerOutReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 解析时间
		dt, err := time.Parse("2006-01-02 15:04:05", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02 15:04:05"}
		}

		// 获取 user_id
		userID := req.UserID
		if userID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing user_id"}
		}

		ctx := r.Context()

		// 更新为已出售状态
		if err := db.UpdatePlayerOwnToSold(ctx, a.db, userID, req.PlayerID, req.Cost, dt); err != nil {
			if err == db.ErrNoRows {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: "not own this player yet"}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}

// UserPlayers 获取用户拥有球员列表
func (a *API) UserPlayers() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()

		userIDStr := r.URL.Query().Get("user_id")
		if userIDStr == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing user_id"}
		}
		userID, err := strconv.ParseUint(userIDStr, 10, 32)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid user_id"}
		}

		// 获取用户拥有的球员记录
		ownedList, err := db.GetUserOwnedPlayers(ctx, a.db, uint(userID))
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		if len(ownedList) == 0 {
			return api.UserPlayersRes{Rosters: []api.OwnedPlayer{}}, nil
		}

		// 获取球员 ID 列表
		playerIDs := make([]uint, len(ownedList))
		playerIDMap := make(map[uint]bool)
		for i, o := range ownedList {
			playerIDs[i] = o.PlayerID
			playerIDMap[o.PlayerID] = true
		}

		// 查询球员详细信息
		query := db.NewPlayersQuery(1, 100, "", true)
		players, err := query.GetPlayersByIDs(ctx, a.db, playerIDs)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		// 构建响应
		playerMap := make(map[uint]*model.Players)
		for _, p := range players {
			playerMap[p.PlayerID] = p
		}

		rosters := make([]api.OwnedPlayer, 0, len(ownedList))
		for _, o := range ownedList {
			pp := playerMap[o.PlayerID]
			if pp == nil {
				continue // 跳过找不到球员信息的记录
			}
			rosters = append(rosters, api.OwnedPlayer{
				PlayerID: o.PlayerID,
				PriceIn:  o.PriceIn,
				PriceOut: o.PriceOut,
				OwnSta:   o.OwnSta,
				OwnNum:   o.NumIn,
				DtIn:     o.DtIn.Format("2006-01-02 15:04:05"),
				DtOut:    formatTimeOrEmpty(o.DtOut),
				PP:       *pp,
			})
		}

		return api.UserPlayersRes{Rosters: rosters}, nil
	})
}

// UserFavPlayer 用户收藏球员接口
func (a *API) UserFavPlayer() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		var req api.UserFavPlayerReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		ctx := r.Context()

		// 检查是否已收藏
		count, err := db.CountFavPlayer(ctx, a.db, req.UserID, req.PlayerID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if count > 0 {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: "already fav this player"}
		}

		// 插入收藏记录
		if err := db.InsertFavPlayer(ctx, a.db, req.UserID, req.PlayerID); err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}
