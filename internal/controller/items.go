package controller

import (
	"net/http"
	"strconv"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/service"
)

// Items 获取道具列表 GET /items（不分页，一次性返回全部，最多 100 条）
func (a *API) Items() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		orderBy := r.URL.Query().Get("order_by")
		orderAsc := r.URL.Query().Get("order_asc") == "true"
		itemName := r.URL.Query().Get("item_name")

		opts := service.ItemListOptions{
			Limit:    100, // 固定最多 100 条，不分页
			OrderBy:  orderBy,
			OrderAsc: orderAsc,
			ItemName: itemName,
		}

		items, err := a.itemsService.ListItems(ctx, opts)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return api.ItemsRes{Items: items}, nil
	})
}

// ItemHistory 获取单个道具价格历史 GET /item-history?item_id=xxx
func (a *API) ItemHistory() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		ctx := r.Context()
		itemIDStr := r.URL.Query().Get("item_id")
		if itemIDStr == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing item_id"}
		}
		itemID, err := strconv.ParseUint(itemIDStr, 10, 32)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid item_id"}
		}

		limit := parseIntDefault(r.URL.Query().Get("limit"), 500)

		itemInfo, history, err := a.itemsService.GetItemHistory(ctx, uint(itemID), limit)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return api.ItemHistoryRes{ItemInfo: *itemInfo, History: history}, nil
	})
}
