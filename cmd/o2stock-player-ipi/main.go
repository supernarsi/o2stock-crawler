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

	// 从环境变量加载 IPI 配置（支持外部配置权重、阈值等）
	ipiCfg := config.LoadIPIConfigFromEnv()
	log.Printf("IPI 配置: Season=%s, Weights=%.2f/%.2f/%.2f, HistoryDays=%d",
		ipiCfg.Season, ipiCfg.Weights.SPerf, ipiCfg.Weights.VGap, ipiCfg.Weights.MGrowth, ipiCfg.HistoryDays)

	ipiService := service.NewIPIServiceWithConfig(database, ipiCfg)
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

	taskStatusRepo := repositories.NewTaskStatusRepository(database.DB)
	if err := taskStatusRepo.Upsert(ctx, "o2stock-player-ipi", time.Now()); err != nil {
		log.Printf("更新 o2stock-player-ipi 任务状态失败: %v", err)
	} else {
		log.Printf("更新 o2stock-player-ipi 任务状态成功")
	}

	log.Printf(">>> IPI 计算完成 <<< 耗时: %v, 写入条数: %d, 计算时间: %s",
		time.Since(start), len(results), calculatedAt.Format(time.RFC3339))
}

/*
SQL 语句：获取最近5场场均上场时间增长次数为4次的球员
WITH RecentFive AS (
    -- 1. 严格锁定每个球员最近的 5 场比赛
    SELECT
        tx_player_id,
        minutes,
        game_date,
        ROW_NUMBER() OVER (PARTITION BY tx_player_id ORDER BY game_date DESC) as rn
    FROM player_game_stats
),
StepAnalysis AS (
    -- 2. 仅对这 5 场比赛进行“后一场比前一场”的对比
    -- 注意：这里按日期升序排，对比的是这 5 场内部的连续性
    SELECT
        tx_player_id,
        minutes,
        LAG(minutes) OVER (PARTITION BY tx_player_id ORDER BY game_date ASC) as prev_minutes,
        rn
    FROM RecentFive
    WHERE rn <= 4
)
-- 3. 统计增长次数。5场比赛必须有4次增长，且第5场（最早的那场）prev_minutes 为空是正常的
SELECT
    p.p_name_show AS '球员姓名',
    p.team_abbr AS '球队',
    GROUP_CONCAT(s.minutes ORDER BY s.rn DESC) AS '近5场时长(新->旧)',
    p.price_current_lowest AS '最低售价',
    p.tx_player_id
FROM StepAnalysis s
JOIN players p ON s.tx_player_id = p.tx_player_id
GROUP BY s.tx_player_id, p.p_name_show, p.team_abbr, p.price_current_lowest
HAVING
    SUM(CASE WHEN s.minutes > s.prev_minutes THEN 1 ELSE 0 END) = 3
    AND COUNT(*) = 4;
*/
