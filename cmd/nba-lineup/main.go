package main

import (
	"context"
	"fmt"
	"log"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/service"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const defaultFeedbackActualDir = "docs/data/feedback_actual"

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
		// go run ./cmd/nba-lineup recommend [2026-03-04]
		gameDate := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		if len(os.Args) >= 3 {
			gameDate = os.Args[2]
		}
		if err := svc.GenerateRecommendation(ctx, gameDate); err != nil {
			log.Fatalf("推荐失败: %v", err)
		}

	case "import-actual":
		// go run ./cmd/nba-lineup import-actual 2026-03-04 [docs/data/feedback_actual]
		if len(os.Args) < 3 {
			log.Fatalf("用法: nba-lineup import-actual <gameDate> [feedbackDirOrFile]")
		}
		gameDate, err := parseGameDateArg(os.Args[2])
		if err != nil {
			log.Fatalf("日期参数错误: %v", err)
		}
		feedbackArg := ""
		if len(os.Args) >= 4 {
			feedbackArg = os.Args[3]
		}
		actualFile := resolveFeedbackFilePath(gameDate, feedbackArg)
		log.Printf("读取反馈文件: %s", actualFile)
		if err := svc.ImportActualFeedback(ctx, gameDate, actualFile); err != nil {
			log.Fatalf("导入真实战力反馈失败: %v", err)
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

	case "feedback":
		// go run ./cmd/nba-lineup feedback 2026-03-04 [docs/data/feedback_actual]
		if len(os.Args) < 3 {
			log.Fatalf("用法: nba-lineup feedback <gameDate> [feedbackDirOrFile]")
		}
		gameDate, err := parseGameDateArg(os.Args[2])
		if err != nil {
			log.Fatalf("日期参数错误: %v", err)
		}
		feedbackArg := ""
		if len(os.Args) >= 4 {
			feedbackArg = os.Args[3]
		}
		actualFile := resolveFeedbackFilePath(gameDate, feedbackArg)
		log.Printf("读取反馈文件: %s", actualFile)
		if err := svc.ImportActualFeedback(ctx, gameDate, actualFile); err != nil {
			log.Fatalf("导入真实战力反馈失败: %v", err)
		}
		if err := svc.RunBacktest(ctx, gameDate, 3); err != nil {
			log.Fatalf("回测失败: %v", err)
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
	log.Println("  import <dataDir>      导入 JSON 数据到 nba_game_player 表")
	log.Println("                        dataDir 下需要有 match_data.json, player_salary.json, team_id.json")
	log.Println("  recommend [date]      生成指定日期推荐阵容（默认明日）")
	log.Println("  import-actual <date> [feedbackDirOrFile]")
	log.Println("                        默认读取 docs/data/feedback_actual/<date>.json")
	log.Println("  backtest <date>       基于真实反馈执行回测并输出真实最优 Top3")
	log.Println("  feedback <date> [feedbackDirOrFile]")
	log.Println("                        一步执行: 导入真实反馈 + 回测（默认目录同上）")
	log.Println()
	log.Println("示例:")
	log.Println("  go run ./cmd/nba-lineup import docs/data/")
	log.Println("  go run ./cmd/nba-lineup recommend 2026-03-04")
	log.Println("  go run ./cmd/nba-lineup feedback 2026-03-04")
	log.Println("  go run ./cmd/nba-lineup feedback 2026-03-04 docs/data/feedback_actual/")
	log.Println("  go run ./cmd/nba-lineup feedback 2026-03-04 /tmp/custom/2026-03-04.json")
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

func resolveFeedbackFilePath(gameDate string, feedbackArg string) string {
	target := strings.TrimSpace(feedbackArg)
	if target == "" {
		target = defaultFeedbackActualDir
	}
	if strings.HasSuffix(strings.ToLower(target), ".json") {
		return target
	}
	return filepath.Join(target, gameDate+".json")
}
