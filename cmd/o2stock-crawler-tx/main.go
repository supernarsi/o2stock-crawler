package main

import (
	"context"
	"log"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/service"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// 1. Load .env file first (runtime config)
	_ = godotenv.Load()

	// 2. Load embedded config (compile-time fallback)
	config.LoadEmbedded()

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

	if len(os.Args) < 2 {
		log.Println("Usage: o2stock-crawler-tx <command> [args]")
		log.Println("Commands:")
		log.Println("  tx-nba [date] [--no-season]  Crawl daily stats and sync player season stats")
		log.Println("  tx-sync-players [teamID]      Sync player/team data from Tencent")
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "tx-nba":
		date := time.Now().Format("2006-01-02")
		flag := 2
		noSeason := false

		// 简单的参数解析
		for i := 2; i < len(os.Args); i++ {
			arg := os.Args[i]
			if arg == "--no-season" {
				noSeason = true
			} else if !strings.HasPrefix(arg, "-") && date == time.Now().Format("2006-01-02") {
				date = arg
				flag = 1
			}
		}

		txService := service.NewTxNBAService(database)
		pids, err := txService.CrawlDailyStats(ctx, date, flag)
		if err != nil {
			log.Fatalf("抓取腾讯 NBA 数据失败: %v", err)
		}

		if !noSeason && len(pids) > 0 {
			if err := txService.SyncPlayerSeasonStats(ctx, pids); err != nil {
				log.Printf("同步球员赛季统计失败: %v", err)
			}
		}

	case "tx-sync-players":
		teamID := ""
		if len(os.Args) >= 3 {
			teamID = os.Args[2]
		}
		txService := service.NewTxNBAService(database)
		if err := txService.SyncPlayers(ctx, teamID); err != nil {
			log.Fatalf("同步腾讯球员数据失败: %v", err)
		}

	default:
		log.Printf("未知命令: %s", command)
		os.Exit(1)
	}
}
