package controller

import (
	"net/http"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/middleware"
)

// NBALineups 返回 NBA 推荐阵容数据（支持通过 date 查询指定日期及前三天历史）
func (a *API) NBALineups() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		queryDate := r.URL.Query().Get("date")
		userID, _ := GetUserIDFromContext(r.Context())
		res, err := a.lineupService.GetNBALineups(r.Context(), queryDate, userID)
		if err != nil {
			return nil, &middleware.APIError{
				Status: http.StatusInternalServerError,
				Code:   http.StatusInternalServerError,
				Msg:    "failed to get nba lineups",
			}
		}
		
		// 如果数据为空，提供一个空的返回而非 nil，保持良好的前端接口规范
		if res == nil {
			return &api.NBALineupsRes{
				History: []api.NBALineupDay{},
			}, nil
		}
		return res, nil
	})
}
