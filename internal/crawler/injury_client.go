package crawler

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// InjuryClient NBA 伤病数据客户端（通过 ESPN 获取最新伤病报告）
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
	Status      string // Out / Day-To-Day
	Description string // 伤病描述
	Date        string // 报告日期 (从 Comment 中提取)
}

// GetInjuryReports 从 ESPN 获取当前所有球员伤病报告
func (c *InjuryClient) GetInjuryReports(ctx context.Context) ([]InjuryReport, error) {
	url := "https://www.espn.com/nba/injuries"

	log.Printf("由 ESPN 获取 NBA 伤病报告: %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN 返回状态 %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 HTML 失败: %w", err)
	}

	var reports []InjuryReport

	// ESPN 的结构：多个 .ResponsiveTable，每个包含一个 team
	doc.Find(".ResponsiveTable").Each(func(i int, s *goquery.Selection) {
		teamName := strings.TrimSpace(s.Find(".Table__Title").Text())

		s.Find("tr.Table__TR").Each(func(j int, tr *goquery.Selection) {
			// 跳过表头和子标题
			if tr.HasClass("Table__TR--subhead") || tr.Find("th").Length() > 0 {
				return
			}

			// 获取列
			tds := tr.Find("td")
			if tds.Length() < 5 {
				return
			}

			playerName := strings.TrimSpace(tds.Eq(0).Find("a").Text())
			if playerName == "" {
				playerName = strings.TrimSpace(tds.Eq(0).Text())
			}

			status := strings.TrimSpace(tds.Eq(3).Text())
			comment := strings.TrimSpace(tds.Eq(4).Text())

			// 从 Comment 中提取日期 (例如 "Mar 2: ...")
			reportDate := ""
			if idx := strings.Index(comment, ":"); idx != -1 {
				reportDate = strings.TrimSpace(comment[:idx])
			}

			reports = append(reports, InjuryReport{
				PlayerName:  playerName,
				TeamName:    teamName,
				Status:      status,
				Description: comment,
				Date:        reportDate,
			})
		})
	})

	log.Printf("从 ESPN 获取到 %d 条伤病报告", len(reports))
	return reports, nil
}

// StatusToAvailabilityScore 将伤病状态转换为出场可用性分数
func StatusToAvailabilityScore(status string) float64 {
	s := strings.ToLower(strings.TrimSpace(status))

	// ESPN 常见状态：Out, Day-To-Day
	// 针对 "Day-To-Day" (每日观察)，通常对应 Questionable/Probable
	if s == "out" {
		return 0.0
	}
	if s == "day-to-day" {
		return 0.5 // 默认为存疑
	}

	// 其他关键字
	if strings.Contains(s, "out") || strings.Contains(s, "injured") || strings.Contains(s, "suspended") {
		return 0.0
	}
	if strings.Contains(s, "questionable") {
		return 0.5
	}
	if strings.Contains(s, "doubtful") {
		return 0.15
	}
	if strings.Contains(s, "probable") || strings.Contains(s, "available") {
		return 1.0
	}

	return 1.0 // 默认为健康
}

// MatchInjuryToPlayer 强化匹配逻辑
func MatchInjuryToPlayer(injuryName string, playerEnName string) bool {
	// 1. 标准化：转小写，移除点(.)，多余空格
	clean := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, ".", " ")
		s = strings.Join(strings.Fields(s), " ")
		return s
	}

	inj := clean(injuryName)
	pl := clean(playerEnName)

	if inj == "" || pl == "" {
		return false
	}

	// 2. 精确匹配
	if inj == pl {
		return true
	}

	// 3. 包含匹配 (例如 "Luka Doncic" vs "Luka Doncic (calf)")
	if strings.Contains(inj, pl) || strings.Contains(pl, inj) {
		return true
	}

	// 4. LastName + FirstName 首字母匹配 (如 "L. Doncic")
	injParts := strings.Fields(inj)
	plParts := strings.Fields(pl)
	if len(injParts) >= 1 && len(plParts) >= 2 {
		// 检查 "L. Doncic" 格式
		if len(injParts[0]) == 1 || (len(injParts[0]) == 2 && injParts[0][1] == '.') {
			if injParts[0][0] == plParts[0][0] && injParts[len(injParts)-1] == plParts[len(plParts)-1] {
				return true
			}
		}
	}

	return false
}
