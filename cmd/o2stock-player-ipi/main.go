package main

import (
	"context"
	"log"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/service"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
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
	ipiService := service.NewIPIService(database)
	ipiRepo := repositories.NewIPIRepository(database.DB)

	log.Printf(">>> 开始执行 IPI 批量计算 <<<")
	start := time.Now()

	results, err := ipiService.BatchCalcIPI(ctx, nil)
	if err != nil {
		log.Fatalf("IPI 批量计算失败: %v", err)
	}

	calculatedAt := time.Now()
	if err := ipiRepo.SaveBatch(ctx, results, calculatedAt); err != nil {
		log.Fatalf("写入 player_ipi 失败: %v", err)
	}

	log.Printf(">>> IPI 计算完成 <<< 耗时: %v, 写入条数: %d, 计算时间: %s",
		time.Since(start), len(results), calculatedAt.Format(time.RFC3339))
}
