package main

import (
	"context"
	"log"
	"math/rand"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// 1. Load .env file first (runtime config)
	// This will NOT overwrite existing system env vars
	_ = godotenv.Load()

	// 2. Load embedded config (compile-time fallback)
	// This will only set vars that are still missing
	config.LoadEmbedded()

	cfg, err := crawler.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	dbCfg, err := db.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("加载数据库配置失败: %v", err)
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	client := crawler.NewClient(cfg)

	// Simple one-shot run by default
	if len(os.Args) == 1 || os.Args[1] == "run-once" {
		if err := runOnce(ctx, client, database); err != nil {
			log.Fatalf("一次性抓取球员数据失败: %v", err)
		}
		return
	}

	// Simple loop scheduler: o2stock-crawler loop <minutes>
	if os.Args[1] == "loop" {
		interval := time.Minute * 60
		if len(os.Args) >= 3 {
			if d, err := time.ParseDuration(os.Args[2]); err == nil {
				interval = d
			}
		}

		log.Printf("开始循环抓取球员数据，间隔: %s", interval)
		for {
			now := time.Now()

			// 检查是否在禁止抓取的时间段（03:00~08:00）
			if shouldSkipCrawl(now) {
				nextRun := getNextRunTime(now)
				waitDuration := nextRun.Sub(now)
				log.Printf("当前时间 %s 在禁止抓取的时间段（03:00~08:00），下次抓取时间: %s (等待 %s)",
					now.Format("15:04:05"), nextRun.Format("15:04:05"), waitDuration)
				time.Sleep(waitDuration)
				continue
			}

			if err := runOnce(ctx, client, database); err != nil {
				log.Printf("循环抓取球员数据失败: %v", err)
			}
			time.Sleep(interval)
		}
	}
}

func runOnce(ctx context.Context, client *crawler.Client, database *db.DB) error {
	// 如果当前时间在 03:00 ~ 08:00 之间，则不抓取球员数据
	if time.Now().Hour() >= 3 && time.Now().Hour() < 8 {
		log.Printf(">>> 当前时间在 03:00 ~ 08:00 之间，不抓取球员数据 <<<")
		return nil
	}

	hasMore := true
	rosterList := []crawler.RosterItemModel{}
	log.Printf(">>> 开始抓取球员数据 <<<")

	saveSnapshotDb := db.NewSaveSnapshotDb()

	// 从 resp.Data.HasMore 判断是否需要继续抓取，最多抓取 20 页数据，每次抓取间隔随机 2~4 秒
	limit := 20
	for i := 0; i < limit; i++ {
		page := i + 1
		log.Printf("--> 开始抓取第 %d 页球员数据 <--", page)

		resp, err := client.FetchRoster(ctx, page)
		if err != nil {
			log.Printf("抓取球员数据失败: %v", err)
			return err
		}

		rosterList = client.ParseRosterItemList(resp.Data.RosterList)
		hasMore = resp.Data.HasMore

		log.Printf("抓取第 %d 页球员数据成功，球员数量: %+v，是否还有更多: %+v", page, len(rosterList), hasMore)

		now := time.Now()
		if err := saveSnapshotDb.SaveSnapshot(ctx, database, rosterList, now); err != nil {
			log.Printf("保存球员数据失败: %v", err)
			continue
		}
		log.Printf("保存球员数据成功，时间: %s，球员数量: %d", now.Format(time.RFC3339), len(rosterList))

		if !hasMore {
			break
		}

		if i < limit-1 {
			sleepDuration := time.Duration(rand.Intn(2)+2) * time.Second
			log.Printf("等待 %s 后开始抓取第 %d 页球员数据", sleepDuration, page+1)
			time.Sleep(sleepDuration)
		}
		log.Println("================================")
	}
	log.Printf(">>> 抓取球员数据完成 <<<")

	return nil
}

// shouldSkipCrawl 检查当前时间是否在禁止抓取的时间段（03:00~08:00）
func shouldSkipCrawl(t time.Time) bool {
	hour := t.Hour()
	return hour >= 3 && hour < 8
}

// getNextRunTime 计算下次应该执行的时间
// 如果当前在禁止时间段，返回 08:00；否则返回当前时间（实际不会用到）
func getNextRunTime(now time.Time) time.Time {
	hour := now.Hour()
	if hour >= 3 && hour < 8 {
		// 当前在禁止时间段，返回今天的 08:00
		next := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
		return next
	}
	// 不在禁止时间段，返回当前时间（实际不会用到）
	return now
}
