package crawler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// InjuryClient NBA 伤病数据客户端（通过 NBA CDN 获取伤病快照）
type InjuryClient struct {
	client *http.Client
}

// NewInjuryClient 创建新的伤病数据客户端
func NewInjuryClient() *InjuryClient {
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

	return &InjuryClient{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// InjuryReport 伤病报告
type InjuryReport struct {
	PlayerName  string // 球员英文名 (e.g. "LeBron James")
	TeamName    string // 球队英文名 (e.g. "Los Angeles Lakers")
	TeamAbbr    string // 球队缩写 (e.g. "LAL")
	Status      string // Out / Doubtful / Questionable / Probable / Available
	Description string // 伤病描述 (e.g. "Right Knee Soreness")
	Date        string // 报告日期
}

// nbaInjuryResponse NBA CDN 伤病快照 JSON 结构
type nbaInjuryResponse struct {
	LeagueName      string `json:"LeagueName"`
	SeasonYear      string `json:"SeasonYear"`
	LastUpdatedTime string `json:"LastUpdatedTime"`
	Teams           []struct {
		TeamID   int    `json:"TeamId"`
		TeamName string `json:"TeamName"`
		TeamCity string `json:"TeamCity"`
		TeamAbbr string `json:"TeamTricode"`
		Players  []struct {
			PlayerID    int    `json:"PlayerId"`
			PlayerName  string `json:"PlayerName"`
			Status      string `json:"Status"`
			Description string `json:"Comment"`
			Date        string `json:"Date"`
		} `json:"Players"`
	} `json:"Teams"`
}

// GetInjuryReports 获取当前所有球员伤病报告
// 从 NBA 官方 CDN 伤病端点获取数据
func (c *InjuryClient) GetInjuryReports(ctx context.Context) ([]InjuryReport, error) {
	// NBA 官方 CDN 伤病快照端点
	url := "https://cdn.nba.com/static/json/liveData/injuries/injuries_AllTeams.json"

	log.Printf("获取 NBA 伤病报告: %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.nba.com/")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NBA CDN 返回状态 %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var nbaResp nbaInjuryResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&nbaResp); err != nil {
		return nil, fmt.Errorf("解析伤病数据失败: %w", err)
	}

	var reports []InjuryReport
	for _, team := range nbaResp.Teams {
		teamFullName := fmt.Sprintf("%s %s", team.TeamCity, team.TeamName)
		for _, player := range team.Players {
			reports = append(reports, InjuryReport{
				PlayerName:  player.PlayerName,
				TeamName:    teamFullName,
				TeamAbbr:    team.TeamAbbr,
				Status:      player.Status,
				Description: player.Description,
				Date:        player.Date,
			})
		}
	}

	log.Printf("获取到 %d 条伤病报告", len(reports))
	return reports, nil
}

// StatusToAvailabilityScore 将伤病状态转换为出场可用性分数
func StatusToAvailabilityScore(status string) float64 {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "available", "active", "probable":
		return 1.0
	case "questionable":
		return 0.5
	case "doubtful":
		return 0.15
	case "out", "injured", "not with team", "rest", "suspended", "traded":
		return 0.0
	default:
		return 1.0 // 未知状态默认可用
	}
}

// MatchInjuryToPlayer 尝试将伤病报告匹配到球员英文名
// 先精确匹配，失败后按 LastName 匹配
func MatchInjuryToPlayer(injuryName string, playerEnName string) bool {
	// 统一转小写比较
	injuryLower := strings.ToLower(strings.TrimSpace(injuryName))
	playerLower := strings.ToLower(strings.TrimSpace(playerEnName))

	if injuryLower == "" || playerLower == "" {
		return false
	}

	// 1. 精确匹配
	if injuryLower == playerLower {
		return true
	}

	// 2. 尝试 Last, First 格式匹配 "James, LeBron" -> "LeBron James"
	if strings.Contains(injuryLower, ",") {
		parts := strings.SplitN(injuryLower, ",", 2)
		if len(parts) == 2 {
			reversed := strings.TrimSpace(parts[1]) + " " + strings.TrimSpace(parts[0])
			if reversed == playerLower {
				return true
			}
		}
	}

	// 3. LastName 匹配（如果名字有多个部分，取最后一个单词）
	injuryParts := strings.Fields(injuryLower)
	playerParts := strings.Fields(playerLower)
	if len(injuryParts) > 0 && len(playerParts) > 0 {
		injuryLast := injuryParts[len(injuryParts)-1]
		playerLast := playerParts[len(playerParts)-1]
		if injuryLast == playerLast && len(injuryParts) > 1 && len(playerParts) > 1 {
			// LastName 相同且首字母相同
			if injuryParts[0][0] == playerParts[0][0] {
				return true
			}
		}
	}

	return false
}
