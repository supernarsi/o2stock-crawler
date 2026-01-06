package main

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/narsihuang/o2stock-crawler/internal/crawler"
	"github.com/narsihuang/o2stock-crawler/internal/db"
)

func main() {
	// Load .env if present (optional for local dev)
	_ = godotenv.Load()

	cfg, err := crawler.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbCfg, err := db.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load db config: %v", err)
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	client := crawler.NewClient(cfg)

	// Simple one-shot run by default
	if len(os.Args) == 1 || os.Args[1] == "run-once" {
		if err := runOnce(client, database); err != nil {
			log.Fatalf("run-once failed: %v", err)
		}
		return
	}

	// Simple loop scheduler: o2stock-crawler loop <minutes>
	if os.Args[1] == "loop" {
		interval := time.Minute * 60
		if len(os.Args) >= 3 {
			if d, err := time.ParseDuration(os.Args[2]); err == nil {
				interval = d
			}
		}

		log.Printf("start loop, interval=%s", interval)
		for {
			if err := runOnce(client, database); err != nil {
				log.Printf("loop run failed: %v", err)
			}
			time.Sleep(interval)
		}
	}
}

func runOnce(client *crawler.Client, database *db.DB) error {
	resp, err := client.FetchRoster()
	if err != nil {
		return err
	}

	// println("resp is:", resp.Data)
	// return nil

	now := time.Now()
	if err := db.SaveSnapshot(database, resp, now); err != nil {
		return err
	}

	log.Printf("saved snapshot at %s, players=%d", now.Format(time.RFC3339), len(resp.Data.RosterList))
	return nil
}
