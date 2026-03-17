package utils

import "time"

// IsOl2CrawlerSleepTime 检查指定时间是否在 o2stock-crawler-ol2 的禁止抓取时间段（北京时间 02:00~08:00）
func IsOl2CrawlerSleepTime(t time.Time) bool {
	hour := t.In(time.FixedZone("CST", 8*3600)).Hour()
	return hour >= 2 && hour < 8
}
