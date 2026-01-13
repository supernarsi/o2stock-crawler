package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"o2stock-crawler/internal/model"
)

/*
用户拥有球员数据表
```sql
CREATE TABLE `u_p_own` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `uid` int unsigned NOT NULL DEFAULT '0' COMMENT '用户 id',
  `pid` int unsigned NOT NULL DEFAULT '0' COMMENT '球员 id',
  `own_sta` tinyint unsigned NOT NULL DEFAULT '0' COMMENT '状态：0.未拥有；1.已购买；2.已出售',
  `price_in` int unsigned NOT NULL DEFAULT '0' COMMENT '购买时的总价格',
  `price_out` int unsigned NOT NULL DEFAULT '0' COMMENT '出售时的总价格',
  `num_in` int unsigned NOT NULL DEFAULT '0' COMMENT '购买的卡数',
  `dt_in` datetime NOT NULL COMMENT '购买时间',
  `dt_out` datetime DEFAULT NULL COMMENT '出售时间',
  PRIMARY KEY (`id`),
  KEY `idx_uid` (`uid`),
  KEY `idx_pid` (`pid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户拥有球员数据表';
```
*/

// ============================================================================
// UserPlayerOwnQuery 用户球员拥有查询
// ============================================================================

// UserPlayerOwnQuery 用户球员拥有相关查询
type UserPlayerOwnQuery struct {
	QueryBase
	userID uint
}

// NewUserPlayerOwnQuery 创建一个 UserPlayerOwnQuery
func NewUserPlayerOwnQuery(userID uint) *UserPlayerOwnQuery {
	return &UserPlayerOwnQuery{
		QueryBase: QueryBase{},
		userID:    userID,
	}
}

// CountOwnedPlayers 统计用户拥有的指定球员数量（状态为 1：已购买）
func (q *UserPlayerOwnQuery) CountOwnedPlayers(ctx context.Context, database *DB, playerID uint) (int, error) {
	const query = `
SELECT COUNT(*) 
FROM u_p_own 
WHERE uid = ? AND pid = ? AND own_sta = 1`

	var count int
	err := database.QueryRowContext(ctx, query, q.userID, playerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count owned players: %w", err)
	}
	return count, nil
}

// GetUserOwnedPlayers 获取用户拥有的所有球员记录（包括已出售）
func (q *UserPlayerOwnQuery) GetUserOwnedPlayers(ctx context.Context, database *DB) ([]*model.UserPlayerOwn, error) {
	const query = `
SELECT id, uid, pid, own_sta, price_in, price_out, num_in, dt_in, dt_out
FROM u_p_own
WHERE uid = ?
ORDER BY dt_in DESC`

	rows, err := database.QueryContext(ctx, query, q.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user owned players: %w", err)
	}
	defer rows.Close()

	result := make([]*model.UserPlayerOwn, 0)
	for rows.Next() {
		var r model.UserPlayerOwn
		var dtOut sql.NullTime
		if err := scanUserPlayerOwnRow(rows, &r, &dtOut); err != nil {
			return nil, fmt.Errorf("failed to scan user player own row: %w", err)
		}
		if dtOut.Valid {
			r.DtOut = &dtOut.Time
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user player own rows: %w", err)
	}
	return result, nil
}

// GetOwnedInfoByPlayerIDs 根据球员 ID 列表获取用户的拥有信息（仅状态为 1 或 2 的）
func (q *UserPlayerOwnQuery) GetOwnedInfoByPlayerIDs(ctx context.Context, database *DB, playerIDs []uint) (map[uint][]*model.OwnInfo, error) {
	if len(playerIDs) == 0 {
		return make(map[uint][]*model.OwnInfo), nil
	}

	// 构建 IN 查询
	placeholders, args := buildINClause(convertUintToAny(playerIDs))
	args = append([]any{q.userID}, args...)

	query := fmt.Sprintf(`
SELECT pid, own_sta, price_in, price_out, num_in, dt_in, dt_out
FROM u_p_own
WHERE uid = ? AND pid IN (%s) AND own_sta IN (1, 2)
ORDER BY dt_in DESC`, placeholders)

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query owned info by player IDs: %w", err)
	}
	defer rows.Close()

	result := make(map[uint][]*model.OwnInfo)
	for rows.Next() {
		var r model.UserPlayerOwn
		var dtOut sql.NullTime
		if err := rows.Scan(
			&r.PlayerID,
			&r.OwnSta,
			&r.PriceIn,
			&r.PriceOut,
			&r.NumIn,
			&r.DtIn,
			&dtOut,
		); err != nil {
			return nil, fmt.Errorf("failed to scan owned info row: %w", err)
		}
		if dtOut.Valid {
			r.DtOut = &dtOut.Time
		}
		info := r.ToOwnInfo()
		result[r.PlayerID] = append(result[r.PlayerID], &info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating owned info rows: %w", err)
	}
	return result, nil
}

// scanUserPlayerOwnRow 扫描用户球员拥有行数据
func scanUserPlayerOwnRow(rows interface {
	Scan(dest ...any) error
}, r *model.UserPlayerOwn, dtOut *sql.NullTime) error {
	return rows.Scan(
		&r.ID,
		&r.UserID,
		&r.PlayerID,
		&r.OwnSta,
		&r.PriceIn,
		&r.PriceOut,
		&r.NumIn,
		&r.DtIn,
		dtOut,
	)
}

// ============================================================================
// UserPlayerOwnCommand 用户球员拥有操作（插入、更新）
// ============================================================================

// UserPlayerOwnCommand 用户球员拥有相关操作
type UserPlayerOwnCommand struct {
	DbBase
}

// NewUserPlayerOwnCommand 创建一个 UserPlayerOwnCommand
func NewUserPlayerOwnCommand() *UserPlayerOwnCommand {
	return &UserPlayerOwnCommand{
		DbBase: DbBase{},
	}
}

// InsertPlayerOwn 插入一条购买记录
func (c *UserPlayerOwnCommand) InsertPlayerOwn(ctx context.Context, database *DB, userID, playerID, num, cost uint, dt time.Time) error {
	const query = `
INSERT INTO u_p_own 
	(uid, pid, own_sta, price_in, num_in, dt_in)
VALUES (?, ?, 1, ?, ?, ?)`

	_, err := database.ExecContext(ctx, query, userID, playerID, cost, num, dt)
	if err != nil {
		return fmt.Errorf("failed to insert player own: %w", err)
	}
	return nil
}

// UpdatePlayerOwnToSold 将已购买的球员标记为已出售
func (c *UserPlayerOwnCommand) UpdatePlayerOwnToSold(ctx context.Context, database *DB, userID, playerID, cost uint, dt time.Time) error {
	const query = `
UPDATE u_p_own 
SET own_sta = 2, price_out = ?, dt_out = ?
WHERE uid = ? AND pid = ? AND own_sta = 1
LIMIT 1`

	result, err := database.ExecContext(ctx, query, cost, dt, userID, playerID)
	if err != nil {
		return fmt.Errorf("failed to update player own to sold: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNoRows
	}
	return nil
}
