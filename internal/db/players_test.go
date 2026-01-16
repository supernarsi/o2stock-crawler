package db

import (
	"context"
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

	db, err := Open(cfg)
	if err != nil {
		t.Skipf("skip test: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	query := NewPlayersQuery(10, 0, "price_change", true)
	players, err := query.ListPlayers(ctx, db, 1, false, "")
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

	db, err := Open(cfg)
	if err != nil {
		t.Skipf("skip test: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	playerIDs := []uint{10005, 10006}
	query := NewPlayersQuery(1, 100, "price_standard", true)
	players, err := query.GetPlayersByIDs(ctx, db, playerIDs, false)
	if err != nil {
		t.Fatalf("GetPlayersByIDs failed: %v", err)
	}
	t.Logf("Found %d players", len(players))
}
