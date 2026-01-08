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

type API struct {
	db *db.DB
}

func NewAPI(database *db.DB) *API {
	return &API{db: database}
}

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

func (a *API) Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

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
		players, err := db.GetPlayersByIDs(ctx, a.db, playerIDs)
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

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func formatTimeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
