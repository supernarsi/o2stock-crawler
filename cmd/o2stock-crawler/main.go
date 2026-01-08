package main

import (
	"log"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present (optional for local dev)
	_ = godotenv.Load()

	cfg, err := crawler.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbCfg, err := db.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load db config: %v", err)
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	client := crawler.NewClient(cfg)

	// Simple one-shot run by default
	if len(os.Args) == 1 || os.Args[1] == "run-once" {
		if err := runOnce(client, database); err != nil {
			log.Fatalf("run-once failed: %v", err)
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

		log.Printf("start loop, interval=%s", interval)
		for {
			now := time.Now()

			// 检查是否在禁止抓取的时间段（03:00~08:00）
			if shouldSkipCrawl(now) {
				nextRun := getNextRunTime(now)
				waitDuration := nextRun.Sub(now)
				log.Printf("current time %s is in skip period (03:00~08:00), next run at %s (wait %s)",
					now.Format("15:04:05"), nextRun.Format("15:04:05"), waitDuration)
				time.Sleep(waitDuration)
				continue
			}

			if err := runOnce(client, database); err != nil {
				log.Printf("loop run failed: %v", err)
			}
			time.Sleep(interval)
		}
	}
}

func runOnce(client *crawler.Client, database *db.DB) error {
	resp, err := client.FetchRoster()
	if err != nil {
		return err
	}

	// println("resp is:", resp.Data)
	// return nil

	now := time.Now()
	if err := db.SaveSnapshot(database, resp, now); err != nil {
		return err
	}

	log.Printf("saved snapshot at %s, players=%d", now.Format(time.RFC3339), len(resp.Data.RosterList))
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
