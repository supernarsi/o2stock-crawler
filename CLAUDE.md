# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

O2Stock-Crawler 是一个使用 Golang 实现的 NBA2K Online2 球员/道具价格抓取与入库工具，包含数据采集爬虫和 HTTP API 服务。

## 开发命令

### 构建与运行

```bash
# 安装依赖
go mod tidy

# 运行爬虫 (一次性抓取)
go run ./cmd/o2stock-crawler-ol2

# 运行爬虫 (循环模式，间隔 1 小时)
go run ./cmd/o2stock-crawler-ol2 loop 1h

# 运行 API 服务 (默认 :8080)
go run ./cmd/o2stock-api

# 构建 (带 .env 嵌入)
./build.sh
./build.sh api o2stock-api    # 构建 API 服务
```

### 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包测试
go test ./internal/db/...
go test ./internal/service/...

# 短模式 (跳过数据库相关测试)
go test -short ./...
```

### 运行特定爬虫

```bash
go run ./cmd/o2stock-player-ipi      # IPI 球员评分
go run ./cmd/o2stock-player-extra    # 球员额外数据
go run ./cmd/nba-lineup              # NBA 阵容分析
go run ./cmd/o2stock-crawler-tx      # 腾讯 NBA 数据
```

## 架构概览

### 分层架构 (Clean Architecture)

```
cmd/                          # 程序入口，仅做装配/启动
├── o2stock-api/              # HTTP API 服务
├── o2stock-crawler-ol2/      # OL2 球员/道具爬虫
└── ...

internal/                     # 核心业务实现
├── controller/               # HTTP 层 (路由、参数校验、响应)
├── service/                  # 业务逻辑层 (编排、领域逻辑)
├── db/repositories/          # 数据访问层 (GORM 操作)
├── middleware/               # 中间件 (鉴权、签名、日志、CORS)
├── crawler/                  # 外部 API 客户端
├── entity/                   # 领域实体 (GORM 模型)
├── dto/                      # 数据传输对象
└── consts/                   # 常量定义

api/                          # API 通用定义/响应结构
docs/                         # 需求/设计/ADR 文档
```

### 分层职责

- **controller**: HTTP 请求处理、参数校验、调用 service、返回响应，不直接写 SQL
- **service**: 业务编排与领域逻辑，依赖 repository 与外部客户端
- **db/repositories**: 数据访问层，封装 GORM 操作，提供清晰方法接口
- **middleware**: 请求链路中间件，按顺序执行 (Client → CORS → Logging → Signature)

### 配置系统

程序优先读取运行时 `.env` 文件，不存在时使用编译时注入的配置：

```bash
# 编译时嵌入 .env
go build -ldflags "-X 'o2stock-crawler/internal/config.EmbeddedEnv=<content>'"
```

### 数据库表

- `players`: 球员价格表
- `p_p_history`: 球员历史价格
- `items`: 道具表
- `p_i_history`: 道具历史价格
- `u_p_own`: 用户拥有球员数据
- `u_i_own`: 用户拥有道具数据
- `task_status`: 任务状态记录

## Cursor Rules 摘要

项目包含以下开发约定 (位于 `.cursor/rules/`):

- **00-core.mdc**: 项目结构与代码组织
- **05-docs-and-language.mdc**: 简体中文回复，文档统一放在 `docs/`
- **10-go-style.mdc**: Go 代码规范 (gofmt、错误处理、context 传递)
- **20-api-http.mdc**: HTTP API、路由、中间件约定
- **30-db-gorm.mdc**: Gorm repository 模式、事务与错误处理
- **90-project-structure.mdc**: 目录结构说明

## 环境变量

### OL2 接口配置
- `OL2_OPENID`, `OL2_ACCESS_TOKEN`, `OL2_SIGN`, `OL2_LOGIN_CHANNEL`, `OL2_NONSE_STR`
- `OL2_BASE_URL`: 默认 `https://nba2k2app.game.qq.com/user/favorite/rosters`

### 数据库配置
- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`

### API 配置
- `API_ADDR`: API 监听地址，默认 `:8080`
- `JWT_SECRET`: JWT 密钥
- `WECHAT_APP_ID`, `WECHAT_APP_SECRET`: 微信公众号配置

## 部署

### Systemd 服务

```bash
# 复制 service 文件
sudo cp o2stock-api.service /etc/systemd/system/
sudo systemctl daemon-reload

# 管理命令
sudo systemctl start|stop|restart|status o2stock-api
sudo systemctl enable o2stock-api
journalctl -u o2stock-api -f
```

## 重要约定

- **文档位置**: 所有需求/设计/ADR 文档必须放在 `docs/` 目录
- **测试规范**: 数据库写入/修改/删除操作必须使用 mock
- **Go 版本**: `go 1.24.0`
- **回复语言**: 默认使用简体中文
