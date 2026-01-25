package db

import (
	"context"
	"o2stock-crawler/internal/db/repositories"
	"testing"
)

func TestListPlayers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Skipf("skip test: %v", err)
	}

	database, err := Open(cfg)
	if err != nil {
		t.Skipf("skip test: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	query := NewPlayersQuery(database, repositories.PlayerFilter{
		Page:     1,
		Limit:    10,
		OrderBy:  "price_change",
		OrderAsc: true,
		Period:   1,
	})
	players, err := query.ListPlayers(ctx)
	if err != nil {
		t.Fatalf("ListPlayers failed: %v", err)
	}
	t.Logf("Found %d players", len(players))
}

func TestGetPlayersByIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Skipf("skip test: %v", err)
	}

	database, err := Open(cfg)
	if err != nil {
		t.Skipf("skip test: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	playerIDs := []uint{10005, 10006}
	query := NewPlayersQuery(database, repositories.PlayerFilter{})
	players, err := query.GetPlayersByIDs(ctx, playerIDs)
	if err != nil {
		t.Fatalf("GetPlayersByIDs failed: %v", err)
	}
	t.Logf("Found %d players", len(players))
}
