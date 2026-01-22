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

	apiCtl := controller.NewAPI(database)
	authCtl := controller.NewAuthController(database, dbCfg)

	// 定义全局中间件
	globalMiddlewares := []middleware.Middleware{
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
	router.RegisterAPI("/multi-players-history", apiCtl.MultiPlayersHistory(), http.MethodGet)

	// --- 需要鉴权的接口 --- //
	authGroup := middleware.NewRouter(append(globalMiddlewares, authCtl.Middleware)...)
	// 标记购买
	authGroup.RegisterAPI("/player/in", apiCtl.PlayerIn(), http.MethodPost)
	// 标记出售
	authGroup.RegisterAPI("/player/out", apiCtl.PlayerOut(), http.MethodPost)
	// 修改持仓记录
	authGroup.RegisterAPI("/u-player/record", apiCtl.PlayerOwnEdit(), http.MethodPut)
	// 删除持仓记录
	authGroup.RegisterAPI("/u-player/record", apiCtl.PlayerOwnDel(), http.MethodDelete)
	// 用户拥有球员列表
	authGroup.RegisterAPI("/u-players", apiCtl.UserPlayers(), http.MethodGet)
	// 用户收藏球员列表
	authGroup.RegisterAPI("/u-fav-players", apiCtl.UserFavList(), http.MethodGet)
	// 用户收藏球员
	authGroup.RegisterAPI("/player/fav", apiCtl.UserFavPlayer(), http.MethodPost)
	// 用户取消收藏球员
	authGroup.RegisterAPI("/player/fav", apiCtl.UserUnFavPlayer(), http.MethodDelete)

	mux := http.NewServeMux()
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
