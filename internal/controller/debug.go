package controller

import (
	"fmt"
	"net/http"
	"os"

	"o2stock-crawler/api"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/wechat"

	"gorm.io/gorm"
)

func (a *API) Test() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		return nil, nil
	})
}

// DebugSendPlayerBreakEvenNotify 内部调试：传入 uid + player_id，给该用户推送该球员的回本订阅消息
//
// 安全约束：
// - 仅在 DEBUG=true 时可用（否则 404）
// - 要求请求头 x-debug=42（否则 401；也可触发签名中间件 debug bypass）
func (a *API) DebugSendPlayerBreakEvenNotify() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		if os.Getenv("DEBUG") != "true" {
			return nil, &middleware.APIError{Status: http.StatusNotFound, Code: http.StatusNotFound, Msg: "not found"}
		}
		if r.Header.Get("x-debug") != "42" {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: http.StatusUnauthorized, Msg: "unauthorized"}
		}

		ctx := r.Context()
		var req api.DebugSendPlayerBreakEvenNotifyReq
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body: " + err.Error()}
		}
		if req.UID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing uid"}
		}
		if req.PlayerID == 0 {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "missing player_id"}
		}

		wxCfg := config.LoadWechatConfigFromEnv()
		if wxCfg.AppID == "" || wxCfg.AppSecret == "" {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: "wx config missing"}
		}
		wc := wechat.NewClient(wxCfg)

		userRepo := repositories.NewUserRepository(a.db.DB)
		playerRepo := repositories.NewPlayerRepository(a.db.DB)
		ownRepo := repositories.NewOwnRepository(a.db.DB)

		user, err := userRepo.GetByID(ctx, req.UID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if user == nil || user.WxOpenID == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "user wx_openid missing"}
		}

		player, err := playerRepo.GetByID(ctx, req.PlayerID)
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if player == nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "player not found"}
		}

		// 固定推送“回本信息”：amount4=当前身价（price_standard），amount3=成本均价（优先取持仓成本）
		costAvg := float64(0)
		own, err := ownRepo.GetLatestActiveByUserAndGoods(ctx, req.UID, req.PlayerID, consts.OwnGoodsPlayer)
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}
		if own != nil && own.BuyCount > 0 {
			costAvg = float64(own.BuyPrice) / float64(own.BuyCount)
		}

		if err := wc.SendPriceNotify(
			user.WxOpenID,
			fmt.Sprintf("%d", player.PriceStandard),
			fmt.Sprintf("%.0f", costAvg),
			"球员已达到回本价格",
			player,
		); err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: err.Error()}
		}

		return nil, nil
	})
}
