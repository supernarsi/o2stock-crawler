# DB 层设计原则：方法 vs 函数

## 核心原则

### 使用**方法（Method）**的场景

1. **需要访问类型状态**
   - 当操作需要读取或修改类型的字段时
   - 例如：`PlayersQuery` 需要访问 `QueryBase` 中的 `limit`、`offset`、`orderBy`

```go
// ✅ 使用方法：需要访问 QueryBase 的状态
func (q *PlayersQuery) ListPlayers(ctx context.Context, database *DB, ...) {
    // 使用 q.limit, q.offset, q.orderBy
}

// ❌ 不应该用函数：需要传入太多参数
func ListPlayers(ctx, database, limit, offset, orderBy, ...) // 参数过多
```

2. **封装查询参数**
   - 当查询需要多个相关参数时，使用类型封装更清晰
   - 例如：`PlayerHistoryQuery` 封装了 `playerID` 和 `QueryBase`

```go
// ✅ 使用方法：参数封装在类型中
type PlayerHistoryQuery struct {
    QueryBase
    playerID uint32
}
func (q *PlayerHistoryQuery) GetPlayerHistory(...) // 简洁

// ❌ 函数版本：参数过多
func GetPlayerHistory(ctx, database, playerID, limit, orderBy, ...) // 参数过多
```

3. **业务逻辑与类型紧密相关**
   - 当操作是类型的主要职责时
   - 例如：`UserPlayerOwnQuery` 的所有操作都与用户相关

```go
// ✅ 使用方法：操作属于类型的职责
func (q *UserPlayerOwnQuery) GetUserOwnedPlayers(...) // 属于 UserPlayerOwnQuery 的职责
func (q *UserPlayerOwnQuery) CountOwnedPlayers(...)   // 属于 UserPlayerOwnQuery 的职责
```

4. **需要区分 Query 和 Command**
   - Query：只读操作，返回数据
   - Command：写操作，修改数据
   - 使用不同的类型区分职责

```go
// Query：只读操作
type UserPlayerOwnQuery struct { ... }
func (q *UserPlayerOwnQuery) GetUserOwnedPlayers(...) // 查询

// Command：写操作
type UserPlayerOwnCommand struct { ... }
func (c *UserPlayerOwnCommand) InsertPlayerOwn(...) // 插入
```

### 使用**函数（Function）**的场景

1. **无状态的纯函数**
   - 输入相同，输出一定相同
   - 不依赖任何外部状态
   - 例如：数据转换、格式化、计算

```go
// ✅ 使用函数：纯函数，无状态
func calculateStartTime(period uint8) time.Time {
    // 只依赖输入参数，不依赖任何状态
}

func getOrderDirection(orderAsc bool) string {
    // 纯函数，输入输出映射
}
```

2. **通用的工具函数**
   - 可以被多个类型共享
   - 不特定于某个类型
   - 例如：SQL 构建、数据提取

```go
// ✅ 使用函数：通用工具，多个类型都会用到
func buildINClause(values []any) (string, []any) {
    // PlayersQuery, MultiPlayersHistoryQuery 都会用到
}

func extractPlayerIDs(players []*model.Players) []uint {
    // 通用的数据提取逻辑
}
```

3. **数据扫描函数**
   - 扫描操作是通用的，不依赖类型状态
   - 可以被多个查询方法复用

```go
// ✅ 使用函数：通用的扫描逻辑
func scanPlayerRow(rows interface{ Scan(...) error }, r *model.Players) error {
    // 可以被多个查询方法使用
}

// ✅ 使用方法：需要访问类型状态
func (q *PlayersQuery) queryPlayers(...) {
    // 使用 q.limit, q.offset
    scanPlayerRow(rows, &r) // 调用通用函数
}
```

4. **向后兼容的包装函数**
   - 为了保持 API 兼容性
   - 内部调用新的方法实现

```go
// ✅ 使用函数：向后兼容
func GetOwnedInfoByPlayerIDs(ctx, database, userID, playerIDs) {
    // 内部调用新的方法
    return NewUserPlayerOwnQuery(userID).GetOwnedInfoByPlayerIDs(...)
}
```

## 设计模式

### Query 模式

```go
// 1. 定义 Query 类型（封装查询参数）
type PlayersQuery struct {
    QueryBase  // limit, offset, orderBy
}

// 2. 构造函数（设置初始状态）
func NewPlayersQuery(page, limit int, orderBy string, orderAsc bool) *PlayersQuery {
    return &PlayersQuery{
        QueryBase: QueryBase{
            limit:   limit,
            offset:  (page - 1) * limit,
            orderBy: NewOrderBy(orderBy, orderDir),
        },
    }
}

// 3. 查询方法（使用封装的状态）
func (q *PlayersQuery) ListPlayers(ctx, database, period, orderBy, orderAsc) {
    // 使用 q.limit, q.offset, q.orderBy
}
```

### Command 模式

```go
// 1. 定义 Command 类型（无状态，只用于组织方法）
type UserPlayerOwnCommand struct {
    DbBase  // 空结构，仅用于方法分组
}

// 2. 构造函数
func NewUserPlayerOwnCommand() *UserPlayerOwnCommand {
    return &UserPlayerOwnCommand{}
}

// 3. 命令方法（修改数据）
func (c *UserPlayerOwnCommand) InsertPlayerOwn(...) {
    // 插入操作
}
```

### 混合模式

```go
// 方法：需要访问类型状态
func (q *PlayersQuery) ListPlayers(...) {
    // 使用 q.limit, q.offset
    players := q.queryPlayers(...)  // 调用方法
    ids := extractPlayerIDs(players) // 调用函数
}

// 方法：内部实现，需要访问状态
func (q *PlayersQuery) queryPlayers(...) {
    // 使用 q.limit, q.offset
}

// 函数：通用工具，无状态
func extractPlayerIDs(players []*model.Players) []uint {
    // 纯函数
}
```

## 决策流程图

```
需要访问类型状态？
├─ 是 → 使用方法
│   ├─ 需要封装多个参数？ → Query 模式
│   ├─ 需要修改数据？ → Command 模式
│   └─ 业务逻辑与类型相关？ → 类型方法
│
└─ 否 → 使用函数
    ├─ 纯函数（输入输出映射）？ → 工具函数
    ├─ 多个类型共享？ → 通用函数
    └─ 向后兼容？ → 包装函数
```

## 实际案例对比

### 案例 1：查询球员列表

```go
// ✅ 使用方法：需要封装 limit, offset, orderBy
type PlayersQuery struct {
    QueryBase  // limit, offset, orderBy
}
func (q *PlayersQuery) ListPlayers(...) {
    // 使用 q.limit, q.offset
}

// ❌ 函数版本：参数过多
func ListPlayers(ctx, database, limit, offset, orderBy, period, ...) {
    // 参数太多，难以维护
}
```

### 案例 2：计算开始时间

```go
// ✅ 使用函数：纯函数，无状态
func calculateStartTime(period uint8) time.Time {
    // 只依赖输入，不依赖任何状态
}

// ❌ 方法版本：不需要状态，用方法多余
func (q *PlayersQuery) calculateStartTime(period uint8) time.Time {
    // q 没有被使用，用方法没有意义
}
```

### 案例 3：提取球员 ID

```go
// ✅ 使用函数：通用工具，多个地方使用
func extractPlayerIDs(players []*model.Players) []uint {
    // PlayersQuery, UserPlayerOwnQuery 都会用到
}

// ❌ 方法版本：不特定于某个类型
func (q *PlayersQuery) extractPlayerIDs(...) {
    // 这个逻辑不特定于 PlayersQuery，其他类型也需要
}
```

## 总结

- **方法**：有状态、封装参数、业务逻辑与类型相关
- **函数**：无状态、通用工具、纯函数、向后兼容

关键判断标准：**是否需要访问或修改类型的字段？**
- 需要 → 方法
- 不需要 → 函数
