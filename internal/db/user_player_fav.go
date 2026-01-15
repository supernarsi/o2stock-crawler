package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

/*
用户自选球员表
```sql
CREATE TABLE `u_p_fav` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `uid` int unsigned NOT NULL DEFAULT '0' COMMENT '用户 id',
  `pid` int unsigned NOT NULL DEFAULT '0' COMMENT '球员 id',
  `c_time` datetime NOT NULL COMMENT '添加时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户自选球员表';
```
*/

// CountFavPlayer 统计用户收藏的指定球员数量
func CountFavPlayer(ctx context.Context, database *DB, userID, playerID uint) (int, error) {
	const query = `
SELECT COUNT(*) 
FROM u_p_fav 
WHERE uid = ? AND pid = ?`

	var count int
	err := database.QueryRowContext(ctx, query, userID, playerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count fav player: %w", err)
	}
	return count, nil
}

// InsertFavPlayer 插入一条收藏记录
func InsertFavPlayer(ctx context.Context, database *DB, userID, playerID uint) error {
	const query = `
INSERT INTO u_p_fav 
	(uid, pid, c_time)
VALUES (?, ?, ?)`

	_, err := database.ExecContext(ctx, query, userID, playerID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert fav player: %w", err)
	}
	return nil
}

// GetFavPlayerIDs 获取用户收藏的所有球员ID
func GetFavPlayerIDs(ctx context.Context, database *DB, userID uint) ([]uint, error) {
	const query = `
SELECT pid 
FROM u_p_fav 
WHERE uid = ?`

	rows, err := database.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query fav player ids: %w", err)
	}
	defer rows.Close()

	var pids []uint
	for rows.Next() {
		var pid uint
		if err := rows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("failed to scan fav player id: %w", err)
		}
		pids = append(pids, pid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating fav player rows: %w", err)
	}
	return pids, nil
}

// GetFavMapByPlayerIDs 批量获取用户对指定球员的收藏状态
func GetFavMapByPlayerIDs(ctx context.Context, database *DB, userID uint, playerIDs []uint) (map[uint]bool, error) {
	if len(playerIDs) == 0 {
		return map[uint]bool{}, nil
	}

	// 构建 IN 查询
	placeholders := make([]string, len(playerIDs))
	args := make([]any, len(playerIDs)+1)
	args[0] = userID
	for i, pid := range playerIDs {
		placeholders[i] = "?"
		args[i+1] = pid
	}

	query := fmt.Sprintf(`SELECT pid FROM u_p_fav WHERE uid = ? AND pid IN (%s)`, strings.Join(placeholders, ","))

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query fav status: %w", err)
	}
	defer rows.Close()

	favMap := make(map[uint]bool)
	for rows.Next() {
		var pid uint
		if err := rows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("failed to scan fav pid: %w", err)
		}
		favMap[pid] = true
	}
	return favMap, nil
}
