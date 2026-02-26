package controller

import (
	"net/http"
	"o2stock-crawler/internal/middleware"
	"o2stock-crawler/internal/utils"
	"time"
)

// Healthz 健康检查接口
func (a *API) Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// CrawlerHealthz 任务健康检查，返回所有任务的最近成功执行时间和健康状态
func (a *API) CrawlerHealthz() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		statuses, err := a.taskStatusRepo.GetAll(r.Context())
		if err != nil {
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: 1, Msg: "查询任务状态失败"}
		}

		updateAt := map[string]int64{
			"o2stock-crawler-ol2": 0,
			"o2stock-crawler-tx":  0,
			"o2stock-player-ipi":  0,
		}

		for _, s := range statuses {
			updateAt[s.TaskName] = s.LastSuccessAt.Unix()
		}

		nowTs := time.Now().Unix()

		healthStatus := map[string]bool{
			"o2stock-crawler-ol2": false,
			"o2stock-crawler-tx":  false,
			"o2stock-player-ipi":  false,
		}

		// o2stock-crawler-ol2 1 小时
		if updateAt["o2stock-crawler-ol2"] > 0 && nowTs-updateAt["o2stock-crawler-ol2"] <= 3600 {
			healthStatus["o2stock-crawler-ol2"] = true
		}

		// o2stock-crawler-ol2 在 03:00~08:00 之间不抓取数据，固定返回 true
		if utils.IsOl2CrawlerSleepTime(time.Now()) {
			healthStatus["o2stock-crawler-ol2"] = true
		}

		// o2stock-crawler-tx 25 小时
		if updateAt["o2stock-crawler-tx"] > 0 && nowTs-updateAt["o2stock-crawler-tx"] <= 25*3600 {
			healthStatus["o2stock-crawler-tx"] = true
		}

		// o2stock-player-ipi 72 小时
		if updateAt["o2stock-player-ipi"] > 0 && nowTs-updateAt["o2stock-player-ipi"] <= 72*3600 {
			healthStatus["o2stock-player-ipi"] = true
		}

		return map[string]any{
			"update_at":     updateAt,
			"health_status": healthStatus,
		}, nil
	})
}
