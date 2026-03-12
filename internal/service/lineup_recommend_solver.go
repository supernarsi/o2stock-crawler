package service

import (
	"log"
	"math"
	"sort"
)

const (
	// 阵容结构评价阈值
	CheapSalaryThreshold         = 10
	HighValueSalaryThreshold1    = 12
	HighValuePowerThreshold1     = 35
	HighValueSalaryThreshold2    = 10
	HighValuePowerThreshold2     = 30
	ValueRatioThreshold          = 3.0
	ExplosiveUpsideThreshold     = 1.35
	LineupAvgValueRatioHigh      = 3.5
	LineupAvgValueRatioMid       = 3.2
	LineupHighValueCountRequired = 2
	CoreTripletPowerThreshold    = 165.0
	CoreTripletThirdMinPower     = 47.0
)

// solveOptimalLineup 在推荐模式下求解 TopN 阵容（仅使用正战力候选）。
func (s *LineupRecommendService) solveOptimalLineup(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
) [][]PlayerCandidate {
	return s.solveOptimalLineupInternal(candidates, salaryCap, pickCount, topN, false, 2)
}

func (s *LineupRecommendService) solveOptimalLineupAllowZero(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
) [][]PlayerCandidate {
	return s.solveOptimalLineupInternal(candidates, salaryCap, pickCount, topN, true, 0)
}

func (s *LineupRecommendService) solveOptimalLineupInternal(
	candidates []PlayerCandidate,
	salaryCap int,
	pickCount int,
	topN int,
	allowNonPositive bool,
	minDiversity int,
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

				var newStates []lineupState
				for _, prev := range prevStates {
					nextIdx := make([]int, len(prev.indices), len(prev.indices)+1)
					copy(nextIdx, prev.indices)
					nextIdx = append(nextIdx, i)
					newStates = append(newStates, lineupState{
						score:   prev.score + power,
						salary:  k,
						indices: nextIdx,
					})
				}

				combined := append(dp[j][k], newStates...)
				sort.Slice(combined, func(a, b int) bool {
					return lineupStateLess(combined[a], combined[b])
				})
				if len(combined) > stateLimit {
					combined = combined[:stateLimit]
				}
				dp[j][k] = combined
			}
		}
	}

	bestStates := make([]lineupState, 0)
	for k := 0; k <= salaryCap; k++ {
		bestStates = append(bestStates, dp[pickCount][k]...)
	}
	sort.Slice(bestStates, func(a, b int) bool {
		return lineupStateLess(bestStates[a], bestStates[b])
	})
	if len(bestStates) > stateLimit {
		bestStates = bestStates[:stateLimit]
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
		if minDiversity > 0 {
			if isLineupDiverseEnough(results, item.lineup, minDiversity) {
				results = append(results, item.lineup)
			}
		} else {
			results = append(results, item.lineup)
		}
		if len(results) >= topN {
			break
		}
	}
	return results
}

// isLineupDiverseEnough 检查新阵容与已选阵容集合是否具有足够的差异度。
func isLineupDiverseEnough(existing [][]PlayerCandidate, newLineup []PlayerCandidate, minDiff int) bool {
	for _, lineup := range existing {
		shared := countSharedPlayers(lineup, newLineup)
		if len(lineup)-shared < minDiff {
			return false
		}
	}
	return true
}

// countSharedPlayers 计算两套阵容中相同的球员数量。
func countSharedPlayers(l1, l2 []PlayerCandidate) int {
	m := make(map[uint]bool)
	for _, p := range l1 {
		m[p.Player.NBAPlayerID] = true
	}
	count := 0
	for _, p := range l2 {
		if m[p.Player.NBAPlayerID] {
			count++
		}
	}
	return count
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
		if c.Player.Salary <= CheapSalaryThreshold {
			cheapCount++
		}
		// 识别低薪高能球员
		if (c.Player.Salary <= HighValueSalaryThreshold1 && c.Prediction.PredictedPower >= HighValuePowerThreshold1) ||
			(c.Player.Salary <= HighValueSalaryThreshold2 && c.Prediction.PredictedPower >= HighValuePowerThreshold2) {
			lowSalaryHighPowerCount++
		}
		// 识别性价比球员
		if c.Player.Salary > 0 {
			valueRatio := c.Prediction.PredictedPower / float64(c.Player.Salary)
			totalValueRatio += valueRatio
			if valueRatio >= ValueRatioThreshold {
				valueRatioCount++
			}
		}
		// 识别爆发型球员
		if c.Prediction.Upside3 >= ExplosiveUpsideThreshold {
			explosiveCount++
		}
	}

	avgValueRatio := totalValueRatio / float64(defaultPickCount)
	coreTripletBonus := calcCoreTripletBonus(lineup)

	// 高性价比阵容
	if avgValueRatio >= LineupAvgValueRatioHigh && valueRatioCount >= LineupHighValueCountRequired {
		return 1.02 + coreTripletBonus
	}
	if avgValueRatio >= LineupAvgValueRatioMid && valueRatioCount >= LineupHighValueCountRequired {
		return 1.0 + coreTripletBonus
	}

	// 有爆发型球员或高性价比球员时，减少惩罚（这些是潜在的价值 picks）
	if explosiveCount > 0 || valueRatioCount >= 2 {
		switch {
		case cheapCount <= 2:
			return 1.0 + coreTripletBonus
		case cheapCount == 3:
			return 0.98 + coreTripletBonus
		default:
			return 0.95 + coreTripletBonus
		}
	}

	// 有低薪高能球员时，减少惩罚（这些是价值 picks）
	if lowSalaryHighPowerCount > 0 {
		switch {
		case cheapCount <= 2:
			return 1.0 + coreTripletBonus
		case cheapCount == 3:
			return 0.97 + coreTripletBonus
		default:
			return 0.93 + coreTripletBonus
		}
	}

	switch {
	case cheapCount <= 1:
		return 1.0 + coreTripletBonus
	case cheapCount == 2:
		return 0.98 + coreTripletBonus
	case cheapCount == 3:
		return 0.95 + coreTripletBonus
	default:
		return 0.88 + coreTripletBonus
	}
}

func calcCoreTripletBonus(lineup []PlayerCandidate) float64 {
	if len(lineup) < 3 {
		return 0.0
	}

	powers := make([]float64, 0, len(lineup))
	for _, candidate := range lineup {
		powers = append(powers, candidate.Prediction.PredictedPower)
	}
	sort.Slice(powers, func(i, j int) bool { return powers[i] > powers[j] })

	top3Total := powers[0] + powers[1] + powers[2]
	if top3Total < CoreTripletPowerThreshold || powers[2] < CoreTripletThirdMinPower {
		return 0.0
	}

	bonus := 0.0
	bonus += clamp((top3Total-CoreTripletPowerThreshold)*0.0008, 0.0, 0.012)
	bonus += clamp((powers[2]-CoreTripletThirdMinPower)*0.0012, 0.0, 0.006)
	return bonus
}

type lineupState struct {
	score   float64
	salary  int
	indices []int
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

// applyTeamExposurePenalty 对同队第 3 名及之后的候选球员施加惩罚，避免推荐阵容过度堆叠单队风险。
func applyTeamExposurePenalty(candidates []PlayerCandidate) []PlayerCandidate {
	if len(candidates) == 0 {
		return candidates
	}

	teamToIndexes := make(map[string][]int)
	for idx := range candidates {
		teamCode := normalizeTeamCode(candidates[idx].Player.TeamName)
		if teamCode == "" {
			teamCode = candidates[idx].Player.NBATeamID
		}
		teamToIndexes[teamCode] = append(teamToIndexes[teamCode], idx)
		candidates[idx].Prediction.TeamExposureFactor = 1.0
	}

	for _, indexes := range teamToIndexes {
		sort.Slice(indexes, func(i, j int) bool {
			left := candidates[indexes[i]].Prediction.OptimizedPower
			if left <= 0 {
				left = candidates[indexes[i]].Prediction.PredictedPower
			}
			right := candidates[indexes[j]].Prediction.OptimizedPower
			if right <= 0 {
				right = candidates[indexes[j]].Prediction.PredictedPower
			}
			if left == right {
				return candidates[indexes[i]].Player.Salary < candidates[indexes[j]].Player.Salary
			}
			return left > right
		})

		secondPower := 0.0
		if len(indexes) >= 2 {
			secondPower = candidates[indexes[1]].Prediction.OptimizedPower
			if secondPower <= 0 {
				secondPower = candidates[indexes[1]].Prediction.PredictedPower
			}
		}
		teamPressureFactor := estimateTeamPressureFactor(candidates, indexes)
		extraSecondPenalty := 1.0
		if teamPressureFactor < 0.88 {
			extraSecondPenalty = 0.92
		} else if teamPressureFactor < 0.92 {
			extraSecondPenalty = 0.96
		}

		// 识别低薪高能球员，减少惩罚
		// 使用动态阈值：工资≤15 且预测战力/工资比值≥3.0 视为价值球员
		highValueCount := 0
		for _, idx := range indexes {
			c := candidates[idx]
			valueRatio := 0.0
			if c.Player.Salary > 0 {
				valueRatio = c.Prediction.PredictedPower / float64(c.Player.Salary)
			}
			isHighValue := (c.Player.Salary <= 12 && c.Prediction.PredictedPower >= 35) ||
				(c.Player.Salary <= 15 && valueRatio >= 3.5) ||
				(c.Player.Salary <= 20 && valueRatio >= 4.0)
			if isHighValue {
				highValueCount++
			}
		}

		for rank, idx := range indexes {
			c := candidates[idx]
			valueRatio := 0.0
			if c.Player.Salary > 0 {
				valueRatio = c.Prediction.PredictedPower / float64(c.Player.Salary)
			}
			isHighValue := (c.Player.Salary <= 12 && c.Prediction.PredictedPower >= 35) ||
				(c.Player.Salary <= 15 && valueRatio >= 3.5) ||
				(c.Player.Salary <= 20 && valueRatio >= 4.0)
			hasUpside := c.Prediction.Upside3 >= 1.4
			isExplosive := isHighValue && hasUpside

			penalty := 1.0
			switch {
			case rank <= 1:
				if rank == 1 {
					if isExplosive {
						penalty = 1.0
					} else {
						penalty = extraSecondPenalty
					}
				}
			case rank == 2:
				current := candidates[idx].Prediction.OptimizedPower
				if current <= 0 {
					current = candidates[idx].Prediction.PredictedPower
				}
				if secondPower > 0 && current/secondPower < 0.75 {
					if isExplosive {
						penalty = 0.98 * extraSecondPenalty
					} else if isHighValue {
						penalty = 0.97 * extraSecondPenalty
					} else {
						penalty = 0.96 * extraSecondPenalty
					}
				} else {
					if isExplosive {
						penalty = 1.0
					} else if isHighValue {
						penalty = 0.99 * extraSecondPenalty
					} else {
						penalty = 0.98 * extraSecondPenalty
					}
				}
			case rank == 3:
				if isExplosive {
					penalty = 0.96 * extraSecondPenalty
				} else if isHighValue {
					penalty = 0.94 * extraSecondPenalty
				} else {
					penalty = 0.91 * extraSecondPenalty
				}
			default:
				if isExplosive {
					penalty = 0.92 * extraSecondPenalty
				} else if isHighValue {
					penalty = 0.88 * extraSecondPenalty
				} else {
					penalty = 0.85 * extraSecondPenalty
				}
			}

			base := candidates[idx].Prediction.OptimizedPower
			if base <= 0 {
				base = candidates[idx].Prediction.PredictedPower
			}
			candidates[idx].Prediction.TeamExposureFactor = penalty
			candidates[idx].Prediction.OptimizedPower = base * penalty
		}
	}

	return candidates
}

func estimateTeamPressureFactor(candidates []PlayerCandidate, indexes []int) float64 {
	if len(indexes) == 0 {
		return 1.0
	}

	limit := min(2, len(indexes))
	total := 0.0
	count := 0
	for i := 0; i < limit; i++ {
		pred := candidates[indexes[i]].Prediction
		matchup := pred.MatchupFactor
		if matchup <= 0 {
			matchup = 1.0
		}
		anchor := pred.DefenseAnchorFactor
		if anchor <= 0 {
			anchor = 1.0
		}
		rim := pred.RimDeterrenceFactor
		if rim <= 0 {
			rim = 1.0
		}
		form := pred.OpponentFormFactor
		if form <= 0 {
			form = 1.0
		}

		total += matchup * anchor * rim * form
		count++
	}
	if count == 0 {
		return 1.0
	}
	return clamp(total/float64(count), 0.75, 1.05)
}
