package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// TxNBAClient wraps HTTP calls to the Tencent Sports NBA API.
type TxNBAClient struct {
	client *http.Client
	config *TxNBAConfig
}

// TxNBAConfig holds URLs for the TxNBAClient.
type TxNBAConfig struct {
	MatchListURL   string
	MatchStatURL   string
	TeamLineupURL  string
	PlayerInfoURL  string
	PlayerStatsURL string
}

// LoadTxNBAConfigFromEnv loads TxNBA API URLs from environment variables.
func LoadTxNBAConfigFromEnv() *TxNBAConfig {
	matchListURL := os.Getenv("TX_NBA_MATCH_LIST_URL")
	if matchListURL == "" {
		matchListURL = "https://app.sports.qq.com/match/list?columnId=100000&unitType=&appvid=&flag=%d&date=%s"
	}

	matchStatURL := os.Getenv("TX_NBA_MATCH_STAT_URL")
	if matchStatURL == "" {
		matchStatURL = "https://app.sports.qq.com/stats/matchStat?mid=%s&appvid=&from=videoApp"
	}

	teamLineupURL := os.Getenv("TX_NBA_TEAM_LINEUP_URL")
	if teamLineupURL == "" {
		teamLineupURL = "https://matchweb.sports.qq.com/match/api/v2/team/lineup?competitionId=100000&teamId=%s"
	}

	playerInfoURL := os.Getenv("TX_NBA_PLAYER_INFO_URL")
	if playerInfoURL == "" {
		playerInfoURL = "https://matchweb.sports.qq.com/playerUtil/playerInfo?competitionId=100000&playerId=%s"
	}

	playerStatsURL := os.Getenv("TX_NBA_PLAYER_STATS_URL")
	if playerStatsURL == "" {
		playerStatsURL = "https://app.sports.qq.com/match/api/v2/player/stats?playerId=%s&competitionId=100000&moduleIds=statList&appvid="
	}

	return &TxNBAConfig{
		MatchListURL:   matchListURL,
		MatchStatURL:   matchStatURL,
		TeamLineupURL:  teamLineupURL,
		PlayerInfoURL:  playerInfoURL,
		PlayerStatsURL: playerStatsURL,
	}
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
		config: LoadTxNBAConfigFromEnv(),
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

// TxPlayerStatsResponse 腾讯体育球员统计数据响应
type TxPlayerStatsResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Modules []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			StatList struct {
				Tabs []struct {
					Name  string `json:"name"`
					Stats []struct {
						Key   string `json:"key"`
						Name  string `json:"name"`
						Value string `json:"value"`
						Rank  int    `json:"rank"`
					} `json:"stats"`
				} `json:"tabs"`
			} `json:"statList"`
		} `json:"modules"`
		Options []struct {
			Key        string `json:"key"`
			Value      string `json:"value"`
			Name       string `json:"name"`
			SubOptions []struct {
				Key     string `json:"key"`
				Value   string `json:"value"`
				Name    string `json:"name"`
				Default bool   `json:"default"`
			} `json:"subOptions"`
			Default bool `json:"default"`
		} `json:"options"`
	} `json:"data"`
}

// GetMatchList 获取指定日期的比赛列表
func (c *TxNBAClient) GetMatchList(ctx context.Context, date string, flag int) (*TxMatchListResponse, error) {
	url := fmt.Sprintf(c.config.MatchListURL, flag, date)

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
	url := fmt.Sprintf(c.config.MatchStatURL, mid)

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
	url := fmt.Sprintf(c.config.TeamLineupURL, teamID)

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

// TxPlayerInfoResponse 腾讯体育球员详情响应（含年龄等基础信息）
type TxPlayerInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		EnName   string `json:"enName"`
		BaseInfo []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"baseInfo"`
		NewBaseInfo []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"newBaseInfo"`
	} `json:"data"`
}

// GetPlayerInfo 获取球员详情（含年龄等基础信息）
func (c *TxNBAClient) GetPlayerInfo(ctx context.Context, txPlayerID string) (*TxPlayerInfoResponse, error) {
	url := fmt.Sprintf(c.config.PlayerInfoURL, txPlayerID)

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

	var out TxPlayerInfoResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

// GetPlayerStats 获取球员统计数据 (含赛季场均)
func (c *TxNBAClient) GetPlayerStats(ctx context.Context, txPlayerID string) (*TxPlayerStatsResponse, error) {
	url := fmt.Sprintf(c.config.PlayerStatsURL, txPlayerID)

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

	var out TxPlayerStatsResponse
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
