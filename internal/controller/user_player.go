package controller

import (
	"net/http"
	"strings"
	"time"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/middleware"
)

// PlayerIn 标记购买接口
func (a *API) PlayerIn() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.PlayerInReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 参数校验
		if req.PlayerID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}

		// 解析时间
		dt, err := time.Parse("2006-01-02 15:04:05", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02 15:04:05"}
		}

		// 调用服务层
		err = a.userPlayerService.PlayerIn(ctx, userID, req.PlayerID, req.Num, req.Cost, dt)
		if err != nil {
			// 处理业务错误
			if strings.Contains(err.Error(), "already owned more than 2 players") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}

// PlayerOut 标记出售接口
func (a *API) PlayerOut() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.PlayerOutReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 参数校验
		if req.PlayerID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}

		// 解析时间
		dt, err := time.Parse("2006-01-02 15:04:05", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02 15:04:05"}
		}

		// 调用服务层
		err = a.userPlayerService.PlayerOut(ctx, userID, req.PlayerID, req.Cost, dt)
		if err != nil {
			// 处理业务错误
			if strings.Contains(err.Error(), "not own this player yet") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
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
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		rosters, err := a.userPlayerService.GetUserPlayers(ctx, userID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return api.UserPlayersRes{Rosters: rosters}, nil
	})
}

// UserFavList 获取用户收藏球员列表
func (a *API) UserFavList() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		players, err := a.userPlayerService.GetUserFavPlayers(ctx, userID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return api.PlayersWithOwnedRes{Players: players}, nil
	})
}

// UserFavPlayer 用户收藏球员接口
func (a *API) UserFavPlayer() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.UserFavPlayerReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 参数校验
		if req.PlayerID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}

		// 调用服务层
		err := a.userPlayerService.FavPlayer(ctx, userID, req.PlayerID)
		if err != nil {
			// 处理业务错误
			if strings.Contains(err.Error(), "already fav this player") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}
