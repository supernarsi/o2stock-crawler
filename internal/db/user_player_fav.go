package db

import (
	"context"
	"fmt"
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
