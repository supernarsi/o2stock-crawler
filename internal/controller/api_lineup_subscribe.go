package controller

import (
	"net/http"
	"o2stock-crawler/internal/middleware"
)

// SubscribeLineup 订阅或取消订阅今日阵容推荐
func (a *API) SubscribeLineup() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		var req struct {
			Action string `json:"action"` // subscribe or unsubscribe
		}
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Msg: "invalid request body"}
		}

		ctx := r.Context()
		userID, ok := GetUserIDFromContext(ctx)
		if !ok {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Msg: "need login"}
		}

		var status uint8
		switch req.Action {
		case "subscribe":
			status = 1
		case "unsubscribe":
			status = 0
		default:
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Msg: "invalid action"}
		}

		// 假设我们在 API 结构体中通过订阅接口来操作，
		// 虽然目前是通过 lineupService 进行常规业务，但订阅操作我们可以直接用 repo
		// 或者扩展阵容服务，这里我们先使用我们在 API 初始化时添加的 repos
		// 实际上由于分层，更推荐在 lineupService 中实现逻辑
		// 但根据项目既有模式（如 PlayerPriceNotify 直接在 api.go 下通过 repero 操作），我们也可以保持一致。
		
		// 查阅 api.go 发现已经有很多 service 了，我们在这里需要确保 API 实例有对应的 repo 或 service。
		// 为了遵循分层原则，我们在 service/lineup_api_service.go 中增加方法是不错的选择。
		
		err := a.lineupService.UpdateSubscription(ctx, userID, status)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Msg: "failed to update subscription"}
		}

		return map[string]any{"subscribed": status == 1}, nil
	})
}
