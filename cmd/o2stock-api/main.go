package main

import (
	"log"
	"net/http"
	"o2stock-crawler/internal/controller"
	"o2stock-crawler/internal/db"
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
	mux := http.NewServeMux()
	mux.HandleFunc("/players", apiCtl.Players())
	mux.HandleFunc("/player-history", apiCtl.PlayerHistory())

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
