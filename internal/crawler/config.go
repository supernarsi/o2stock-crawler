package crawler

import (
	"errors"
	"os"
)

// Config holds API-related configuration.
type Config struct {
	OpenID      string
	AccessToken string
	NonseStr    string
	Sign        string
	BaseURL     string
	ItemListURL string // 道具列表接口 URL，可选环境变量 OL2_ITEM_LIST_URL
}

// LoadConfigFromEnv loads crawler config from environment variables.
//
// Required:
//   - OL2_OPENID
//   - OL2_ACCESS_TOKEN
//   - OL2_SIGN
//
// Optional:
//   - OL2_NONSE_STR (default "VKE5z")
//   - OL2_BASE_URL
func LoadConfigFromEnv() (*Config, error) {
	openID := os.Getenv("OL2_OPENID")
	accessToken := os.Getenv("OL2_ACCESS_TOKEN")
	sign := os.Getenv("OL2_SIGN")

	if openID == "" || accessToken == "" || sign == "" {
		return nil, errors.New("missing OL2_OPENID or OL2_ACCESS_TOKEN or OL2_SIGN")
	}

	loginChannel := os.Getenv("OL2_LOGIN_CHANNEL")
	if loginChannel == "" {
		loginChannel = "qq"
	}

	nonseStr := os.Getenv("OL2_NONSE_STR")
	if nonseStr == "" {
		nonseStr = "VKE5z"
	}

	baseURL := os.Getenv("OL2_BASE_URL")
	if baseURL == "" {
		baseURL = "https://nba2k2app.game.qq.com/game/trade/rosterList"
	}

	itemListURL := os.Getenv("OL2_ITEM_LIST_URL")
	if itemListURL == "" {
		itemListURL = "https://nba2k2app.game.qq.com/game/trade/itemList"
	}

	return &Config{
		OpenID:      openID,
		AccessToken: accessToken,
		NonseStr:    nonseStr,
		Sign:        sign,
		BaseURL:     baseURL,
		ItemListURL: itemListURL,
	}, nil
}
