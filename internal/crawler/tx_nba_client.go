package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// TxNBAClient wraps HTTP calls to the Tencent Sports NBA API.
type TxNBAClient struct {
	client *http.Client
}

// NewTxNBAClient creates a new TxNBAClient.
func NewTxNBAClient() *TxNBAClient {
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
		Timeout:   15 * time.Second,
		Transport: transport,
	}

	return &TxNBAClient{
		client: httpClient,
	}
}

// TxMatchListResponse 腾讯体育比赛列表响应
type TxMatchListResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Matches map[string]struct {
			List []struct {
				MatchInfo struct {
					MatchType string `json:"matchType"`
					Mid       string `json:"mid"`
					LeftName  string `json:"leftName"`
					RightName string `json:"rightName"`
					StartTime string `json:"startTime"`
					EndTime   string `json:"endTime"`
				} `json:"matchInfo"`
			} `json:"list"`
		} `json:"matches"`
	} `json:"data"`
}

// TxMatchStatResponse 腾讯体育比赛详情统计响应
type TxMatchStatResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TeamInfo struct {
			LeftName  string `json:"leftName" dc:"主队"`
			RightName string `json:"rightName" dc:"客队"`
			LeftID    string `json:"leftId" dc:"主队ID"`
			RightID   string `json:"rightId" dc:"客队ID"`
		} `json:"teamInfo"`
		Stats []struct {
			Type        string          `json:"type" dc:"统计类型:19.球员统计 21.赛况 23.球员PK 22.球队数据 24.投篮热图 37.本场最佳"`
			PlayerStats json.RawMessage `json:"playerStats" dc:"球员统计"`
		} `json:"stats"`
	} `json:"data"`
}

type TxPlayerStatsTeam struct {
	Oncrt []struct {
		PlayerID string   `json:"playerId"`
		Row      []string `json:"row"`
	} `json:"oncrt"`
}

// TxTeamLineupResponse 腾讯体育球队阵容响应
type TxTeamLineupResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		LineUp struct {
			Players []struct {
				ID       string `json:"id"`
				CnName   string `json:"cnName"`
				EnName   string `json:"enName"`
				Logo     string `json:"logo"`
				Position string `json:"position"`
			} `json:"players"`
		} `json:"lineUp"`
	} `json:"data"`
}

// GetMatchList 获取指定日期的比赛列表
func (c *TxNBAClient) GetMatchList(ctx context.Context, date string, flag int) (*TxMatchListResponse, error) {
	url := fmt.Sprintf("https://app.sports.qq.com/match/list?columnId=100000&unitType=&appvid=&flag=%d&date=%s", flag, date)

	log.Printf("获取比赛列表: %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tencent API returned status: %d", resp.StatusCode)
	}

	var out TxMatchListResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

// GetMatchStat 获取指定比赛的统计数据
func (c *TxNBAClient) GetMatchStat(ctx context.Context, mid string) (*TxMatchStatResponse, error) {
	url := fmt.Sprintf("https://app.sports.qq.com/stats/matchStat?mid=%s&appvid=&from=videoApp", mid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tencent API returned status: %d", resp.StatusCode)
	}

	var out TxMatchStatResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

// GetTeamLineup 获取球队球员阵容
func (c *TxNBAClient) GetTeamLineup(ctx context.Context, teamID string) (*TxTeamLineupResponse, error) {
	url := fmt.Sprintf("https://matchweb.sports.qq.com/match/api/v2/team/lineup?competitionId=100000&teamId=%s", teamID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tencent API returned status: %d", resp.StatusCode)
	}

	var out TxTeamLineupResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *TxNBAClient) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://sports.qq.com/")
}
