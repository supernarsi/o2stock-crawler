package main

import (
	"context"
	"log"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/service"
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

	if len(os.Args) < 2 {
		if err := runOnce(ctx, client, database); err != nil {
			log.Fatalf("一次性抓取失败: %v", err)
		}
		return
	}

	command := os.Args[1]
	switch command {
	case "run-once":
		if err := runOnce(ctx, client, database); err != nil {
			log.Fatalf("一次性抓取失败: %v", err)
		}

	case "loop":
		interval := time.Hour
		if len(os.Args) >= 3 {
			if d, err := time.ParseDuration(os.Args[2]); err == nil {
				interval = d
			}
		}
		runLoop(ctx, client, database, interval)

	default:
		log.Printf("未知命令: %s", command)
		os.Exit(1)
	}
}

func runLoop(ctx context.Context, client *crawler.Client, database *db.DB, interval time.Duration) {
	log.Printf("开始循环抓取，间隔: %s", interval)
	for {
		now := time.Now()
		if shouldSkipCrawl(now) {
			nextRun := getNextRunTime(now)
			log.Printf("当前在禁止抓取时段，下次抓取时间: %s", nextRun.Format("15:04:05"))
			time.Sleep(time.Until(nextRun))
			continue
		}

		if err := runOnce(ctx, client, database); err != nil {
			log.Printf("循环抓取失败: %v", err)
		}
		time.Sleep(interval)
	}
}

func runOnce(ctx context.Context, client *crawler.Client, database *db.DB) error {
	// 如果当前时间在 03:00 ~ 08:00 之间，则不抓取球员数据
	if time.Now().Hour() >= 3 && time.Now().Hour() < 8 {
		log.Printf(">>> 当前时间在 03:00 ~ 08:00 之间，不抓取球员数据 <<<")
		return nil
	}

	log.Printf(">>> 开始按球队抓取球员数据，共 %d 支球队 <<<", len(consts.AllCrawlTeamIDs))
	snapshotService := service.NewSnapshotService(database)

	for i, teamId := range consts.AllCrawlTeamIDs {
		teamName := consts.TeamIDToName[teamId]
		if teamName == "" {
			teamName = "未知"
		}

		// teamId=501（自由球员）仅抓取第一页；其他球队最多抓取 2 页
		maxPages := 1
		if teamId != consts.TeamIDFreeAgent {
			maxPages = 2
		}

		if err := fetchTeamRoster(ctx, client, snapshotService, teamId, teamName, maxPages); err != nil {
			log.Printf("抓取球队 %s(teamId=%d) 失败: %v", teamName, teamId, err)
			return err
		}

		// 完成当前球队后暂停 10 秒再抓取下一支球队
		if i < len(consts.AllCrawlTeamIDs)-1 {
			log.Printf("等待 10 秒后抓取下一支球队...")
			time.Sleep(10 * time.Second)
		}
	}

	log.Printf(">>> 按球队抓取球员数据完成 <<<")

	playersService := service.NewPlayersService(database)

	// 同步涨跌幅逻辑：在抓取完成后执行
	if err := playersService.SyncAllPlayersPriceChanges(ctx); err != nil {
		log.Printf("同步球员涨跌幅失败: %v", err)
	}

	// 战力值更新逻辑：仅在 15:00 ~ 16:00 之间执行
	if isPowerCalculationWindow(time.Now()) {
		if err := playersService.CalculateAndSyncPower(ctx); err != nil {
			log.Printf("同步球员战力值失败: %v", err)
		}
	}

	return nil
}

// fetchTeamRoster 抓取指定球队的球员数据，最多抓取 maxPages 页；根据 hasMore 决定是否翻页。
func fetchTeamRoster(ctx context.Context, client *crawler.Client, snapshotService *service.SnapshotService, teamId int, teamName string, maxPages int) error {
	log.Printf("--> 开始抓取球队: %s (teamId=%d)，最多 %d 页 <--", teamName, teamId, maxPages)

	for page := 1; page <= maxPages; page++ {
		resp, err := client.FetchRoster(ctx, teamId, page)
		if err != nil {
			return err
		}

		rosterList := client.ParseRosterItemList(resp.Data.RosterList)
		hasMore := resp.Data.HasMore

		log.Printf("抓取 %s 第 %d 页成功，球员数量: %d，hasMore: %v", teamName, page, len(rosterList), hasMore)

		now := time.Now()
		if err := snapshotService.SaveSnapshot(ctx, rosterList, now); err != nil {
			log.Printf("保存 %s 第 %d 页球员数据失败: %v", teamName, page, err)
			return err
		}
		log.Printf("保存 %s 第 %d 页球员数据成功，时间: %s，球员数量: %d", teamName, page, now.Format(time.RFC3339), len(rosterList))

		if !hasMore || page >= maxPages {
			break
		}
		log.Println("------------------------------")
	}
	return nil
}

// shouldSkipCrawl 检查当前时间是否在禁止抓取的时间段（03:00~08:00）
func shouldSkipCrawl(t time.Time) bool {
	hour := t.Hour()
	return hour >= 3 && hour < 8
}

// isPowerCalculationWindow 检查当前时间是否在战力计算的时间窗口（15:00 ~ 16:00）
func isPowerCalculationWindow(t time.Time) bool {
	hour := t.Hour()
	return hour == 15
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
