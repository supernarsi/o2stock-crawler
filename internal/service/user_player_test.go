package service

import (
	"context"
	"o2stock-crawler/internal/db"
	"testing"
	"time"

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

func TestUserFavPlayers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	database, teardown := setupTestDB(t)
	defer teardown()

	ctx := context.Background()
	userSvc := NewUserPlayerService(database)
	playersSvc := NewPlayersService(database)

	userID := uint(99999)   // Test User ID
	playerID := uint(10005) // Assuming this player exists in DB

	// Insert dummy player if not exists
	_, err := database.ExecContext(ctx, `
		INSERT IGNORE INTO players (player_id, p_name_show, p_name_en, team_abbr, version, card_type, player_img, price_standard)
		VALUES (?, 'Test Player', 'Test Player En', 'LAL', 1, 1, 'img', 1000)
	`, playerID)
	if err != nil {
		t.Fatalf("failed to insert dummy player: %v", err)
	}

	// Insert dummy p_p_history if not exists (required for GetPlayersWithPriceChangeByIDs)
	// We insert a record for "now" so it falls within any recent time window
	nowStr := db.FormatDateTimeHour(time.Now())
	_, err = database.ExecContext(ctx, `
		INSERT IGNORE INTO p_p_history (player_id, at_date_hour, price_standard, price_current_sale, c_time)
		VALUES (?, ?, 1000, 1000, NOW())
	`, playerID, nowStr)
	if err != nil {
		t.Fatalf("failed to insert dummy p_p_history: %v", err)
	}

	// Clean up before and after
	cleanup := func() {
		database.ExecContext(ctx, "DELETE FROM u_p_fav WHERE uid = ? AND pid = ?", userID, playerID)
		// Only delete if we are sure it's our test data, but for safety maybe just leave players/history
		// or delete them if we are sure.
		// Given we used INSERT IGNORE, it might have existed.
		// But for 10005 it's likely test data.
		// Let's at least clean up u_p_fav.
	}
	cleanup()
	defer cleanup()

	// 1. Check initially not fav
	favs, err := userSvc.GetUserFavPlayers(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserFavPlayers failed: %v", err)
	}
	for _, p := range favs {
		if p.PlayerID == playerID {
			t.Fatalf("Player should not be fav yet")
		}
	}

	// 2. Add Fav
	err = userSvc.FavPlayer(ctx, userID, playerID)
	if err != nil {
		t.Fatalf("FavPlayer failed: %v", err)
	}

	// 3. Check Fav List
	favs, err = userSvc.GetUserFavPlayers(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserFavPlayers failed: %v", err)
	}
	found := false
	for _, p := range favs {
		if p.PlayerID == playerID {
			found = true
			if !p.IsFav {
				t.Errorf("IsFav should be true in fav list")
			}
			break
		}
	}
	if !found {
		t.Errorf("Player %d not found in fav list", playerID)
	}

	// 4. Check Player List (ListPlayersWithOwned)
	// Try to find the player in a large list
	list, err := playersSvc.ListPlayersWithOwned(ctx, 1, 5000, "", false, 1, &userID, false, "")
	if err != nil {
		t.Fatalf("ListPlayersWithOwned failed: %v", err)
	}

	foundInList := false
	for _, p := range list {
		if p.PlayerID == playerID {
			foundInList = true
			if !p.IsFav {
				t.Errorf("ListPlayersWithOwned: IsFav should be true for %d", playerID)
			}
		}
	}

	if !foundInList {
		t.Logf("Player %d not found in list (might be missing in DB or filtered), skipping IsFav check in List", playerID)
	} else {
		t.Logf("Verified IsFav in player list for %d", playerID)
	}

	// 5. Test Access Control (Simulation)
	// If we pass nil userID to ListPlayersWithOwned, IsFav should be false
	listNoAuth, err := playersSvc.ListPlayersWithOwned(ctx, 1, 5000, "", false, 1, nil, false, "")
	if err != nil {
		t.Fatalf("ListPlayersWithOwned (no auth) failed: %v", err)
	}
	for _, p := range listNoAuth {
		if p.PlayerID == playerID {
			if p.IsFav {
				t.Errorf("IsFav should be false when user is not logged in")
			}
		}
	}
}

func TestUserFavLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	database, teardown := setupTestDB(t)
	defer teardown()

	ctx := context.Background()
	userSvc := NewUserPlayerService(database)
	userID := uint(99998) // Another Test User ID

	// Clean up
	database.ExecContext(ctx, "DELETE FROM u_p_fav WHERE uid = ?", userID)
	defer database.ExecContext(ctx, "DELETE FROM u_p_fav WHERE uid = ?", userID)

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
