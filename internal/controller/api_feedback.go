package controller

import (
	"log"
	"net/http"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/middleware"
)

// Feedback 提交反馈接口
func (a *API) Feedback() http.HandlerFunc {
	return middleware.API(func(r *http.Request) (any, *middleware.APIError) {
		var req dto.FeedbackRequest
		if err := middleware.DecodeJSONBody(r, &req); err != nil {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "invalid request body"}
		}

		if req.Content == "" {
			return nil, &middleware.APIError{Status: http.StatusBadRequest, Code: http.StatusBadRequest, Msg: "反馈内容不能为空"}
		}

		uid, _ := GetUserIDFromContext(r.Context())
		client := middleware.MustGetClient(r.Context())

		// 优先使用请求 Body 中的版本号，否则使用 Header 中的
		appVersion := req.AppVersion
		if appVersion == "" {
			appVersion = client.AppVersion
		}

		if err := a.feedbackService.SubmitFeedback(r.Context(), uid, req.WxOpenID, req.Content, appVersion, client.IP, uint8(client.OS)); err != nil {
			log.Printf("Submit feedback failed: %v", err)
			return nil, &middleware.APIError{Status: http.StatusInternalServerError, Code: http.StatusInternalServerError, Msg: "提交反馈失败，请稍后重试"}
		}

		return map[string]string{"message": "ok"}, nil
	})
}
