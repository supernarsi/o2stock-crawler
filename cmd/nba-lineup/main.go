package main

import (
	"context"
	"fmt"
	"log"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/entity"
	"o2stock-crawler/internal/service"
	"o2stock-crawler/internal/wechat"
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
	if err := ensureNBALineupTables(database); err != nil {
		log.Fatalf("初始化 NBA 相关表失败: %v", err)
	}

	ctx := context.Background()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	svc := service.NewLineupRecommendService(database)

	switch os.Args[1] {
	case "import":
		// go run ./cmd/nba-lineup import docs/data/
		if len(os.Args) < 3 {
			log.Fatalf("用法: nba-lineup import <dataDir>")
		}
		dataDir := os.Args[2]
		if err := svc.ImportGameData(ctx, dataDir); err != nil {
			log.Fatalf("导入数据失败: %v", err)
		}

	case "recommend":
		// go run ./cmd/nba-lineup recommend 2026-03-04
		if len(os.Args) < 2 { // This condition is technically redundant as os.Args[1] must exist to reach this case
			log.Fatalf("用法: nba-lineup recommend [date]")
		}
		var gameDate time.Time
		var err error
		if len(os.Args) >= 3 {
			gameDateStr, parseErr := parseGameDateArg(os.Args[2])
			if parseErr != nil {
				log.Fatalf("日期参数错误: %v", parseErr)
			}
			gameDate, err = time.Parse("2006-01-02", gameDateStr)
			if err != nil {
				log.Fatalf("日期解析错误: %v", err)
			}
		} else {
			gameDate = time.Now().AddDate(0, 0, 1) // 默认明天
		}
		if err := svc.GenerateRecommendation(ctx, gameDate.Format("2006-01-02")); err != nil {
			log.Fatalf("推荐失败: %v", err)
		}

	case "backtest":
		// go run ./cmd/nba-lineup backtest 2026-03-04
		if len(os.Args) < 3 {
			log.Fatalf("用法: nba-lineup backtest <gameDate>")
		}
		gameDate, err := parseGameDateArg(os.Args[2])
		if err != nil {
			log.Fatalf("日期参数错误: %v", err)
		}
		if err := svc.RunBacktest(ctx, gameDate, 3); err != nil {
			log.Fatalf("回测失败: %v", err)
		}

	case "notify":
		// go run ./cmd/nba-lineup notify
		wxCfg := config.LoadWechatConfigFromEnv()
		wxClient := wechat.NewClient(wxCfg)
		notifySvc := service.NewLineupNotifyService(database, wxClient)
		if err := notifySvc.RunDailyNotify(ctx); err != nil {
			log.Fatalf("阵容推送失败: %v", err)
		}

	default:
		log.Printf("未知命令: %s", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	log.Println("用法: nba-lineup <command> [args]")
	log.Println()
	log.Println("命令:")
	log.Println("  import <dataDir>      兼容旧 JSON 导入；会同时回填 nba_player_salary")
	log.Println("                        dataDir 下需要有 today_nba_total_prepare.json")
	log.Println("  recommend [date]      自动抓官方赛程并生成指定日期推荐阵容（默认明日）")
	log.Println("  backtest <date>       基于 player_game_stats 回测并输出真实最优 Top3")
	log.Println("  notify                对已订阅用户发送每日 NBA 阵容推荐推送")
	log.Println()
	log.Println("示例:")
	log.Println("  go run ./cmd/nba-lineup recommend 2026-03-04")
	log.Println("  go run ./cmd/nba-lineup backtest 2026-03-04")
}

func parseGameDateArg(value string) (string, error) {
	gameDate := strings.TrimSpace(value)
	if gameDate == "" {
		return "", fmt.Errorf("gameDate 不能为空")
	}
	if _, err := time.Parse("2006-01-02", gameDate); err != nil {
		return "", fmt.Errorf("gameDate 必须是 YYYY-MM-DD")
	}
	return gameDate, nil
}

func ensureNBALineupTables(database *db.DB) error {
	return database.AutoMigrate(
		&entity.NBAPlayerSalary{},
		&entity.NBAGameInjurySnapshot{},
	)
}
