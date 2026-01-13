package db

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
