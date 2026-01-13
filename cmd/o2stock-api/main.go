package main

import (
	"log"
	"net/http"
	"o2stock-crawler/internal/controller"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/middleware"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

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

	// 定义全局中间件
	globalMiddlewares := []middleware.Middleware{
		middleware.CORS,
		middleware.Logging,
	}

	// 创建路由器并注册路由
	router := middleware.NewRouter(globalMiddlewares...)
	router.RegisterAPI("/healthz", apiCtl.Healthz(), "") // 允许所有方法
	router.RegisterAPI("/players", apiCtl.Players(), http.MethodGet)
	router.RegisterAPI("/player-history", apiCtl.PlayerHistory(), http.MethodGet)
	router.RegisterAPI("/multi-players-history", apiCtl.MultiPlayersHistory(), http.MethodGet)
	router.RegisterAPI("/player/in", apiCtl.PlayerIn(), http.MethodPost)
	router.RegisterAPI("/player/out", apiCtl.PlayerOut(), http.MethodPost)
	router.RegisterAPI("/u-players", apiCtl.UserPlayers(), http.MethodGet)
	router.RegisterAPI("/player/fav", apiCtl.UserFavPlayer(), http.MethodPost)

	mux := http.NewServeMux()
	router.Apply(mux)

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
