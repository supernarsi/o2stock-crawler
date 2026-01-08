package db

import (
	"context"
	"testing"
)

func TestGetPlayerHistory(t *testing.T) {
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
	history, err := GetPlayerHistory(ctx, db, 10005, 10)
	if err != nil {
		t.Fatalf("GetPlayerHistory failed: %v", err)
	}
	t.Logf("Found %d history records", len(history))
}
