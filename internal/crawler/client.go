package crawler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// Client wraps HTTP calls to the OL2 API.
type Client struct {
	cfg    *Config
	client *http.Client
}

// NewClient creates a new Client with sane defaults.
func NewClient(cfg *Config) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	return &Client{
		cfg:    cfg,
		client: httpClient,
	}
}

// APIResponse is the top-level response from OL2 API.
type APIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		RosterList []RosterItem `json:"rosterList"`
		HasMore    bool         `json:"hasMore"`
	} `json:"data"`
}

// 接口返回的原始数据类型
type RosterItem struct {
	PlayerID    string `json:"playerId" dc:"球员ID"`
	Grade       string `json:"grade" dc:"等级"`
	ShowName    string `json:"showName" dc:"展示名称"`
	PlayerEn    string `json:"PlayerEnName" dc:"球员英文名称"`
	PlayerImg   string `json:"playerImg" dc:"球员图片"`
	TeamAbbr    string `json:"teamAbbr" dc:"球队"`
	CardTypeStr string `json:"cardType" dc:"系列: 1.现役 2.复刻 3.历史 4.自建 5.收藏"`
	VersionStr  string `json:"Version" dc:"球员年代，0 表示现役"`
	OverAll     int    `json:"overAll" dc:"球员能力值"`
	Price       struct {
		StandardPrice      int    `json:"standardPrice" dc:"标准价格"`
		CurrentLowestPrice string `json:"currentLowestPrice" dc:"当前出售的最低价格"`
		LowerPriceForSale  int    `json:"lowerPriceForSale" dc:"最低可售价"`
		UpperPriceForSale  int    `json:"upperPriceForSale" dc:"最高可售价"`
		Popularity         string `json:"popularity" dc:"人气"`
	} `json:"price" dc:"价格"`
}

// 类型转换后的数据类型
type RosterItemModel struct {
	PlayerID    uint   `json:"playerId" dc:"球员ID"`
	Grade       uint8  `json:"grade" dc:"等级"`
	ShowName    string `json:"showName" dc:"展示名称"`
	PlayerEn    string `json:"PlayerEnName" dc:"球员英文名称"`
	PlayerImg   string `json:"playerImg" dc:"球员图片"`
	TeamAbbr    string `json:"teamAbbr" dc:"球队"`
	CardTypeStr string `json:"cardType" dc:"系列: 1.现役 2.复刻 3.历史 4.自建 5.收藏"`
	VersionStr  string `json:"Version" dc:"球员年代，0 表示现役"`
	OverAll     int    `json:"overAll" dc:"球员能力值"`
	Price       struct {
		StandardPrice      int    `json:"standardPrice" dc:"标准价格"`
		CurrentLowestPrice string `json:"currentLowestPrice" dc:"当前出售的最低价格"`
		LowerPriceForSale  int    `json:"lowerPriceForSale" dc:"最低可售价"`
		UpperPriceForSale  int    `json:"upperPriceForSale" dc:"最高可售价"`
		Popularity         string `json:"popularity" dc:"人气"`
	} `json:"price" dc:"价格"`
}

// FetchRoster fetches current roster data from OL2 API by team.
// teamId 指定球队；page 为页码；lowPrice 固定为 0。
func (c *Client) FetchRoster(ctx context.Context, teamId int, page int) (*APIResponse, error) {
	ts := time.Now().UnixMilli()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.cfg.BaseURL,
		nil,
	)
	if err != nil {
		return nil, err
	}

	nonceStr := c.generateNonceStr(5)

	q := req.URL.Query()
	q.Set("openid", c.cfg.OpenID)            // 用户 openid
	q.Set("access_token", c.cfg.AccessToken) // 用户 access_token
	q.Set("teamId", strconv.Itoa(teamId))    // 指定球队
	q.Set("page", strconv.Itoa(page))        // 页码
	q.Set("orderBy", "price")                // 排序方式: price 价格，grade 等级，popularity 人气
	q.Set("orientation", "desc")             // 排序方向: desc 降序，asc 升序
	q.Set("cardType", "1")                   // 筛选系列: 1.现役 2.复刻 3.历史 4.自建 5.收藏
	q.Set("badges", "-1")                    // 筛选徽章
	q.Set("grade", "1")                      // 筛选突破等级
	q.Set("lowPrice", "0")                   // 固定为 0
	q.Set("highPrice", "0")                  // 筛选最高价格，0 表示不限制
	q.Set("collapseAll", "false")
	q.Set("versionLabel", "")
	q.Set("source", "trade")
	q.Set("timeStamp", fmt.Sprintf("%d", ts))
	q.Set("nonseStr", nonceStr)
	q.Set("sign", c.generateSign(nonceStr, ts))
	req.URL.RawQuery = q.Encode()

	log.Printf("请求球员数据接口 teamId=%d page=%d", teamId, page)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求球员数据接口失败，状态码: %s", resp.Status)
	}

	var out APIResponse
	// 使用 jsoniter 解析
	if err := jsoniter.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("解析球员数据失败: %s", err.Error())
	}

	if out.Code != 0 {
		return nil, fmt.Errorf("请求球员数据接口失败，错误码: %d 错误信息: %s", out.Code, out.Msg)
	}
	log.Printf("抓取球员数据成功，球员数量: %+v", len(out.Data.RosterList))

	return &out, nil
}

func (c *Client) generateNonceStr(length int) string {
	const nonceChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, length)
	for i := range b {
		b[i] = nonceChars[rand.Intn(len(nonceChars))]
	}
	return string(b)
}

func (c *Client) generateSign(nonceStr string, timestamp int64) string {
	data := nonceStr + strconv.FormatInt(timestamp, 10)

	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (c *Client) parseRosterItem(item RosterItem) RosterItemModel {
	playerID, _ := strconv.Atoi(item.PlayerID)
	grade, _ := strconv.Atoi(item.Grade)
	return RosterItemModel{
		PlayerID:    uint(playerID),
		Grade:       uint8(grade),
		ShowName:    item.ShowName,
		PlayerEn:    item.PlayerEn,
		PlayerImg:   item.PlayerImg,
		TeamAbbr:    item.TeamAbbr,
		CardTypeStr: item.CardTypeStr,
		VersionStr:  item.VersionStr,
		Price:       item.Price,
		OverAll:     item.OverAll,
	}
}

func (c *Client) ParseRosterItemList(items []RosterItem) []RosterItemModel {
	rosterList := make([]RosterItemModel, len(items))
	for i, item := range items {
		rosterList[i] = c.parseRosterItem(item)
	}
	return rosterList
}
