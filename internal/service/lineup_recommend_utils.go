package service

import (
	"fmt"
	"math"
	"strings"
	"time"

	"o2stock-crawler/internal/entity"
)

// 通用工具与归一化函数。
type teamAlias struct {
	alias string
	code  string
}

var englishTeamAliases = []teamAlias{
	{alias: "los angeles lakers", code: "LAL"},
	{alias: "los angeles clippers", code: "LAC"},
	{alias: "oklahoma city thunder", code: "OKC"},
	{alias: "new orleans pelicans", code: "NOP"},
	{alias: "san antonio spurs", code: "SAS"},
	{alias: "golden state warriors", code: "GSW"},
	{alias: "minnesota timberwolves", code: "MIN"},
	{alias: "portland trail blazers", code: "POR"},
	{alias: "philadelphia 76ers", code: "PHI"},
	{alias: "indiana pacers", code: "IND"},
	{alias: "washington wizards", code: "WAS"},
	{alias: "orlando magic", code: "ORL"},
	{alias: "new york knicks", code: "NYK"},
	{alias: "brooklyn nets", code: "BKN"},
	{alias: "charlotte hornets", code: "CHA"},
	{alias: "cleveland cavaliers", code: "CLE"},
	{alias: "dallas mavericks", code: "DAL"},
	{alias: "denver nuggets", code: "DEN"},
	{alias: "detroit pistons", code: "DET"},
	{alias: "houston rockets", code: "HOU"},
	{alias: "memphis grizzlies", code: "MEM"},
	{alias: "miami heat", code: "MIA"},
	{alias: "milwaukee bucks", code: "MIL"},
	{alias: "phoenix suns", code: "PHX"},
	{alias: "sacramento kings", code: "SAC"},
	{alias: "toronto raptors", code: "TOR"},
	{alias: "utah jazz", code: "UTA"},
	{alias: "atlanta hawks", code: "ATL"},
	{alias: "boston celtics", code: "BOS"},
	{alias: "chicago bulls", code: "CHI"},
	{alias: "trail blazers", code: "POR"},
	{alias: "thunder", code: "OKC"},
	{alias: "lakers", code: "LAL"},
	{alias: "clippers", code: "LAC"},
	{alias: "hawks", code: "ATL"},
	{alias: "nets", code: "BKN"},
	{alias: "celtics", code: "BOS"},
	{alias: "hornets", code: "CHA"},
	{alias: "bulls", code: "CHI"},
	{alias: "cavaliers", code: "CLE"},
	{alias: "cavs", code: "CLE"},
	{alias: "mavericks", code: "DAL"},
	{alias: "nuggets", code: "DEN"},
	{alias: "pistons", code: "DET"},
	{alias: "warriors", code: "GSW"},
	{alias: "rockets", code: "HOU"},
	{alias: "pacers", code: "IND"},
	{alias: "grizzlies", code: "MEM"},
	{alias: "heat", code: "MIA"},
	{alias: "bucks", code: "MIL"},
	{alias: "timberwolves", code: "MIN"},
	{alias: "wolves", code: "MIN"},
	{alias: "pelicans", code: "NOP"},
	{alias: "knicks", code: "NYK"},
	{alias: "magic", code: "ORL"},
	{alias: "sixers", code: "PHI"},
	{alias: "76ers", code: "PHI"},
	{alias: "suns", code: "PHX"},
	{alias: "blazers", code: "POR"},
	{alias: "kings", code: "SAC"},
	{alias: "spurs", code: "SAS"},
	{alias: "raptors", code: "TOR"},
	{alias: "jazz", code: "UTA"},
	{alias: "wizards", code: "WAS"},
}

func normalizeTeamCode(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}

	// 中国队名 -> 英文缩写
	if abbr := convertTeamName(trimmed); abbr != trimmed {
		return strings.ToUpper(strings.TrimSpace(abbr))
	}

	upper := strings.ToUpper(trimmed)
	if len(upper) >= 2 && len(upper) <= 4 {
		return upper
	}

	lower := strings.ToLower(trimmed)
	for _, item := range englishTeamAliases {
		if strings.Contains(lower, item.alias) {
			return item.code
		}
	}

	return upper
}

func normalizePlayerName(name string) string {
	replacer := strings.NewReplacer(
		".", " ",
		"-", " ",
		",", " ",
		"'", "",
		"’", "",
		"(", " ",
		")", " ",
	)
	clean := replacer.Replace(strings.ToLower(strings.TrimSpace(name)))
	return strings.Join(strings.Fields(clean), " ")
}

func calcPowerFromStats(g entity.PlayerGameStats) float64 {
	return float64(g.Points) + 1.2*float64(g.Rebounds) + 1.5*float64(g.Assists) +
		3*float64(g.Steals) + 3*float64(g.Blocks) - float64(g.Turnovers)
}

func calcUsageProxyFromStats(g entity.PlayerGameStats) float64 {
	proxy := float64(g.FieldGoalsAttempted) + 0.44*float64(g.FreeThrowsAttempted) + float64(g.Turnovers)
	if proxy > 0 {
		return proxy
	}
	// 没有命中/出手字段时回退到基础持球代理
	return float64(g.Points) + 0.7*float64(g.Assists) + 0.4*float64(g.Turnovers)
}

func inferSeasonByGameDate(gameDate string) string {
	dt, ok := parseISODate(gameDate)
	if !ok {
		return ""
	}
	startYear := dt.Year()
	if dt.Month() < 10 {
		startYear--
	}
	return fmt.Sprintf("%d-%02d", startYear, (startYear+1)%100)
}

func parseISODate(value string) (time.Time, bool) {
	dt, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return normalizeDateOnly(dt), true
}

func isHistoricalGameDate(gameDate string) bool {
	targetDate, ok := parseISODate(gameDate)
	if !ok {
		return false
	}
	now := normalizeDateOnly(time.Now())
	return targetDate.Before(now)
}

func normalizeDateOnly(dt time.Time) time.Time {
	y, m, d := dt.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func clamp(val, minVal, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func roundTo(val float64, precision int) float64 {
	p := math.Pow10(precision)
	return math.Round(val*p) / p
}

func padRight(s string, length int) string {
	runeStr := []rune(s)
	// CJK 字符占 2 个宽度
	width := 0
	for _, r := range runeStr {
		if r > 127 {
			width += 2
		} else {
			width++
		}
	}
	if width >= length {
		return s
	}
	return s + strings.Repeat(" ", length-width)
}
