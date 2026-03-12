package dto

// FeedbackRequest 反馈请求
type FeedbackRequest struct {
	Code       string `json:"code"`
	Content    string `json:"content"`
	AppVersion string `json:"app_version"` // 可选，由客户端直接传入的版本号
}
