package config

import "os"

// WechatConfig 微信小程序配置（用于订阅消息等）
type WechatConfig struct {
	AppID     string // 小程序 appid
	AppSecret string // 小程序 secret

	// 订阅消息模板 id（回本/盈利通知）
	// 订阅消息模板 id（回本/盈利通知）
	SubscribeTemplateID string
	// 点击消息打开的小程序页面
	SubscribePage string

	// 阵容推荐订阅通知模板 ID
	LineupSubscribeTemplateID string
	// 阵容推荐通知跳转页面
	LineupSubscribePage string
}

// LoadWechatConfigFromEnv 从环境变量加载微信配置
func LoadWechatConfigFromEnv() WechatConfig {
	return WechatConfig{
		AppID:                     os.Getenv("WX_APP_ID"),
		AppSecret:                 os.Getenv("WX_APP_SECRET"),
		SubscribeTemplateID:       os.Getenv("WX_SUBSCRIBE_TEMPLATE_ID"),
		SubscribePage:             os.Getenv("WX_SUBSCRIBE_PAGE"),
		LineupSubscribeTemplateID: os.Getenv("WX_LINEUP_SUBSCRIBE_TEMPLATE_ID"),
		LineupSubscribePage:       os.Getenv("WX_LINEUP_SUBSCRIBE_PAGE"),
	}
}
