package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/entity"
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

// GetConfig 获取微信配置
func (c *Client) GetConfig() config.WechatConfig {
	return c.cfg
}

// SubscribeSendResp 订阅消息发送响应
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

// SendPriceNotify 发送价格订阅通知（固定模板变量结构，球员）
// templateID、page 若为空则使用 Client 配置中的默认值；如仍为空，使用内置默认值。
func (c *Client) SendPriceNotify(openID, currentPrice, costPrice, remark string, player *entity.Player) error {
	if player == nil {
		return fmt.Errorf("player is nil")
	}
	templateID := c.cfg.SubscribeTemplateID
	if templateID == "" {
		return fmt.Errorf("subscribe template id is empty")
	}
	page := c.cfg.SubscribePage
	if page == "" {
		page = "pages/player/player?playerId=" + fmt.Sprintf("%d", player.PlayerID)
	}
	now := time.Now().Format("2006-01-02 15:04")
	data := SubscribeData{
		"thing2":  {Value: player.ShowName},
		"amount4": {Value: currentPrice},
		"amount3": {Value: costPrice},
		"time5":   {Value: now},
		"thing6":  {Value: remark},
	}
	return c.SendSubscribe(openID, templateID, page, data)
}

// SendPriceNotifyForItem 发送道具价格订阅通知（与球员共用同一模板 id，thing2 为道具名称）
func (c *Client) SendPriceNotifyForItem(openID, currentPrice, costPrice, remark string, item *entity.Item) error {
	if item == nil {
		return fmt.Errorf("item is nil")
	}
	templateID := c.cfg.SubscribeTemplateID
	if templateID == "" {
		return fmt.Errorf("subscribe template id is empty")
	}
	page := c.cfg.SubscribePage
	if page == "" {
		page = "pages/item/item?itemId=" + fmt.Sprintf("%d", item.ItemID)
	}
	now := time.Now().Format("2006-01-02 15:04")
	data := SubscribeData{
		"thing2":  {Value: item.Name},
		"amount4": {Value: currentPrice},
		"amount3": {Value: costPrice},
		"time5":   {Value: now},
		"thing6":  {Value: remark},
	}
	return c.SendSubscribe(openID, templateID, page, data)
}

// SendLineupNotify 发送阵容推荐订阅通知
// thing1: 类型 (今日NBA阵容推荐)
// time2: 时间 (YYYY-MM-DD HH:mm:ss)
// short_thing4: 我的预测 (预测总战力：258)
// thing5: 温馨提示 (点击查看阵容明细)
func (c *Client) SendLineupNotify(openID, templateID, page string, totalPower float64) error {
	if templateID == "" {
		return fmt.Errorf("lineup subscribe template id is empty")
	}
	if page == "" {
		page = "pages/nba-lineup/nba-lineup"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	data := SubscribeData{
		"thing2": {Value: "今日NBA阵容推荐"},
		"thing1": {Value: now},
		"thing3": {Value: fmt.Sprintf("预测战力：%.0f", totalPower) + "\n点击查看推荐阵容明细"},
	}

	log.Printf("[wechat] send lineup notify: openID=%s templateID=%s page=%s data=%v", openID, templateID, page, data)
	return c.SendSubscribe(openID, templateID, page, data)
}
