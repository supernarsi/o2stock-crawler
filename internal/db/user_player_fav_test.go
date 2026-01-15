package db

import (
	"context"
	"testing"
)

func TestFavPlayerOperations(t *testing.T) {
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
	userID := uint(999999)   // Test user ID
	playerID := uint(888888) // Test player ID

	// 1. Ensure clean state
	_ = DeleteFavPlayer(ctx, db, userID, playerID)

	// 2. Insert Fav
	err = InsertFavPlayer(ctx, db, userID, playerID)
	if err != nil {
		t.Fatalf("InsertFavPlayer failed: %v", err)
	}

	// 3. Count Fav (should be 1)
	count, err := CountFavPlayer(ctx, db, userID, playerID)
	if err != nil {
		t.Fatalf("CountFavPlayer failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// 4. Delete Fav
	err = DeleteFavPlayer(ctx, db, userID, playerID)
	if err != nil {
		t.Fatalf("DeleteFavPlayer failed: %v", err)
	}

	// 5. Count Fav (should be 0)
	count, err = CountFavPlayer(ctx, db, userID, playerID)
	if err != nil {
		t.Fatalf("CountFavPlayer after delete failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after delete, got %d", count)
	}

	// 6. Delete again (should fail with ErrNoRows)
	err = DeleteFavPlayer(ctx, db, userID, playerID)
	if err != ErrNoRows {
		t.Errorf("expected ErrNoRows when deleting non-existent fav, got %v", err)
	}
}
