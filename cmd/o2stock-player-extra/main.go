package main

import (
	"context"
	"fmt"
	"log"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/service"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// 1. Load .env file first
	_ = godotenv.Load()

	// 2. Load embedded config
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
	playersService := service.NewPlayersService(database)

	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			printUsage()
			return
		case "import-detail-json":
			dataDir := "data"
			if len(args) >= 2 && strings.TrimSpace(args[1]) != "" {
				dataDir = strings.TrimSpace(args[1])
			}

			start := time.Now()
			log.Printf(">>> 开始导入球员 detail_json，目录: %s <<<", dataDir)
			stats, err := playersService.ImportPlayerDetailJSONFromDir(ctx, dataDir)
			if err != nil {
				log.Fatalf("导入 detail_json 失败: %v", err)
			}
			log.Printf(">>> detail_json 导入完成 <<< updated=%d not_found=%d invalid=%d 耗时=%v", stats.Updated, stats.NotFound, stats.Invalid, time.Since(start))
			return
		}
	}

	// 默认执行同步；兼容两种输入：
	// 1) o2stock-player-extra sync [id1,id2 ...]
	// 2) o2stock-player-extra [id1,id2 ...]
	var playerIDArgs []string
	if len(args) > 0 && args[0] == "sync" {
		playerIDArgs = args[1:]
	} else {
		playerIDArgs = args
	}

	playerIDs, err := parsePlayerIDs(playerIDArgs)
	if err != nil {
		printUsage()
		log.Fatalf("参数解析失败: %v", err)
	}

	log.Printf(">>> 开始执行球员扩展信息及徽章同步任务 <<<")
	start := time.Now()

	if err := playersService.SyncPlayerExtraAndBadges(ctx, playerIDs); err != nil {
		log.Fatalf("同步失败: %v", err)
	}

	log.Printf(">>> 任务执行完成 <<< 耗时: %v", time.Since(start))
}

func parsePlayerIDs(args []string) ([]uint, error) {
	playerIDs := make([]uint, 0)
	for _, arg := range args {
		ids := strings.SplitSeq(arg, ",")
		for idStr := range ids {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			id, err := strconv.Atoi(idStr)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("非法球员 ID: %q", idStr)
			}
			playerIDs = append(playerIDs, uint(id))
		}
	}
	return playerIDs, nil
}

func printUsage() {
	log.Println("用法: o2stock-player-extra [command] [args]")
	log.Println()
	log.Println("命令:")
	log.Println("  import-detail-json [dataDir]   导入 dataDir 中的 detail_json 到 players.detail_json（默认 data）")
	log.Println("  sync [playerIDs...]            同步球员扩展信息与徽章（支持逗号分隔或空格分隔）")
	log.Println()
	log.Println("兼容写法:")
	log.Println("  o2stock-player-extra 1001,1002 1003")
}
