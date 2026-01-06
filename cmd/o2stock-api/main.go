package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/narsihuang/o2stock-crawler/internal/db"
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

	mux := http.NewServeMux()
	mux.HandleFunc("/api/players", withCORS(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

		rows, err := db.ListPlayers(ctx, database, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, rows)
	}))

	mux.HandleFunc("/api/player-history", withCORS(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		playerIDStr := r.URL.Query().Get("player_id")
		if playerIDStr == "" {
			http.Error(w, "missing player_id", http.StatusBadRequest)
			return
		}
		id64, err := strconv.ParseUint(playerIDStr, 10, 32)
		if err != nil {
			http.Error(w, "invalid player_id", http.StatusBadRequest)
			return
		}

		limit := parseIntDefault(r.URL.Query().Get("limit"), 200)

		rows, err := db.GetPlayerHistory(ctx, database, uint32(id64), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, rows)
	}))

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

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// withCORS 简单允许所有源，方便本地开发前后端分离。
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}
