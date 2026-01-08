package db

import (
	"context"
	"database/sql"
	"time"

	"o2stock-crawler/internal/model"
)

// CountOwnedPlayers 统计用户拥有的指定球员数量（状态为 1：已购买）
func CountOwnedPlayers(ctx context.Context, database *DB, userID, playerID uint) (int, error) {
	const q = `
SELECT COUNT(*) 
FROM u_p_own 
WHERE user_id = ? AND player_id = ? AND own_sta = 1`

	var count int
	err := database.QueryRowContext(ctx, q, userID, playerID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// InsertPlayerOwn 插入一条购买记录
func InsertPlayerOwn(ctx context.Context, database *DB, userID, playerID, num, cost uint, dt time.Time) error {
	const q = `
INSERT INTO u_p_own 
	(user_id, player_id, own_sta, price_in, num_in, dt_in)
VALUES (?, ?, 1, ?, ?, ?)`

	_, err := database.ExecContext(ctx, q, userID, playerID, cost, num, dt)
	return err
}

// UpdatePlayerOwnToSold 将已购买的球员标记为已出售
func UpdatePlayerOwnToSold(ctx context.Context, database *DB, userID, playerID, cost uint, dt time.Time) error {
	const q = `
UPDATE u_p_own 
SET own_sta = 2, price_out = ?, dt_out = ?
WHERE user_id = ? AND player_id = ? AND own_sta = 1
LIMIT 1`

	result, err := database.ExecContext(ctx, q, cost, dt, userID, playerID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNoRows
	}
	return nil
}

// GetUserOwnedPlayers 获取用户拥有的所有球员记录（包括已出售）
func GetUserOwnedPlayers(ctx context.Context, database *DB, userID uint) ([]*model.UserPlayerOwn, error) {
	const q = `
SELECT id, user_id, player_id, own_sta, price_in, price_out, num_in, dt_in, dt_out
FROM u_p_own
WHERE user_id = ?
ORDER BY dt_in DESC`

	rows, err := database.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.UserPlayerOwn
	for rows.Next() {
		var r model.UserPlayerOwn
		var dtOut sql.NullTime
		err := rows.Scan(
			&r.ID,
			&r.UserID,
			&r.PlayerID,
			&r.OwnSta,
			&r.PriceIn,
			&r.PriceOut,
			&r.NumIn,
			&r.DtIn,
			&dtOut,
		)
		if err != nil {
			return nil, err
		}
		if dtOut.Valid {
			r.DtOut = &dtOut.Time
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetOwnedInfoByPlayerIDs 根据球员 ID 列表获取用户的拥有信息（仅状态为 1 或 2 的）
func GetOwnedInfoByPlayerIDs(ctx context.Context, database *DB, userID uint, playerIDs []uint) (map[uint][]*model.OwnInfo, error) {
	if len(playerIDs) == 0 {
		return make(map[uint][]*model.OwnInfo), nil
	}

	// 构建 IN 查询的占位符
	placeholders := ""
	args := make([]interface{}, 0, len(playerIDs)+1)
	args = append(args, userID)
	for i, pid := range playerIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, pid)
	}

	q := `
SELECT player_id, own_sta, price_in, price_out, num_in, dt_in, dt_out
FROM u_p_own
WHERE user_id = ? AND player_id IN (` + placeholders + `) AND own_sta IN (1, 2)
ORDER BY dt_in DESC`

	rows, err := database.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uint][]*model.OwnInfo)
	for rows.Next() {
		var r model.UserPlayerOwn
		var dtOut sql.NullTime
		err := rows.Scan(
			&r.PlayerID,
			&r.OwnSta,
			&r.PriceIn,
			&r.PriceOut,
			&r.NumIn,
			&r.DtIn,
			&dtOut,
		)
		if err != nil {
			return nil, err
		}
		if dtOut.Valid {
			r.DtOut = &dtOut.Time
		}
		info := r.ToOwnInfo()
		result[r.PlayerID] = append(result[r.PlayerID], &info)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
