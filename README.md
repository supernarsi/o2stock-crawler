## O2Stock-Crawler

一个使用 Golang 实现的 NBA2K Online2 球员价格抓取与入库工具。

### 功能概述

- **定时/一次性调用**：请求官方接口获取收藏列表中的球员价格数据。
- **JSON 解析与模型映射**：解析接口返回的 `rosterList` 数据。
- **MySQL 持久化**：
  - 更新/插入当前球员价格到 `players` 表。
  - 将每次抓取的价格快照写入 `p_p_history` 历史表。

### 数据库表结构

请在 MySQL 中创建以下表（来自设计文档，可自行调整索引/约束）：

```sql
CREATE TABLE `players` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `player_id` int unsigned NOT NULL COMMENT '球员 id',
  `p_name_show` varchar(255) NOT NULL COMMENT '球员展示名称',
  `p_name_en` varchar(255) NOT NULL COMMENT '球员英文名称',
  `team_abbr` varchar(255) NOT NULL COMMENT '球队名称',
  `version` int unsigned NOT NULL DEFAULT '0' COMMENT '球员版本',
  `card_type` int unsigned NOT NULL DEFAULT '0' COMMENT '卡类型',
  `player_img` varchar(255) NOT NULL COMMENT '球员头像',
  `price_standard` int unsigned NOT NULL DEFAULT '0' COMMENT '单卡价格-基准',
  `price_current_lowest` int unsigned NOT NULL DEFAULT '0' COMMENT '市场最低售价',
  `price_sale_lower` int unsigned NOT NULL DEFAULT '0' COMMENT '售价-低',
  `price_sale_upper` int unsigned NOT NULL DEFAULT '0' COMMENT '售价-高',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_player` (`player_id`,`version`,`card_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='球员价格表';

CREATE TABLE `p_p_history` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `player_id` int unsigned NOT NULL DEFAULT '0' COMMENT '球员 id',
  `at_date` date NOT NULL COMMENT '日期',
  `at_date_hour` char(10) NOT NULL DEFAULT '2026010100' COMMENT '价格对应的日期小时，格式为：年月日时（例 2026010223）',
  `at_year` char(4) NOT NULL DEFAULT '2026' COMMENT '价格对应的年份',
  `at_month` char(2) NOT NULL DEFAULT '01' COMMENT '价格对应的月份',
  `at_day` char(2) NOT NULL DEFAULT '01' COMMENT '价格对应的日期',
  `at_hour` char(2) NOT NULL DEFAULT '00' COMMENT '价格对应的小时',
  `price_standard` int unsigned NOT NULL DEFAULT '0' COMMENT '基础卡片单卡价格',
  `price_lower` int unsigned NOT NULL DEFAULT '0' COMMENT '市场最低价（单卡）',
  `price_upper` int unsigned NOT NULL DEFAULT '0' COMMENT '市场最高价（单卡）',
  `c_time` datetime NOT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='球员历史价格';

CREATE TABLE `u_p_own` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL DEFAULT '0' COMMENT '用户 id',
  `player_id` int unsigned NOT NULL DEFAULT '0' COMMENT '球员 id',
  `own_sta` tinyint unsigned NOT NULL DEFAULT '0' COMMENT '状态：0.未拥有；1.已购买；2.已出售',
  `price_in` int unsigned NOT NULL DEFAULT '0' COMMENT '购买时的总价格',
  `price_out` int unsigned NOT NULL DEFAULT '0' COMMENT '出售时的总价格',
  `num_in` int unsigned NOT NULL DEFAULT '0' COMMENT '购买的卡数',
  `dt_in` datetime NOT NULL COMMENT '购买时间',
  `dt_out` datetime DEFAULT NULL COMMENT '出售时间',
  PRIMARY KEY (`id`),
  KEY `idx_uid` (`user_id`),
  KEY `idx_pid` (`player_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户拥有球员数据表';
```

### 环境变量配置

程序通过环境变量读取接口和数据库配置。你可以在本地创建一个 `.env` 文件，然后使用 `github.com/joho/godotenv` 在本地开发时自动加载。

#### OL2 接口配置

- **OL2_OPENID**：接口中使用的 `openid`。
- **OL2_ACCESS_TOKEN**：接口中使用的 `access_token`。
- **OL2_SIGN**：接口中使用的 `sign`。
- **OL2_LOGIN_CHANNEL**：登录渠道，默认 `qq`。
- **OL2_NONSE_STR**：`nonseStr`。
- **OL2_BASE_URL**：接口地址，默认 `https://nba2k2app.game.qq.com/user/favorite/rosters`。

示例（请根据自己的实际账号信息修改）：

```env
OL2_OPENID=
OL2_ACCESS_TOKEN=
OL2_SIGN=
OL2_LOGIN_CHANNEL=
OL2_NONSE_STR=
OL2_BASE_URL=https://nba2k2app.game.qq.com/user/favorite/rosters
```

#### 数据库配置

通过环境变量覆盖：

```env
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASS=
DB_NAME=
```

### 安装依赖

在项目根目录执行：

```bash
go mod tidy
```

### 运行方式

程序入口在 `cmd/o2stock-crawler/main.go`。

- **一次性抓取并入库**：

```bash
go run ./cmd/o2stock-crawler
# 或
go run ./cmd/o2stock-crawler run-once
```

- **循环定时抓取**（例如每 1 小时抓取一次）：

```bash
go run ./cmd/o2stock-crawler loop 1h
```

间隔参数使用 Go 的 duration 语法，例如 `30m`、`2h`、`90m` 等；若不填则默认 60 分钟。

## API 服务

项目还提供了一个 HTTP API 服务，用于查询球员数据和用户买卖记录。

### 启动 API 服务

```bash
go run ./cmd/o2stock-api
```

默认监听 `:8080`，可通过环境变量 `API_ADDR` 修改。

### 运行测试

项目包含单元测试，运行方式：

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/db/...

# 跳过需要数据库的测试（短模式）
go test -short ./...
```

**注意：** 数据库相关的测试需要配置真实的数据库连接。测试会自动跳过如果无法连接数据库。

### 后续可扩展点

- 使用 cron（如 crontab 或系统级定时任务）调用 `run-once`。
- 增加更多字段入库，例如 `grade`、`popularity` 等。
- 增加日志输出到文件以及 Prometheus 监控等。
- 添加用户认证中间件，从 token 中获取 user_id。
- 添加更多的数据统计和分析接口。


