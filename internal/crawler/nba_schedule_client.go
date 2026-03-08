package crawler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
)

const officialNBAScheduleURL = "https://cdn.nba.com/static/json/staticData/scheduleLeagueV2.json"

// NBAScheduleClient 通过 NBA 官方公开赛程 JSON 获取指定日期赛程。
type NBAScheduleClient struct {
	client *http.Client
}

// NBAScheduleTeam 官方赛程中的球队结构。
type NBAScheduleTeam struct {
	TeamID      uint   `json:"teamId"`
	TeamName    string `json:"teamName"`
	TeamCity    string `json:"teamCity"`
	TeamTricode string `json:"teamTricode"`
	TeamSlug    string `json:"teamSlug"`
}

// NBAScheduleGame 官方赛程中的比赛结构。
type NBAScheduleGame struct {
	GameID         string          `json:"gameId"`
	GameCode       string          `json:"gameCode"`
	GameStatus     int             `json:"gameStatus"`
	GameStatusText string          `json:"gameStatusText"`
	GameDateEst    string          `json:"gameDateEst"`
	GameTimeUTC    string          `json:"gameTimeUTC"`
	HomeTeam       NBAScheduleTeam `json:"homeTeam"`
	AwayTeam       NBAScheduleTeam `json:"awayTeam"`
}

type nbaScheduleResponse struct {
	LeagueSchedule struct {
		GameDates []struct {
			GameDate string            `json:"gameDate"`
			Games    []NBAScheduleGame `json:"games"`
		} `json:"gameDates"`
	} `json:"leagueSchedule"`
}

// NewNBAScheduleClient 创建官方赛程客户端。
func NewNBAScheduleClient() *NBAScheduleClient {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          5,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &NBAScheduleClient{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// GetGamesByDate 获取指定日期的官方赛程。
func (c *NBAScheduleClient) GetGamesByDate(
	ctx context.Context,
	gameDate string,
) ([]NBAScheduleGame, error) {
	dateKey, err := toOfficialScheduleDateKey(gameDate)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, officialNBAScheduleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建官方赛程请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求官方赛程失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("官方赛程返回状态 %d", resp.StatusCode)
	}

	var payload nbaScheduleResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析官方赛程失败: %w", err)
	}

	for _, item := range payload.LeagueSchedule.GameDates {
		if strings.TrimSpace(item.GameDate) == dateKey {
			return item.Games, nil
		}
	}

	return []NBAScheduleGame{}, nil
}

func toOfficialScheduleDateKey(gameDate string) (string, error) {
	dt, err := time.Parse("2006-01-02", strings.TrimSpace(gameDate))
	if err != nil {
		return "", fmt.Errorf("gameDate 必须是 YYYY-MM-DD: %w", err)
	}
	return dt.Format("01/02/2006 00:00:00"), nil
}
