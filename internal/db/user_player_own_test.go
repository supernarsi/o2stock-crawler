package db

import (
	"context"
	"testing"
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
	query := NewUserPlayerOwnQuery(1)
	count, err := query.CountOwnedPlayers(ctx, db, 10005)
	if err != nil {
		t.Fatalf("CountOwnedPlayers failed: %v", err)
	}
	t.Logf("Count: %d", count)
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
	query := NewUserPlayerOwnQuery(1)
	players, err := query.GetUserOwnedPlayers(ctx, db)
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
	query := NewUserPlayerOwnQuery(1)
	ownedMap, err := query.GetOwnedInfoByPlayerIDs(ctx, db, playerIDs)
	if err != nil {
		t.Fatalf("GetOwnedInfoByPlayerIDs failed: %v", err)
	}
	t.Logf("Found owned info for %d players", len(ownedMap))
}
