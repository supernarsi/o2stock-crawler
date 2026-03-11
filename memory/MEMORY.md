# 记忆文件

This file stores key insights and patterns for the O2Stock-Crawler project.

## 低薪高能球员识别问题 (2026-03-10 回测发现)

### 核心问题

预测模型系统性低估低薪高能球员，特别是：
1. **在 `players` 表中没有记录的球员**（如 Jaylin Williams 1631119）
2. **近期有爆发表现但历史数据不足的球员**

### 03-10 回测数据

| 球员 | 工资 | 预测战力 | 实际战力 | 差距 | 原因 |
|------|------|----------|----------|------|------|
| 杰林·威廉姆斯 | 12 | 28.5 | 52.9 | -24.4 | players 表无记录，缺少 power_per5/10 参考 |
| 德里克·琼斯 | 12 | 26.9 | 41.4 | -14.5 | 有 DB 记录但 power_per5=21.5 偏低 |

### Jaylin Williams 近 10 场表现 (tx_player_id=196152)
```
2026-03-10: 29 分 12 板 3 助 - 战力 53.9
2026-03-08: 9 分 14 板 4 助 - 战力 31.8
2026-03-05: 8 分 5 板 2 助 - 战力 17.0
2026-03-04: 17 分 16 板 6 助 - 战力 47.2
2026-03-02: 0 分 5 板 2 助 - 战力 13.0
```
近 5 场平均战力约 32.6，但 combat_power 仅 23.8，导致基础值偏低。

### 优化方向

1. **当 `players` 表无记录时，增加 recent_power 权重**
   - 当前逻辑：`baseValue = gamePower`（combat_power）
   - 优化建议：当 dbPlayer==nil 但 stats 充足时，提高 recent_power 权重至 0.5+

2. **添加"低薪高能"识别因子**
   - 条件：salary≤12 且 recent_avg_power≥30
   - 给予 1.1-1.2 倍的基础值奖励

3. **增加 Upside3 在预测中的权重**
   - Jaylin Williams Upside3 应该很高（多场 50+ 战力），但未有效反映在预测中

### 相关代码位置
- `internal/service/lineup_recommend_predict.go:88-107` - 基础值计算逻辑
- `internal/service/lineup_recommend_solver.go:183-262` - `calcLineupStructureFactor` 结构因子
- `internal/service/lineup_recommend_utils.go:125` - `calcPowerFromStats` 战力计算公式

### 回测命令
```bash
go run ./cmd/nba-lineup backtest 2026-03-10
```
