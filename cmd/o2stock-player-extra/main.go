package main

import (
	"context"
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

	playerIDs := []uint{}
	if len(os.Args) >= 2 {
		// 支持逗号分隔或空格分隔的 ID
		args := os.Args[1:]
		for _, arg := range args {
			ids := strings.SplitSeq(arg, ",")
			for idStr := range ids {
				idStr = strings.TrimSpace(idStr)
				if idStr == "" {
					continue
				}
				id, err := strconv.Atoi(idStr)
				if err == nil && id > 0 {
					playerIDs = append(playerIDs, uint(id))
				}
			}
		}
	}

	playersService := service.NewPlayersService(database)

	log.Printf(">>> 开始执行球员扩展信息及徽章同步任务 <<<")
	start := time.Now()

	if err := playersService.SyncPlayerExtraAndBadges(ctx, playerIDs); err != nil {
		log.Fatalf("同步失败: %v", err)
	}

	log.Printf(">>> 任务执行完成 <<< 耗时: %v", time.Since(start))
}
