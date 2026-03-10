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

// calcRecentVersatilityFactor 计算球员近期的多面手因子
// 基于最近 5 场比赛的场均数据
func calcRecentVersatilityFactor(stats []entity.PlayerGameStats, position uint) float64 {
	if len(stats) == 0 {
		return 1.0
	}

	// 取最近 5 场
	limit := min(5, len(stats))
	totalRebounds := 0.0
	totalAssists := 0.0
	totalSteals := 0.0
	totalBlocks := 0.0

	for i := 0; i < limit; i++ {
		totalRebounds += float64(stats[i].Rebounds)
		totalAssists += float64(stats[i].Assists)
		totalSteals += float64(stats[i].Steals)
		totalBlocks += float64(stats[i].Blocks)
	}

	avgRebounds := totalRebounds / float64(limit)
	avgAssists := totalAssists / float64(limit)
	avgSteals := totalSteals / float64(limit)
	avgBlocks := totalBlocks / float64(limit)

	// 前场球员（C/PF）在篮板和盖帽上有优势，给予额外权重
	// 后场球员（PG/SG）在助攻和抢断上有优势，给予额外权重
	positionGroup := normalizePositionGroup(position)

	var versatilityScore float64
	if positionGroup == 0 {
		// 前场球员：重篮板和盖帽
		versatilityScore = avgRebounds*0.4 + avgAssists*0.3 + avgSteals*0.15 + avgBlocks*0.45
	} else {
		// 后场球员：重助攻和抢断
		versatilityScore = avgRebounds*0.25 + avgAssists*0.4 + avgSteals*0.25 + avgBlocks*0.1
	}

	// 标准化：基准线 10 分为 1.0，15 分以上为 1.15，5 分以下为 0.9
	if versatilityScore >= 15 {
		return 1.15
	}
	if versatilityScore <= 5 {
		return 0.9
	}
	return 0.9 + (versatilityScore-5.0)/10.0*0.25
}

// calcVersatilityScore 计算球员的多面手程度
// 多面手球员（约基奇、东契奇、杰伦约翰逊）在篮板、助攻、抢断、盖帽都有贡献
// 前场球员因防守数据（篮板、盖帽）更容易成为多面手
func calcVersatilityScore(g entity.PlayerGameStats) float64 {
	// 基础全面性：篮板 + 助攻 + 抢断 + 盖帽的加权
	// 前场多面手通常在篮板和盖帽上有优势
	// 后场多面手通常在助攻和抢断上有优势
	rebounds := float64(g.Rebounds)
	assists := float64(g.Assists)
	steals := float64(g.Steals)
	blocks := float64(g.Blocks)

	// 多面手基准：5+5+1+1 或 10+8+1+1 等
	// 使用几何平均避免单项过高导致虚高
	contributions := []float64{
		max(0, rebounds-3),   // 篮板贡献（超过 3 个才算）
		max(0, assists-3),    // 助攻贡献（超过 3 个才算）
		max(0, steals-0.5),   // 抢断贡献
		max(0, blocks-0.5),   // 盖帽贡献
	}

	// 计算均衡度：如果各项数据接近，则给予更高评分
	nonZeroCount := 0
	sum := 0.0
	minVal := 999.0
	for _, v := range contributions {
		if v > 0 {
			nonZeroCount++
			sum += v
			if v < minVal {
				minVal = v
			}
		}
	}

	if nonZeroCount < 2 {
		return 0.8 // 单项球员
	}

	// 均衡度因子：最弱项与最强项的比率
	avg := sum / float64(nonZeroCount)
	balanceFactor := minVal / avg
	if balanceFactor > 1 {
		balanceFactor = 1
	}

	// 多面手得分：总和 * 均衡度
	versatilityScore := sum * (0.5 + 0.5*balanceFactor)

	// 标准化到 0.8-1.2 范围
	score := 0.9 + clamp(versatilityScore/15.0, 0.0, 0.3)
	return clamp(score, 0.8, 1.2)
}

// calcExplosivenessFactor 计算球员的爆发力
// 爆发力强的球员：有单场砍下高分的能力（如米切尔、亚历山大）
// 与稳定性因子形成对比
func calcExplosivenessFactor(stats []entity.PlayerGameStats) float64 {
	if len(stats) < 3 {
		return 1.0
	}

	// 计算每场战力
	powers := make([]float64, 0, len(stats))
	for _, stat := range stats[:min(10, len(stats))] {
		power := calcPowerFromStats(stat)
		if power > 0 {
			powers = append(powers, power)
		}
	}

	if len(powers) < 3 {
		return 1.0
	}

	// 计算平均水平和最高水平
	sum := 0.0
	maxPower := 0.0
	for _, p := range powers {
		sum += p
		if p > maxPower {
			maxPower = p
		}
	}
	avg := sum / float64(len(powers))

	// 爆发力 = 最高表现 / 平均表现
	// 比值越高，说明越有爆发力
	explosivenessRatio := maxPower / avg

	// 标准化：比值 1.5 以上为高爆发，1.2 以下为低爆发
	// 映射到 0.9-1.15 范围
	if explosivenessRatio >= 1.5 {
		return 1.15
	}
	if explosivenessRatio <= 1.2 {
		return 0.9
	}
	return 0.9 + (explosivenessRatio-1.2)/0.3*0.25
}

// calcStableFloorFactor 计算稳定球员的保底能力
// 识别维金斯、波杰姆斯基这类"稳定但上限低"的球员
// 这类球员方差小但最大值也不高
func calcStableFloorFactor(stats []entity.PlayerGameStats) float64 {
	if len(stats) < 3 {
		return 1.0
	}

	powers := make([]float64, 0, len(stats))
	for _, stat := range stats[:min(10, len(stats))] {
		power := calcPowerFromStats(stat)
		if power > 0 {
			powers = append(powers, power)
		}
	}

	if len(powers) < 3 {
		return 1.0
	}

	// 计算方差和最大值
	sum := 0.0
	maxPower := 0.0
	for _, p := range powers {
		sum += p
		if p > maxPower {
			maxPower = p
		}
	}
	mean := sum / float64(len(powers))

	variance := 0.0
	for _, p := range powers {
		diff := p - mean
		variance += diff * diff
	}
	variance /= float64(len(powers))
	stddev := math.Sqrt(variance)

	// 稳定球员：方差小 (<5) 且最大值不高 (<50)
	// 这类球员预测值应该打折，因为缺乏上限
	if stddev < 5 && maxPower < 50 {
		// 稳定但缺乏爆发力，给予轻微惩罚
		return 0.92
	}
	if stddev < 8 && maxPower < 45 {
		return 0.95
	}

	return 1.0
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
