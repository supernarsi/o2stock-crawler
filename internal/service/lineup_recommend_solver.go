package service

import (
	"log"
	"math"
	"sort"
)

// solveOptimalLineup 在推荐模式下求解 TopN 阵容（仅使用正战力候选）。
func (s *LineupRecommendService) solveOptimalLineup(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
) [][]PlayerCandidate {
	return s.solveOptimalLineupInternal(candidates, salaryCap, pickCount, topN, false)
}

func (s *LineupRecommendService) solveOptimalLineupAllowZero(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
) [][]PlayerCandidate {
	return s.solveOptimalLineupInternal(candidates, salaryCap, pickCount, topN, true)
}

func (s *LineupRecommendService) solveOptimalLineupInternal(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
	allowNonPositive bool,
) [][]PlayerCandidate {
	if salaryCap <= 0 || pickCount <= 0 || topN <= 0 {
		return nil
	}

	// 过滤：推荐场景只保留 >0，回测场景允许 0/负分
	var allValid []PlayerCandidate
	for _, c := range candidates {
		if c.Player.Salary == 0 {
			continue
		}
		scorePower := selectionPower(c, allowNonPositive)
		if allowNonPositive || scorePower > 0 {
			allValid = append(allValid, c)
		}
	}
	if len(allValid) < pickCount {
		return nil
	}

	valid := allValid

	// 增加状态保留数量，提升阵容多样性
	// 推荐场景：保留更多状态以探索不同组合
	// 回测场景：保持较小状态限制以提高性能
	stateLimit := topN
	if !allowNonPositive {
		stateLimit = max(topN*18, 54) // 从 topN*12 提升到 topN*18
	}

	log.Printf("DP 求解: 候选球员 %d 人, 工资帽 %d, 选 %d 人, 输出 Top %d", len(valid), salaryCap, pickCount, topN)

	// dp[j][k] = 选 j 人，工资恰好为 k 时的 TopN 阵容
	dp := make([][][]lineupState, pickCount+1)
	for j := 0; j <= pickCount; j++ {
		dp[j] = make([][]lineupState, salaryCap+1)
	}
	dp[0][0] = []lineupState{{score: 0, salary: 0, indices: []int{}}}

	for i, c := range valid {
		salary := int(c.Player.Salary)
		power := selectionPower(c, allowNonPositive)
		if salary > salaryCap {
			continue
		}

		for j := pickCount; j >= 1; j-- {
			for k := salaryCap; k >= salary; k-- {
				prevStates := dp[j-1][k-salary]
				if len(prevStates) == 0 {
					continue
				}

				nextStates := dp[j][k]
				for _, prev := range prevStates {
					nextIdx := append([]int{}, prev.indices...)
					nextIdx = append(nextIdx, i)
					nextStates = insertLineupState(nextStates, lineupState{
						score:   prev.score + power,
						salary:  k,
						indices: nextIdx,
					}, stateLimit)
				}
				dp[j][k] = nextStates
			}
		}
	}

	bestStates := make([]lineupState, 0, stateLimit)
	for k := 0; k <= salaryCap; k++ {
		for _, st := range dp[pickCount][k] {
			bestStates = insertLineupState(bestStates, st, stateLimit)
		}
	}
	if len(bestStates) == 0 {
		return nil
	}

	type scoredLineup struct {
		lineup         []PlayerCandidate
		rawScore       float64
		structureScore float64
		totalSalary    int
	}

	scored := make([]scoredLineup, 0, len(bestStates))
	for _, st := range bestStates {
		lineup := make([]PlayerCandidate, 0, len(st.indices))
		for _, idx := range st.indices {
			lineup = append(lineup, valid[idx])
		}
		sort.Slice(lineup, func(i, j int) bool {
			if lineup[i].Prediction.PredictedPower == lineup[j].Prediction.PredictedPower {
				if lineup[i].Player.Salary == lineup[j].Player.Salary {
					return lineup[i].Player.NBAPlayerID < lineup[j].Player.NBAPlayerID
				}
				return lineup[i].Player.Salary < lineup[j].Player.Salary
			}
			return lineup[i].Prediction.PredictedPower > lineup[j].Prediction.PredictedPower
		})

		totalSalary := 0
		for _, c := range lineup {
			totalSalary += int(c.Player.Salary)
		}

		structureScore := st.score
		if !allowNonPositive {
			structureScore = st.score * calcLineupStructureFactor(lineup)
		}

		scored = append(scored, scoredLineup{
			lineup:         lineup,
			rawScore:       st.score,
			structureScore: structureScore,
			totalSalary:    totalSalary,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if math.Abs(scored[i].structureScore-scored[j].structureScore) > 1e-9 {
			return scored[i].structureScore > scored[j].structureScore
		}
		if math.Abs(scored[i].rawScore-scored[j].rawScore) > 1e-9 {
			return scored[i].rawScore > scored[j].rawScore
		}
		return scored[i].totalSalary < scored[j].totalSalary
	})

	results := make([][]PlayerCandidate, 0, min(topN, len(scored)))
	for _, item := range scored {
		results = append(results, item.lineup)
	}
	if len(results) > topN {
		results = results[:topN]
	}
	return results
}

func selectionPower(candidate PlayerCandidate, allowNonPositive bool) float64 {
	if allowNonPositive {
		return candidate.Prediction.PredictedPower
	}
	if candidate.Prediction.OptimizedPower > 0 {
		return candidate.Prediction.OptimizedPower
	}
	return candidate.Prediction.PredictedPower
}

func calcLineupStructureFactor(lineup []PlayerCandidate) float64 {
	// 仅在标准 5 人阵容下施加结构惩罚，避免低薪 punt 过多导致阵容稳定性差。
	if len(lineup) != defaultPickCount {
		return 1.0
	}

	cheapCount := 0
	lowSalaryHighPowerCount := 0
	explosiveCount := 0
	valueRatioCount := 0
	totalValueRatio := 0.0

	for _, c := range lineup {
		if c.Player.Salary <= 10 {
			cheapCount++
		}
		// 识别低薪高能球员（工资≤12 且预测战力≥35，或工资≤10 且预测战力≥30）
		if (c.Player.Salary <= 12 && c.Prediction.PredictedPower >= 35) ||
			(c.Player.Salary <= 10 && c.Prediction.PredictedPower >= 30) {
			lowSalaryHighPowerCount++
		}
		// 识别性价比球员（战力/工资比值≥3.0）
		if c.Player.Salary > 0 {
			valueRatio := c.Prediction.PredictedPower / float64(c.Player.Salary)
			totalValueRatio += valueRatio
			if valueRatio >= 3.0 {
				valueRatioCount++
			}
		}
		// 识别爆发型球员（Upside3≥1.35 或有爆发潜力）
		if c.Prediction.Upside3 >= 1.35 {
			explosiveCount++
		}
	}

	avgValueRatio := totalValueRatio / 5.0

	// 高性价比阵容：平均性价比≥3.5 且有 2 个以上高性价比球员，给予奖励
	if avgValueRatio >= 3.5 && valueRatioCount >= 2 {
		return 1.02
	}
	if avgValueRatio >= 3.2 && valueRatioCount >= 2 {
		return 1.0
	}

	// 有爆发型球员或高性价比球员时，减少惩罚（这些是潜在的价值 picks）
	if explosiveCount > 0 || valueRatioCount >= 2 {
		switch {
		case cheapCount <= 2:
			return 1.0
		case cheapCount == 3:
			return 0.98
		default:
			return 0.95
		}
	}

	// 有低薪高能球员时，减少惩罚（这些是价值 picks）
	if lowSalaryHighPowerCount > 0 {
		switch {
		case cheapCount <= 2:
			return 1.0
		case cheapCount == 3:
			return 0.97
		default:
			return 0.93
		}
	}

	switch {
	case cheapCount <= 1:
		return 1.0
	case cheapCount == 2:
		return 0.98
	case cheapCount == 3:
		return 0.95
	default:
		return 0.88
	}
}

type lineupState struct {
	score   float64
	salary  int
	indices []int
}

func insertLineupState(states []lineupState, candidate lineupState, limit int) []lineupState {
	for i := range states {
		if sameLineupIndices(states[i].indices, candidate.indices) {
			if lineupStateLess(candidate, states[i]) {
				states[i] = candidate
			}
			sort.Slice(states, func(a, b int) bool {
				return lineupStateLess(states[a], states[b])
			})
			if len(states) > limit {
				states = states[:limit]
			}
			return states
		}
	}

	states = append(states, candidate)
	sort.Slice(states, func(i, j int) bool {
		return lineupStateLess(states[i], states[j])
	})
	if len(states) > limit {
		states = states[:limit]
	}
	return states
}

func lineupStateLess(a, b lineupState) bool {
	if math.Abs(a.score-b.score) > 1e-9 {
		return a.score > b.score
	}
	if a.salary != b.salary {
		return a.salary < b.salary
	}
	return lexicographicallyLess(a.indices, b.indices)
}

func sameLineupIndices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func lexicographicallyLess(a, b []int) bool {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] == b[i] {
			continue
		}
		return a[i] < b[i]
	}
	return len(a) < len(b)
}

// --- 辅助函数 ---
