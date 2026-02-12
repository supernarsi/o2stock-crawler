package service

import (
	"context"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"testing"

	"github.com/joho/godotenv"
)

func setupTestDB(t *testing.T) (*db.DB, func()) {
	_ = godotenv.Load("../../.env")

	cfg, err := db.LoadConfigFromEnv()
	if err != nil {
		t.Skipf("skip test: %v", err)
	}

	database, err := db.Open(cfg)
	if err != nil {
		t.Skipf("skip test: %v", err)
	}

	return database, func() { database.Close() }
}

func TestUserFavLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	database, teardown := setupTestDB(t)
	defer teardown()

	ctx := context.Background()
	userSvc := NewUserPlayerService(
		database,
		repositories.NewOwnRepository(database.DB),
		repositories.NewPlayerRepository(database.DB),
		repositories.NewItemRepository(database.DB),
		repositories.NewFavRepository(database.DB),
	)
	userID := uint(99998) // Another Test User ID

	// Clean up
	database.DB.Exec("DELETE FROM u_p_fav WHERE uid = ?", userID)
	defer database.DB.Exec("DELETE FROM u_p_fav WHERE uid = ?", userID)

	// 1. Insert 50 favs
	for i := 0; i < 50; i++ {
		pid := uint(20000 + i)
		err := userSvc.FavPlayer(ctx, userID, pid)
		if err != nil {
			t.Fatalf("failed to insert fav %d: %v", i, err)
		}
	}

	// 2. Try to insert 51st fav
	err := userSvc.FavPlayer(ctx, userID, 30000)
	if err == nil {
		t.Fatal("expected error when exceeding fav limit, got nil")
	}
	if err.Error() != "fav limit exceeded (max 50)" {
		t.Fatalf("expected 'fav limit exceeded (max 50)', got '%v'", err)
	}
}
