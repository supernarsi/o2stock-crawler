package main

import (
	"log"
	"net/http"
	"o2stock-crawler/internal/config"
	"o2stock-crawler/internal/controller"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/middleware"
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

	dbCfg, err := db.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load db config: %v", err)
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	apiCtl := controller.NewAPI(database, dbCfg)
	authCtl := controller.NewAuthController(database, dbCfg)

	// 定义全局中间件（Client 最先执行，便于后续从 ctx 获取）
	globalMiddlewares := []middleware.Middleware{
		middleware.ClientMiddleware,
		middleware.CORS,
		middleware.Logging,
		middleware.SignatureMiddleware(dbCfg),
	}

	// 创建路由器并注册路由
	// 公开接口使用 OptionalMiddleware，以便在有 Token 时获取用户信息
	router := middleware.NewRouter(append(globalMiddlewares, authCtl.OptionalMiddleware)...)
	router.RegisterAPI("/healthz", apiCtl.Healthz(), "") // 允许所有方法

	// --- 公开接口 --- //
	router.RegisterAPI("/login", authCtl.Login(), http.MethodPost)
	router.RegisterAPI("/players", apiCtl.Players(), http.MethodGet)
	router.RegisterAPI("/player-history", apiCtl.PlayerHistory(), http.MethodGet)
	router.RegisterAPI("/items", apiCtl.Items(), http.MethodGet)
	router.RegisterAPI("/item-history", apiCtl.ItemHistory(), http.MethodGet)
	router.RegisterAPI("/multi-players-history", apiCtl.MultiPlayersHistory(), http.MethodGet)
	router.RegisterAPI("/players/investment-stats", apiCtl.PlayerInvestmentStats(), http.MethodGet)

	// 内部调试：推送指定用户的球员回本订阅消息（需要 DEBUG=true 且 Header: x-debug=SIGNATURE_DEBUG_KEY）
	router.RegisterAPI("/debug/wechat/breakeven", apiCtl.DebugSendPlayerBreakEvenNotify(), http.MethodPost)

	// --- 需要鉴权的接口 --- //
	authGroup := middleware.NewRouter(append(globalMiddlewares, authCtl.Middleware)...)
	// 标记购买
	authGroup.RegisterAPI("/player/in", apiCtl.PlayerIn(), http.MethodPost)
	// 标记出售
	authGroup.RegisterAPI("/player/out", apiCtl.PlayerOut(), http.MethodPost)
	// 标记购买道具
	authGroup.RegisterAPI("/item/in", apiCtl.ItemIn(), http.MethodPost)
	// 标记出售道具
	authGroup.RegisterAPI("/item/out", apiCtl.ItemOut(), http.MethodPost)
	// 修改持仓记录
	authGroup.RegisterAPI("/u-player/record", apiCtl.PlayerOwnEdit(), http.MethodPut)
	// 删除持仓记录
	authGroup.RegisterAPI("/u-player/record", apiCtl.PlayerOwnDel(), http.MethodDelete)
	// 用户拥有球员列表
	authGroup.RegisterAPI("/u-players", apiCtl.UserPlayers(), http.MethodGet)
	// 用户拥有道具列表
	authGroup.RegisterAPI("/u-items", apiCtl.UserItems(), http.MethodGet)
	// 用户收藏球员列表
	authGroup.RegisterAPI("/u-fav-players", apiCtl.UserFavList(), http.MethodGet)
	// 用户收藏球员
	authGroup.RegisterAPI("/player/fav", apiCtl.UserFavPlayer(), http.MethodPost)
	// 用户取消收藏球员
	authGroup.RegisterAPI("/player/fav", apiCtl.UserUnFavPlayer(), http.MethodDelete)
	// 修改球员价格订阅（回本/盈利通知）
	authGroup.RegisterAPI("/player-price/notify", apiCtl.PlayerPriceNotify(), http.MethodPost)
	// 修改道具价格订阅（回本/盈利通知）
	authGroup.RegisterAPI("/item-price/notify", apiCtl.ItemPriceNotify(), http.MethodPost)
	// 收藏道具
	authGroup.RegisterAPI("/item/fav", apiCtl.FavItem(), http.MethodPost)
	// 取消收藏道具
	authGroup.RegisterAPI("/item/fav", apiCtl.UnFavItem(), http.MethodDelete)
	// 收藏道具列表
	authGroup.RegisterAPI("/u-fav-items", apiCtl.UserFavItems(), http.MethodGet)
	// 统一持仓列表
	authGroup.RegisterAPI("/u-own-goods", apiCtl.UserUnifiedOwnGoods(), http.MethodGet)

	authGroup.RegisterAPI("/ipi/rank", apiCtl.IPIRank(), http.MethodGet)
	authGroup.RegisterAPI("/ipi/player", apiCtl.IPIPlayer(), http.MethodGet)
	authGroup.RegisterAPI("/nba/lineups", apiCtl.NBALineups(), http.MethodGet)

	mux := http.NewServeMux()

	// 监控与测试接口，不需要任何鉴权/签名等中间件，直接挂在 mux 上
	mux.HandleFunc("/", apiCtl.Test())
	mux.HandleFunc("/crawler/healthz", apiCtl.CrawlerHealthz())

	router.Apply(mux)
	authGroup.Apply(mux)

	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("o2stock-api listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
