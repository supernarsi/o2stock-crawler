package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"o2stock-crawler/internal/config"
)

const (
	tokenURL   = "https://api.weixin.qq.com/cgi-bin/token"
	sendSubURL = "https://api.weixin.qq.com/cgi-bin/message/subscribe/send"
)

// Client 微信小程序 API 客户端（access_token 缓存 + 订阅消息发送）
type Client struct {
	cfg       config.WechatConfig
	mu        sync.Mutex
	token     string
	expiresAt time.Time
	httpc     *http.Client
}

// NewClient 创建微信客户端
func NewClient(cfg config.WechatConfig) *Client {
	return &Client{
		cfg:   cfg,
		httpc: &http.Client{Timeout: 10 * time.Second},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

// getToken 获取 access_token（带缓存，提前 5 分钟刷新）
func (c *Client) getToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expiresAt.Add(-5*time.Minute)) {
		return c.token, nil
	}

	u, _ := url.Parse(tokenURL)
	q := u.Query()
	q.Set("grant_type", "client_credential")
	q.Set("appid", c.cfg.AppID)
	q.Set("secret", c.cfg.AppSecret)
	u.RawQuery = q.Encode()

	resp, err := c.httpc.Get(u.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.ErrCode != 0 {
		return "", fmt.Errorf("wechat token api: errcode=%d errmsg=%s", tr.ErrCode, tr.ErrMsg)
	}

	c.token = tr.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.token, nil
}

type SubscribeSendResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// SubscribeData 模板变量，key 为模板占位符名（如 thing2、amount4）
type SubscribeData map[string]struct {
	Value string `json:"value"`
}

// SendSubscribe 发送订阅消息
func (c *Client) SendSubscribe(openID, templateID, page string, data SubscribeData) error {
	token, err := c.getToken()
	if err != nil {
		return err
	}
	reqURL := sendSubURL + "?access_token=" + url.QueryEscape(token)

	body := map[string]any{
		"touser":      openID,
		"template_id": templateID,
		"page":        page,
		"data":        data,
	}
	raw, _ := json.Marshal(body)

	resp, err := c.httpc.Post(reqURL, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	var sr SubscribeSendResp
	if err := json.Unmarshal(b, &sr); err != nil {
		return err
	}
	if sr.ErrCode != 0 {
		return fmt.Errorf("wechat subscribe send: errcode=%d errmsg=%s", sr.ErrCode, sr.ErrMsg)
	}
	return nil
}

// SendPriceNotify 发送价格订阅通知（固定模板变量结构）
// templateID、page 若为空则使用 Client 配置中的默认值；如仍为空，使用内置默认值。
func (c *Client) SendPriceNotify(openID, playerName, currentPrice, costPrice, remark string) error {
	templateID := c.cfg.SubscribeTemplateID
	if templateID == "" {
		templateID = "SLYNs1NJ5tU9iRPKh8DtQ4PiOiqgtFUJnBJZJfOn6zI"
	}
	page := c.cfg.SubscribePage
	if page == "" {
		page = "pages/player/player"
	}

	now := time.Now().Format("2006-01-02 15:04")
	// 微信 thing 字段长度限制通常较短，这里做简单截断
	if len(playerName) > 20 {
		playerName = playerName[:17] + "..."
	}
	if len(remark) > 20 {
		remark = remark[:17] + "..."
	}

	data := SubscribeData{
		"thing2":  {Value: playerName},
		"amount4": {Value: currentPrice},
		"amount3": {Value: costPrice},
		"time5":   {Value: now},
		"thing6":  {Value: remark},
	}
	return c.SendSubscribe(openID, templateID, page, data)
}

