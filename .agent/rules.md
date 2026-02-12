# Antigravity Project Rules for o2stock-crawler

这些规则指导 Antigravity 在此项目中的行为和编码标准，结合了项目原有的 `.cursor/rules` 约定。

## 1. 核心行为准则 (Identity & Identity)

-   **身份声明**：在每次聊天回复的开头，必须明确说明“模型名称、模型大小、模型类型及其修订版本（更新日期）”。
-   **回复语言**：默认使用**简体中文**进行回复。
-   **代码变更**：优先采用小的、可评审的 diff；避免无意义的大范围重构。
-   **API 稳定性**：保持导出标识符的稳定；更新调用方后再重命名。

## 2. 语言与文档规范 (Docs & Language)

-   **文档落盘**：所有新创建的需求、设计文档、ADR 必须放在 `docs/` 目录下。
    -   `docs/requirements/`
    -   `docs/design/`
    -   `docs/adr/`
-   **文档质量**：需求必须包含背景、目标、范围和验收标准；设计必须包含方案概览、关键接口和错误处理。

## 3. Go 编码规范 (Go Style)

-   **格式化**：始终运行 `gofmt`。
-   **逻辑组织**：优先使用 Early Returns；保持导出函数简短、可读。
-   **错误处理**：使用 `fmt.Errorf("...: %w", err)` 包装错误；避免字符串比较。
-   **Context**：在涉及 I/O 的 Service/Repo 层必须接收并传递 `context.Context`。
-   **构建清理**：如为验证而执行 `go build`，结束后**必须删除**二进制产物及构建缓存。

## 4. 架构与层级 (Project Structure)

-   **项目结构**：
    -   `cmd/`：仅做装配与启动。
    -   `internal/`：业务实现核心（仅限内部访问）。
    -   `api/`：外部通信协议与响应定义。
-   **分层调用**：
    -   `Controller` 解析输入、校验、调用 Service。
    -   `Service` 编排业务逻辑。
    -   `Repository` (internal/db/repositories) 负责 Gorm/SQL 操作。

## 5. API 与 数据库 (API & DB)

-   **API 响应**：统一使用 `api.Error(...)` 和 JSON 返回辅助函数。
-   **Gorm 安全**：优先使用 Repo 方法；严禁在 Controller 散写 SQL；更新时注意零值覆盖风险。
-   **事务**：多步写入操作必须使用事务以保证原子性。

## 6. 安全与配置 (Security)

-   **密钥保护**：严禁提交 `.env`、JWT 密钥、微信 AppSecret 或数据库凭据。
-   **配置读取**：优先从环境变量读取配置。
-   **日志记录**：严禁在日志中记录敏感字段（Token、密码等）。
## 7. 测试规范 (Testing Rules)

-   **数据库测试**：单元测试涉及到数据库**写入、修改、删除**操作时，严禁真实执行，必须使用 Mock；**查询**操作可以执行真实数据库操作。
