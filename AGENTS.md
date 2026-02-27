# AGENTS.md - o2stock-crawler 开发指南

## 项目概述

这是一个使用 Go 1.24.0 实现的 NBA2K Online2 球员价格抓取与 Web API 工具。项目采用分层架构（Controller → Service → Repository）。

---

## 1. 构建与测试命令

### 常用命令

```bash
# 安装依赖
go mod tidy

# 构建爬虫程序（默认）
./build.sh

# 构建 API 服务
./build.sh o2stock-api o2stock-api

# 运行爬虫（一次性）
go run ./cmd/o2stock-crawler run-once

# 运行爬虫（定时，每小时）
go run ./cmd/o2stock-crawler loop 1h

# 运行 API 服务
go run ./cmd/o2stock-api
```

### 测试命令

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/service/...

# 运行单个测试文件
go test -v ./internal/service/auth_test.go

# 运行单个测试函数
go test -v -run TestToken ./internal/service/

# 跳过需要数据库的测试（短模式）
go test -short ./...
```

### 代码检查

```bash
# 格式化代码（必须执行）
gofmt -w .

# 编译检查
go build ./...
go build -o o2stock-api ./cmd/o2stock-api
```

---

## 2. 代码风格指南

### 基本原则

- 使用 **简体中文** 回复（除非用户明确要求其他语言）
- Prefer **small, reviewable diffs**；避免 drive-by refactors
- Keep **public APIs stable**：避免重命名导出标识符
- 若变更行为，在可行时添加或更新 **tests**

### 格式化

- 代码必须保持 `gofmt` 清洁
- Prefer early returns；保持函数小而可读
- 每个 import 用单独的行（标准库分组，然后第三方库）
- import 分组用空行分隔：
  ```go
  import (
      "context"
      "fmt"
      "time"
  
      "o2stock-crawler/internal/db"
  
      "github.com/golang-jwt/jwt/v5"
  )
  ```

### 命名约定

- **变量/函数**：使用 camelCase
- **常量**：使用 PascalCase 或全大写 SCREAMING_SNAKE_CASE
- **结构体/接口**：使用 PascalCase
- **私有字段**：以小写字母开头
- **缩写词**：保持一致（如 `API` vs `Api` → 统一用 `API`）

### 类型定义

- 优先使用具体类型而非泛型（Go 1.24 特性如需使用需评估）
- 接口设计要小（interface segregation）
- 使用有意义的类型别名：
  ```go
  type PlayerID uint32
  type UserID uint
  ```

### 错误处理

- 使用 `fmt.Errorf("do X: %w", err)` 包装错误
- 避免使用 `err.Error()` 字符串比较；使用 sentinel errors 或 `errors.Is/As`
- 区分 "not found" 与其他错误：
  ```go
  if errors.Is(err, gorm.ErrRecordNotFound) {
      return nil, nil // not found
  }
  return nil, fmt.Errorf("repo X: %w", err)
  ```
- Return errors up the stack；避免在非入口处 `log.Fatal*`
- 避免向客户端泄露内部错误详情；日志记录 server-side，返回 sanitized messages

### Context 使用

- Service/Repository 方法中接受 `context.Context` 进行 I/O 操作
- 使用 `http.NewRequestWithContext` 进行外部 HTTP 调用

### 日志

- 在边缘（handlers, jobs）记录日志，而非深层 helpers
- 避免记录敏感字段（tokens、secrets、credentials）
- 使用结构化日志（项目未强制特定库，保持一致即可）

### 数据库 / Gorm

- Repository 方法放在 `internal/db/repositories/*`
- 保持查询 explicit；避免 "magic" 行为
- 使用事务处理必须原子的多步写入
- 更新时注意零值覆盖问题（使用 `Select`/`Omit`）
- 不在生产路径记录带 secrets/PII 的原始 SQL

### 测试规范

- 使用 table-driven tests
- 保持测试 deterministic；避免真实网络调用
- **数据库操作测试**：
  - **写入/修改/删除**：必须使用 mock，不要真实执行数据库变更
  - **查询操作**：可以真实执行（读取测试数据）

### API / HTTP

- 使用 `middleware.Router` 注册路由
- 使用显式 HTTP 方法常量（`http.MethodGet`, etc）
- Middleware 顺序：先 "context enrichment"
- 返回一致的 JSON 错误 payloads：使用 `api.Error(...)`
- 使用正确的 status codes（`401` 认证, `403` 禁止, `405` 方法, `429` 限流, `500` 意外）
- 验证 controller 中的 query/body 字段

### 项目结构

```
cmd/                    # 可执行程序入口
  o2stock-api/          # API 服务
  o2stock-crawler/      # 爬虫程序
internal/               # 核心业务实现
  controller/           # HTTP 层
  service/              # 业务逻辑层
  db/repositories/      # 数据访问层
  middleware/           # 中间件
  consts/               # 常量定义
  dto/                  # 数据传输对象
  entity/               # 领域实体
api/                    # API 层公共定义
docs/                   # 需求/设计/技术文档
```

### 配置与安全

- **禁止提交** `.env` 或 secrets（JWT secret, WeChat app secret, DB creds）
- 优先从环境变量读取运行时配置
- 构建产物使用后删除（`o2stock-api`、`o2stock-crawler` 等可执行文件）

### 文档规范

- 所有新增文档放在 `docs/` 目录下
- 命名建议：
  - `docs/requirements/<topic>.md`
  - `docs/design/<topic>.md`
- 文档最低内容要求：
  - 需求：背景、目标/非目标、范围、验收标准、风险
  - 设计：方案概览、关键接口/数据结构、错误处理、回滚/迁移方案、测试计划

---

## 3. 开发注意事项

### Go 模块要求

- 兼容 `go.mod`（Go 1.24.0）和现有依赖
- 不要引入新依赖，除非明确需要

### 变更原则

- 新增/修改接口：优先补齐对应 controller/service/repo 分层
- 涉及跨层调整：先在 `docs/` 写简短设计说明，再落代码
