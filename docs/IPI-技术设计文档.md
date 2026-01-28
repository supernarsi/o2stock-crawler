# 球员投资潜力指数（IPI）技术设计文档

> 基于 [球员投资潜力评分模型 (Investment Potential Index, IPI).md](./球员投资潜力评分模型%20(Investment%20Potential%20Index,%20IPI).md) 的业务逻辑，结合项目实际数据结构进行技术设计与实施规划。

---

## 一、设计目标与约束

### 1.1 目标

- 设计并实现 **IPI（Investment Potential Index）** 计算逻辑，量化球员在 2KOL2 游戏内的短期（1～3 个月）投资潜力。
- 输出可排序的 IPI 分数及投资等级，支撑「推荐球员」「潜力排行」等产品功能。

### 1.2 约束与前提

- **投资周期**：短期（1～3 个月内价格有望上涨）。
- **交易成本**：卖出时扣除 **25% 交易税**，所有收益相关计算需考虑税后净收益。
- **数据来源**：仅使用项目现有数据（见下文「数据盘点」），不依赖 2K26 正代、交易流言等外部接口；年龄、伤病、交易流言等可后续扩展。

---

## 二、数据盘点

### 2.1 已有数据（可直接使用）

| 数据 | 来源 | 用途 |
|------|------|------|
| 球员基本信息 | `players` | OVR、价格、球队等 |
| 当前游戏能力值 | `players.over_all` | 同 OVR 段比价、能力值倒挂 |
| 当前游戏价格 | `players.price_standard` | 洼地分、税后安全边际 |
| 球员最近价格变化 | `players.price_change_1d`, `players.price_change_7d` | 价格趋势 |
| 近 5/10 场战力值 | `players.power_per5`, `power_per10` | 表现盈余、能力值倒挂 |
| 历史价格 | `p_p_history` | 价格饱和度、分位数 |
| 本赛季场均数据 | `player_season_stats` | 赛季战力、上场时间趋势 |
| 近期单场数据 | `player_game_stats` | 战力计算、上场时间趋势 |

### 2.2 战力值计算（已实现）

单场战力值公式（与现有 `CalculateAndSyncPower` 保持一致）：

```
Power = Points + 1.2×Rebounds + 1.5×Assists + 3×Steals + 3×Blocks - Turnovers
```

- `power_per5`：近 5 场平均战力。
- `power_per10`：近 10 场平均战力。

**重要**：`over_all` 为游戏内能力值，`power_per5`/`power_per10` 为真实赛场表现，二者**维度不同、不建立映射**。投资逻辑依赖的是「真实表现 vs 游戏能力值」的**相对差异**（如排名倒挂），而非数值换算。

### 2.3 后续可扩展数据

- 年龄（`age`）、伤病（`is_injured`）、交易流言等：当前设计中用占位/默认值，预留扩展点。

---

## 三、IPI 公式与四维度设计

### 3.1 综合公式

```
IPI = ( w₁·S_perf + w₂·V_gap + w₃·M_growth ) × (1 - R_risk)
```

- `S_perf`：表现盈余分  
- `V_gap`：价值洼地分  
- `M_growth`：成长动能与题材分  
- `R_risk`：风险折现因子（0～1，越大则扣减越多）  
- 建议权重：`w₁=0.4`，`w₂=0.35`，`w₃=0.25`（可配置）

---

### 3.2 表现盈余分（S_perf）

**目的**：刻画「最近真实表现优于赛季平均」以及「真实表现 vs 游戏能力值」的倒挂程度。

**公式**：

```
S_perf = α × (PowerPer5 / PowerSeasonAvg) + β × RankInversionIndex
```

- `PowerPer5`：`players.power_per5`（近 5 场平均战力）。
- `PowerSeasonAvg`：根据 `player_season_stats` 用上述战力公式计算本赛季场均战力；若无赛季数据，则用 `power_per10` 或跳过该项（仅用倒挂项）。
- `RankInversionIndex`：能力值倒挂指数，见下。
- 建议 `α=0.6`，`β=0.4`。

**能力值倒挂指数（RankInversionIndex）**  

- 对**参与排名的球员集合**（如 `nba_player_id > 0` 且非自由球员）分别计算：
  - **真实表现排名**：按 `power_per5` 降序排名。
  - **游戏能力值排名**：按 `over_all` 降序排名。
- 定义：
  - `diff = GameOVRRank - RealPerfRank`（游戏排名减真实表现排名）。
  - 若 `diff ≤ 0`（真实表现不如或等于游戏地位），则 `RankInversionIndex = 0`。
  - 若 `diff > 0`，则 `RankInversionIndex = min(1, diff / N)`，其中 `N` 为参与排名球员总数（或固定常数如 500）用于归一化。
- 即：真实表现排名越领先于游戏能力值排名，倒挂越严重，指数越高。

**边界与异常**：

- `PowerSeasonAvg <= 0`：该项比值视为 0 或跳过，避免除零。
- 无 `power_per5` 或无法排名：`S_perf` 仅用有定义子项，或整体置 0。

---

### 3.3 价值洼地分（V_gap）

**目的**：衡量当前价格是否处于同能力值段的相对低位，且考虑 25% 税后仍具安全边际。

**公式**：

```
V_gap = PriceOVRAvg / PriceStandard
```

- `PriceStandard`：`players.price_standard`。
- `PriceOVRAvg`：同 OVR 段（如 `over_all` 在 `[OVR-2, OVR+2]`）球员的 `price_standard` 均值；若本段样本过少，可适当扩大区间或用全表均值回退。

**税后安全边际（校验逻辑，不直接进入 V_gap 公式）**：

- 目标价 `P_target`：可用同 OVR 段均价或稍高估（如 `PriceOVRAvg × 1.05`）作为能力值更新后的预期价。
- 获利条件：`P_target × 0.75 > PriceStandard × 1.1`（税后至少 10% 净利润）。
- 可在输出 IPI 时附带布尔字段 `meets_tax_safe_margin`，便于筛选。

**边界**：

- `PriceStandard <= 0`：`V_gap` 置 0 或跳过该球员。
- 同 OVR 段无人：`PriceOVRAvg` 用全表均价或标记为「无参考」。

---

### 3.4 成长动能与题材分（M_growth）

**目的**：年轻、上场时间提升、具备话题（如交易流言）的球员加分。

**公式**：

```
M_growth = AgeFactor × (1 + MinutesTrendBonus + TradeRumorBonus)
```

- **AgeFactor**
  - `age < 23`：`1.2`
  - `23 ≤ age ≤ 33`：`1.0`
  - `age > 33`：`0.8`
  - 若无年龄：默认 `1.0`。

- **MinutesTrendBonus**
  - `MT_recent`：近 5～10 场场均上场时间（来自 `player_game_stats`）。
  - `MT_season`：本赛季场均上场时间（`player_season_stats.minutes`）。
  - `ΔMT = MT_recent - MT_season`；若 `ΔMT > 0` 且 `MT_season > 0`，则 `MinutesTrendBonus = min(0.2, ΔMT / MT_season)`，否则为 0（上限 20% 加成）。

- **TradeRumorBonus**
  - 有交易流言：`0.15`；否则 `0`。当前无数据，**固定为 0**，预留扩展。

**边界**：缺赛季或近场数据时，对应项为 0。

---

### 3.5 风险折现因子（R_risk）

**目的**：伤病、价格已处高位时，降低 IPI。

**公式**：

```
R_risk = InjuryRisk + PriceSaturationRisk
```

- **InjuryRisk**
  - 若 `is_injured == true`：`0.5`；否则 `0`。
  - 当前无伤病数据，**固定为 0**，预留扩展。

- **PriceSaturationRisk**
  - 基于 `p_p_history` 取该球员过去一段时间（如 90 天）的 `price_standard` 序列，计算当前价格在历史中的分位数。
  - 若 `当前价格 ≥ 90 分位`：`0.3`；
  - 若 `当前价格 ≥ 75 分位`：`0.15`；
  - 否则：`0`。

**约束**：`R_risk`  clamp 在 `[0, 1]`，避免负值或超过 1。

---

## 四、实施步骤

### 阶段一：基础能力与数据准备

| 步骤 | 内容 | 产出 |
|------|------|------|
| 1.1 | 定义 IPI 配置结构（权重、阈值、OVR 区间、分位档位等） | `internal/config` 或 `internal/service/ipi` 内配置 |
| 1.2 | 实现「赛季场均战力」计算：基于 `player_season_stats` + 战力公式 | 函数 `SeasonPowerFromStats(stats)` 或等价 |
| 1.3 | 实现「近 N 场场均上场时间」查询与计算 | 基于 `player_game_stats`，可复用现有 stats 查询封装 |
| 1.4 | 实现「同 OVR 段均价」查询：按 `over_all` 分段聚合 | Repo/Query 层方法，如 `AvgPriceByOVRSegment(ovr, radius)` |
| 1.5 | 实现「单球员历史价格分位数」：基于 `p_p_history` 近 90 天数据 | 函数 `PricePercentile(playerID, days)` → 如 75, 90 分位值 |

### 阶段二：四维度计算

| 步骤 | 内容 | 产出 |
|------|------|------|
| 2.1 | 实现全量参与排名球员的 `power_per5`、`over_all` 排名，并计算 `RankInversionIndex` | `RankInversionIndex(playerID, rankData)` |
| 2.2 | 实现 `S_perf`：结合 `PowerPer5/PowerSeasonAvg` 与 `RankInversionIndex` | `CalcSPerf(...)` |
| 2.3 | 实现 `V_gap`：同 OVR 段均价 / 当前价；可选实现税后安全边际布尔判断 | `CalcVGap(...)`，`MeetsTaxSafeMargin(...)` |
| 2.4 | 实现 `M_growth`：年龄因子（含占位）、上场时间趋势、交易流言（占位 0） | `CalcMGrowth(...)` |
| 2.5 | 实现 `R_risk`：伤病（占位 0）、价格饱和度 | `CalcRRisk(...)` |

### 阶段三：IPI 聚合与输出

| 步骤 | 内容 | 产出 |
|------|------|------|
| 3.1 | 实现 `IPI = (w₁·S_perf + w₂·V_gap + w₃·M_growth) × (1 - R_risk)`，并 clamp 到合理区间 | `CalcIPI(...)` |
| 3.2 | 实现批量计算：给定球员 ID 列表（或「全部目标球员」），循环/批量计算 IPI | `BatchCalcIPI(ctx, playerIDs)` → `[]IPIResult` |
| 3.3 | 定义 `IPIResult` 结构：至少包含 `PlayerID`、`IPI`、`S_perf`、`V_gap`、`M_growth`、`R_risk`，以及可选的 `MeetsTaxSafeMargin`、`RankInversionIndex` 等 | `internal/model` 或 `api` 层 DTO |

### 阶段四：持久化与接口（可选）

| 步骤 | 内容 | 产出 |
|------|------|------|
| 4.1 | 设计 `player_ipi` 表（或等价）：`player_id`、`ipi`、各分项、`calculated_at` 等 | 迁移 + Model |
| 4.2 | 实现 IPI 的定时计算任务（如每日），写入 `player_ipi` | 与 crawler 调度或 api 定时任务对接 |
| 4.3 | 提供 HTTP API：按 IPI 排序分页、筛选 `meets_tax_safe_margin` 等 | Controller + Route |

### 阶段五：扩展与优化（后续）

| 步骤 | 内容 |
|------|------|
| 5.1 | 接入年龄、伤病、交易流言等数据，去掉占位，纳入 `M_growth`、`R_risk` |
| 5.2 | 同 OVR 段均价、历史分位数等做缓存（如 Redis），减少重复计算 |
| 5.3 | 参数调优：权重、分位阈值、OVR 区间等，结合历史回测 |

---

## 五、模块与目录建议

```
internal/
├── config/           # 已有
│   └── ipi.go        # 新增：IPI 权重、阈值等配置（或集中到 embed）
├── model/
│   └── ipi.go        # 新增：IPIResult、配置 struct 等
├── service/
│   └── ipi.go        # 新增：CalcIPI、BatchCalcIPI、各维度计算
├── db/
│   ├── repositories/
│   │   ├── history_repo.go   # 扩展：价格分位数查询
│   │   └── player_repo.go   # 扩展：同 OVR 段均价、排名等
│   └── ...
api/
└── ipi.go            # 新增：IPI 相关 API DTO
```

---

## 六、配置项建议（可配置化）

| 配置项 | 说明 | 默认 |
|--------|------|------|
| `IPI.Weights.SPerf` | w₁ | 0.4 |
| `IPI.Weights.VGap` | w₂ | 0.35 |
| `IPI.Weights.MGrowth` | w₃ | 0.25 |
| `IPI.SPerf.Alpha` | 表现盈余-赛季比权重 | 0.6 |
| `IPI.SPerf.Beta` | 表现盈余-倒挂权重 | 0.4 |
| `IPI.VGap.OVRRadius` | 同 OVR 段半径 | 2 |
| `IPI.VGap.TaxRate` | 交易税率 | 0.25 |
| `IPI.VGap.MinNetProfitRatio` | 最低净利比例（税后） | 0.1 |
| `IPI.RRisk.Pct90` | 90 分位风险系数 | 0.3 |
| `IPI.RRisk.Pct75` | 75 分位风险系数 | 0.15 |
| `IPI.HistoryDays` | 价格历史取最近天数 | 90 |

---

## 七、修订记录

| 日期 | 修改内容 |
|------|----------|
| 2025-01-25 | 初版：四维度公式、能力值倒挂指数、实施步骤、配置项 |

---

**文档状态**：供后续开发使用，实施时以本文档为准；若与业务模型文档冲突，以本文档的技术实现为准。
