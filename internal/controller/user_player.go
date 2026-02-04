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
		dt, err := time.Parse("2006-01-02", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02"}
		}

		notifyType := req.NotifyType
		if notifyType > 2 {
			notifyType = 0
		}
		// 调用服务层
		err = a.userPlayerService.PlayerIn(ctx, userID, req.PlayerID, req.Num, req.Cost, dt, notifyType)
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

// UserUnFavPlayer 用户取消收藏球员接口
func (a *API) UserUnFavPlayer() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var playerID uint
		// 只有当 Body 存在且不为空时才尝试解析
		if r.Body != nil && r.ContentLength != 0 {
			var req api.UserFavPlayerReq
			if err := middleware.DecodeJSONBody(r, &req); err == nil {
				playerID = req.PlayerID
			}
		}

		// 参数校验
		if playerID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}

		// 调用服务层
		err := a.userPlayerService.UnFavPlayer(ctx, userID, playerID)
		if err != nil {
			// 处理业务错误
			if strings.Contains(err.Error(), "player not in fav list") {
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
		dt, err := time.Parse("2006-01-02", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02"}
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

// PlayerOwn 修改持仓记录接口
func (a *API) PlayerOwnEdit() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.PlayerOwnEditReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 参数校验
		if req.RecordId == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing record_id"}
		}
		if req.PriceIn == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing price_in"}
		}
		if req.Num == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing num"}
		}

		// 解析时间
		var dtOut *time.Time
		dtInTime, err := time.Parse("2006-01-02", req.DtIn)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02"}
		}
		if req.SoldOut {
			// 已出售
			dtOutTime, err := time.Parse("2006-01-02", req.DtOut)
			if err != nil {
				return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02"}
			}
			dtOut = &dtOutTime
			if dtOutTime.Before(dtInTime) {
				return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "dt_out must be after dt_in"}
			}
		} else {
			dtOut = nil
			req.PriceOut = 0
		}
		// 查询记录是否存在
		record, err := a.userPlayerService.GetPlayerOwn(ctx, userID, req.RecordId)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
		}
		if record == nil {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: "record not found"}
		}

		// 调用服务层
		err = a.userPlayerService.EditPlayerOwn(ctx, userID, req.RecordId, req.PriceIn, req.PriceOut, req.Num, &dtInTime, dtOut)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
		}
		return nil, nil
	})
}

// PlayerOwn 删除持仓记录接口
func (a *API) PlayerOwnDel() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.PlayerOwnDeleteReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		// 参数校验
		if req.RecordId == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing record_id"}
		}

		// 调用服务层
		err := a.userPlayerService.DeletePlayerOwn(ctx, userID, req.RecordId)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
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
			if strings.Contains(err.Error(), "already fav this player") || strings.Contains(err.Error(), "fav limit exceeded") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}

// PlayerPriceNotify 修改球员价格订阅
func (a *API) PlayerPriceNotify() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.PlayerPriceNotifyReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}
		if req.PlayerID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}
		if req.NotifyType > 2 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid notify_type"}
		}

		err := a.userPlayerService.SetPlayerNotify(ctx, userID, req.PlayerID, req.NotifyType)
		if err != nil {
			if strings.Contains(err.Error(), "未找到可修改的持仓记录") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return nil, nil
	})
}
