package db

import (
	"context"
	"testing"
	"time"
)

// 注意：这些测试需要真实的数据库连接
// 可以通过环境变量配置测试数据库，或使用测试容器
// 这里提供测试框架，实际运行需要配置数据库

func TestCountOwnedPlayers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	// 需要配置测试数据库
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
	count, err := CountOwnedPlayers(ctx, db, 1, 10005)
	if err != nil {
		t.Fatalf("CountOwnedPlayers failed: %v", err)
	}
	t.Logf("Count: %d", count)
}

func TestInsertPlayerOwn(t *testing.T) {
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
	now := time.Now()
	err = InsertPlayerOwn(ctx, db, 1, 10005, 64, 1021310, now)
	if err != nil {
		t.Fatalf("InsertPlayerOwn failed: %v", err)
	}
}

func TestUpdatePlayerOwnToSold(t *testing.T) {
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
	now := time.Now()
	err = UpdatePlayerOwnToSold(ctx, db, 1, 10005, 1200000, now)
	if err != nil {
		if err == ErrNoRows {
			t.Log("No rows to update (expected if player not owned)")
		} else {
			t.Fatalf("UpdatePlayerOwnToSold failed: %v", err)
		}
	}
}

func TestGetUserOwnedPlayers(t *testing.T) {
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
	players, err := GetUserOwnedPlayers(ctx, db, 1)
	if err != nil {
		t.Fatalf("GetUserOwnedPlayers failed: %v", err)
	}
	t.Logf("Found %d owned players", len(players))
}

func TestGetOwnedInfoByPlayerIDs(t *testing.T) {
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
	ownedMap, err := GetOwnedInfoByPlayerIDs(ctx, db, 1, playerIDs)
	if err != nil {
		t.Fatalf("GetOwnedInfoByPlayerIDs failed: %v", err)
	}
	t.Logf("Found owned info for %d players", len(ownedMap))
}
