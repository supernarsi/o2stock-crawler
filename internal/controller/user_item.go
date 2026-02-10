package controller

import (
	"net/http"
	"strings"
	"time"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/middleware"
)

// ItemIn 标记购买道具接口
func (a *API) ItemIn() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.ItemInReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		if req.ItemID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing item_id"}
		}
		if req.Num == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing num"}
		}
		if req.Cost == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing cost"}
		}

		dt, err := time.Parse("2006-01-02", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02"}
		}

		notifyType := req.NotifyType
		if notifyType > 2 {
			notifyType = 0
		}

		if err := a.userItemService.ItemIn(ctx, userID, req.ItemID, req.Num, req.Cost, dt, notifyType); err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return nil, nil
	})
}

// ItemOut 标记出售道具接口（指定持仓记录）
func (a *API) ItemOut() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.ItemOutReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		if req.OwnID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing own_id"}
		}
		if req.ItemID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing item_id"}
		}
		if req.Cost == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing cost"}
		}

		dt, err := time.Parse("2006-01-02", req.Dt)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid dt format, expected: 2006-01-02"}
		}

		if err := a.userItemService.ItemOut(ctx, userID, req.OwnID, req.ItemID, req.Cost, dt); err != nil {
			// 业务错误按球员风格返回 code=-1
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "mismatch") || strings.Contains(err.Error(), "not sellable") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return nil, nil
	})
}

// UserItems 获取用户拥有道具列表
func (a *API) UserItems() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		rosters, err := a.userItemService.GetUserItems(ctx, userID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return api.UserItemsRes{Rosters: rosters}, nil
	})
}

// ItemPriceNotify 修改道具价格订阅
func (a *API) ItemPriceNotify() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req api.ItemPriceNotifyReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}
		if req.ItemID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing item_id"}
		}
		if req.NotifyType > 2 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid notify_type"}
		}

		err := a.userItemService.SetItemNotify(ctx, userID, req.ItemID, req.NotifyType)
		if err != nil {
			if strings.Contains(err.Error(), "未找到可修改的持仓记录") {
				return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
			}
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return nil, nil
	})
}

// FavItem 收藏道具接口
func (a *API) FavItem() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req struct {
			ItemID uint `json:"item_id"`
		}
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		if req.ItemID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing item_id"}
		}

		if err := a.userItemService.FavItem(ctx, userID, req.ItemID); err != nil {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
		}
		return nil, nil
	})
}

// UnFavItem 取消收藏道具接口
func (a *API) UnFavItem() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		var req struct {
			ItemID uint `json:"item_id"`
		}
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}

		if req.ItemID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing item_id"}
		}

		if err := a.userItemService.UnFavItem(ctx, userID, req.ItemID); err != nil {
			return nil, &middleware.APIError{Status: http.StatusOK, Code: -1, Msg: err.Error()}
		}
		return nil, nil
	})
}

// UserFavItems 获取收藏道具列表接口
func (a *API) UserFavItems() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		items, err := a.userItemService.GetUserFavItems(ctx, userID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		return struct {
			Items []dto.Item `json:"items"`
		}{Items: items}, nil
	})
}
