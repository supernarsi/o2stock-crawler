package service

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
)

func TestSolveOptimalLineupMatchesBruteForceTopN(t *testing.T) {
	svc := &LineupRecommendService{}
	candidates := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{NBAPlayerID: 1, Salary: 8}, Prediction: PlayerPrediction{PredictedPower: 24}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 2, Salary: 9}, Prediction: PlayerPrediction{PredictedPower: 23}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 3, Salary: 7}, Prediction: PlayerPrediction{PredictedPower: 20}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 4, Salary: 6}, Prediction: PlayerPrediction{PredictedPower: 18}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 5, Salary: 5}, Prediction: PlayerPrediction{PredictedPower: 15}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 6, Salary: 4}, Prediction: PlayerPrediction{PredictedPower: 14}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 7, Salary: 3}, Prediction: PlayerPrediction{PredictedPower: 10}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 8, Salary: 2}, Prediction: PlayerPrediction{PredictedPower: 8}},
	}

	got := svc.solveOptimalLineup(candidates, 20, 3, 3)
	if len(got) != 3 {
		t.Fatalf("top lineups len=%d, want 3", len(got))
	}

	wantStates := bruteForceTopLineups(candidates, 20, 3, 3)
	if len(wantStates) != 3 {
		t.Fatalf("brute top lineups len=%d, want 3", len(wantStates))
	}

	seen := make(map[string]struct{})
	for i, lineup := range got {
		if len(lineup) != 3 {
			t.Fatalf("lineup[%d] size=%d, want 3", i, len(lineup))
		}

		var totalPower float64
		totalSalary := 0
		playerKey := ""
		for _, p := range lineup {
			totalPower += p.Prediction.PredictedPower
			totalSalary += int(p.Player.Salary)
			playerKey += fmt.Sprintf("|%d", p.Player.NBAPlayerID)
		}
		if totalSalary > 20 {
			t.Fatalf("lineup[%d] totalSalary=%d exceeds cap", i, totalSalary)
		}
		if _, ok := seen[playerKey]; ok {
			t.Fatalf("lineup[%d] duplicated lineup", i)
		}
		seen[playerKey] = struct{}{}

		if math.Abs(totalPower-wantStates[i].score) > 1e-9 {
			t.Fatalf("lineup[%d] totalPower=%.2f, want %.2f", i, totalPower, wantStates[i].score)
		}
		if totalSalary != wantStates[i].salary {
			t.Fatalf("lineup[%d] totalSalary=%d, want %d", i, totalSalary, wantStates[i].salary)
		}
	}
}

func TestCalcMatchupFactorNormalizesTeamName(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{VsTeamName: "LAL", Points: 30, Rebounds: 5, Assists: 5},
		{VsTeamName: "Los Angeles Lakers", Points: 29, Rebounds: 6, Assists: 4},
		{VsTeamName: "湖人", Points: 31, Rebounds: 4, Assists: 6},
	}

	got := svc.calcMatchupFactor(stats, "湖人", 20)
	if math.Abs(got-1.10) > 1e-9 {
		t.Fatalf("matchupFactor=%.2f, want 1.10", got)
	}
}

func TestCalcMatchupFactorRequiresEnoughHistory(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{VsTeamName: "LAL", Points: 30, Rebounds: 5, Assists: 5},
		{VsTeamName: "湖人", Points: 29, Rebounds: 6, Assists: 4},
		{VsTeamName: "BOS", Points: 31, Rebounds: 4, Assists: 6},
	}

	got := svc.calcMatchupFactor(stats, "湖人", 20)
	if got != 1.0 {
		t.Fatalf("matchupFactor=%.2f, want 1.0 when history < 3", got)
	}
}

func TestCalcMatchupFactorWithContext(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{VsTeamName: "LAL", Points: 22},
		{VsTeamName: "湖人", Points: 22},
		{VsTeamName: "Los Angeles Lakers", Points: 22},
	}

	teamMap := map[string]teamMatchupMetric{
		"LAL": {DefRatingFactor: 1.12, PaceFactor: 1.08, SampleCount: 10},
	}
	dvpMap := map[string]map[uint]positionDVPMetric{
		"LAL": {1: {Factor: 1.06, SampleCount: 20}},
	}

	matchup := svc.calcMatchupFactorWithContext(
		stats,
		"湖人",
		20,
		1,
		teamMap,
		dvpMap,
	)

	if math.Abs(matchup.DefRatingFactor-1.05) > 1e-9 {
		t.Fatalf("defFactor=%.3f, want 1.05", matchup.DefRatingFactor)
	}
	if math.Abs(matchup.PaceFactor-1.0333333333) > 1e-9 {
		t.Fatalf("paceFactor=%.3f, want 1.033", matchup.PaceFactor)
	}
	if math.Abs(matchup.DvPFactor-1.05) > 1e-9 {
		t.Fatalf("dvpFactor=%.3f, want 1.05", matchup.DvPFactor)
	}
	if math.Abs(matchup.HistoryFactor-1.05) > 1e-9 {
		t.Fatalf("historyFactor=%.3f, want 1.05", matchup.HistoryFactor)
	}
	if math.Abs(matchup.OpponentFormFactor-1.0) > 1e-9 {
		t.Fatalf("opponentFormFactor=%.3f, want 1.00", matchup.OpponentFormFactor)
	}
	if math.Abs(matchup.RimDeterrenceFactor-1.0) > 1e-9 {
		t.Fatalf("rimDeterrenceFactor=%.3f, want 1.00", matchup.RimDeterrenceFactor)
	}
	if math.Abs(matchup.MatchupFactor-1.0890249999) > 1e-6 {
		t.Fatalf("matchupFactor=%.8f, want 1.08902500", matchup.MatchupFactor)
	}
}

func TestBuildTeamMatchupMetricsFromAggregates(t *testing.T) {
	rows := make([]repositories.TeamGameAggregate, 0, 12)
	for i := 0; i < 6; i++ {
		gameID := fmt.Sprintf("g%d", i+1)
		dt := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		rows = append(rows, repositories.TeamGameAggregate{
			TxGameID:       gameID,
			PlayerTeamName: "LAL",
			VsTeamName:     "BOS",
			GameDate:       dt,
			TeamPoints:     120,
		})
		rows = append(rows, repositories.TeamGameAggregate{
			TxGameID:       gameID,
			PlayerTeamName: "BOS",
			VsTeamName:     "LAL",
			GameDate:       dt,
			TeamPoints:     100,
		})
	}

	metrics := buildTeamMatchupMetricsFromAggregates(rows)
	if metrics["BOS"].SampleCount != 6 {
		t.Fatalf("BOS sample=%d, want 6", metrics["BOS"].SampleCount)
	}
	if !(metrics["BOS"].DefRatingFactor > 1.0) {
		t.Fatalf("BOS defRatingFactor=%.3f, want > 1.0", metrics["BOS"].DefRatingFactor)
	}
	if !(metrics["LAL"].DefRatingFactor < 1.0) {
		t.Fatalf("LAL defRatingFactor=%.3f, want < 1.0", metrics["LAL"].DefRatingFactor)
	}
}

func TestBuildDVPFactorMap(t *testing.T) {
	svc := &LineupRecommendService{}

	allPlayers := []entity.NBAGamePlayer{
		{NBAPlayerID: 1, Position: 1},
		{NBAPlayerID: 2, Position: 1},
	}
	txPlayerIDMap := map[uint]uint{
		1: 101,
		2: 102,
	}
	gameStatsMap := map[uint][]entity.PlayerGameStats{
		101: {
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
			{VsTeamName: "LAL", Points: 30},
		},
		102: {
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
			{VsTeamName: "BOS", Points: 10},
		},
	}

	dvpMap := svc.buildDVPFactorMap(allPlayers, txPlayerIDMap, gameStatsMap)
	if got := dvpMap["LAL"][1].Factor; math.Abs(got-1.10) > 1e-9 {
		t.Fatalf("LAL dvpFactor=%.2f, want 1.10", got)
	}
	if got := dvpMap["BOS"][1].Factor; math.Abs(got-0.92) > 1e-9 {
		t.Fatalf("BOS dvpFactor=%.2f, want 0.92", got)
	}
}

func TestCalcOpponentDefenseAnchorFactorPenalizesEliteRimProtector(t *testing.T) {
	svc := &LineupRecommendService{}

	playerFrontcourt := entity.NBAGamePlayer{
		NBAPlayerID: 1001,
		NBATeamID:   "DET",
		MatchID:     "m1",
		Position:    0,
	}
	playerGuard := entity.NBAGamePlayer{
		NBAPlayerID: 1002,
		NBATeamID:   "DET",
		MatchID:     "m1",
		Position:    1,
	}
	wembyLike := entity.NBAGamePlayer{
		NBAPlayerID: 2001,
		NBATeamID:   "SAS",
		MatchID:     "m1",
		Salary:      42,
		CombatPower: 51,
	}

	allPlayers := []entity.NBAGamePlayer{playerFrontcourt, playerGuard, wembyLike}
	txPlayerIDMap := map[uint]uint{
		2001: 9001,
	}
	gameStatsMap := map[uint][]entity.PlayerGameStats{
		9001: {
			{Blocks: 5, Steals: 2, Minutes: 34},
			{Blocks: 4, Steals: 1, Minutes: 33},
			{Blocks: 3, Steals: 2, Minutes: 35},
			{Blocks: 4, Steals: 1, Minutes: 32},
			{Blocks: 3, Steals: 2, Minutes: 34},
		},
	}

	frontcourtFactor := svc.calcOpponentDefenseAnchorFactor(playerFrontcourt, allPlayers, txPlayerIDMap, gameStatsMap)
	guardFactor := svc.calcOpponentDefenseAnchorFactor(playerGuard, allPlayers, txPlayerIDMap, gameStatsMap)

	if !(frontcourtFactor < 0.94) {
		t.Fatalf("frontcourtFactor=%.3f, want < 0.94", frontcourtFactor)
	}
	if !(guardFactor < 1.0 && guardFactor > frontcourtFactor) {
		t.Fatalf("guardFactor=%.3f, expected between (frontcourtFactor,1.0), frontcourtFactor=%.3f", guardFactor, frontcourtFactor)
	}
}

func TestResolveAvailabilityScore(t *testing.T) {
	player := entity.NBAGamePlayer{
		NBAPlayerID: 1,
		CombatPower: 35,
	}

	outScore := resolveAvailabilityScore(player, map[uint]crawler.InjuryReport{
		1: {Status: "Out"},
	})
	if outScore != 0 {
		t.Fatalf("outScore=%.2f, want 0", outScore)
	}

	questionableScore := resolveAvailabilityScore(player, map[uint]crawler.InjuryReport{
		1: {Status: "Questionable"},
	})
	if math.Abs(questionableScore-0.68) > 1e-9 {
		t.Fatalf("questionableScore=%.2f, want 0.68", questionableScore)
	}
}

func TestCalcMinutesFactorUsesSeasonBaseline(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{Minutes: 36},
		{Minutes: 35},
		{Minutes: 34},
		{Minutes: 33},
	}

	got := svc.calcMinutesFactor(stats, &entity.PlayerSeasonStats{Minutes: 30})
	if math.Abs(got-1.10) > 1e-9 {
		t.Fatalf("minutesFactor=%.2f, want 1.10", got)
	}
}

func TestCalcUsageFactorRecentIncrease(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{FieldGoalsAttempted: 24, FreeThrowsAttempted: 8, Turnovers: 4},
		{FieldGoalsAttempted: 23, FreeThrowsAttempted: 7, Turnovers: 3},
		{FieldGoalsAttempted: 22, FreeThrowsAttempted: 7, Turnovers: 3},
		{FieldGoalsAttempted: 10, FreeThrowsAttempted: 3, Turnovers: 2},
		{FieldGoalsAttempted: 11, FreeThrowsAttempted: 2, Turnovers: 2},
		{FieldGoalsAttempted: 9, FreeThrowsAttempted: 2, Turnovers: 1},
	}

	got := svc.calcUsageFactor(stats)
	if math.Abs(got-1.10) > 1e-9 {
		t.Fatalf("usageFactor=%.2f, want 1.10", got)
	}
}

func TestCalcDefenseUpsideFactor(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{Blocks: 4, Steals: 2, Minutes: 34},
		{Blocks: 3, Steals: 2, Minutes: 33},
		{Blocks: 4, Steals: 1, Minutes: 32},
		{Blocks: 3, Steals: 2, Minutes: 33},
		{Blocks: 4, Steals: 2, Minutes: 34},
	}
	season := &entity.PlayerSeasonStats{
		Blocks: 2.2,
		Steals: 1.0,
	}

	frontcourt := svc.calcDefenseUpsideFactor(stats, season, 0)
	guard := svc.calcDefenseUpsideFactor(stats, season, 1)
	if math.Abs(frontcourt-1.14) > 1e-9 {
		t.Fatalf("frontcourt=%.2f, want 1.14", frontcourt)
	}
	if math.Abs(guard-1.10) > 1e-9 {
		t.Fatalf("guard=%.2f, want 1.10", guard)
	}
}

func TestSoftenEliteFrontcourtNegativeFactors(t *testing.T) {
	player := entity.NBAGamePlayer{
		Position: 0,
		Salary:   45,
	}

	matchup, anchor := softenEliteFrontcourtNegativeFactors(
		player,
		50.7,
		0.87,
		0.94,
		1.10,
		1.10,
		1.14,
	)

	if !(matchup > 0.87 && matchup <= 1.0) {
		t.Fatalf("matchup=%.3f, want softened but <= 1.0", matchup)
	}
	if !(anchor > 0.94 && anchor <= 1.0) {
		t.Fatalf("anchor=%.3f, want softened but <= 1.0", anchor)
	}
}

func TestCalcArchetypeFactorFrontcourtValueAndCheapGuard(t *testing.T) {
	frontcourt := calcArchetypeFactor(
		entity.NBAGamePlayer{Position: 0, Salary: 15, CombatPower: 26},
		37.1,
		1.10,
		1.07,
		0.93,
		1.14,
		1.04,
		1.00,
		1.10,
	)
	cheapGuard := calcArchetypeFactor(
		entity.NBAGamePlayer{Position: 1, Salary: 10, CombatPower: 24},
		36.9,
		1.10,
		1.10,
		1.00,
		1.10,
		1.00,
		1.00,
		1.05,
	)

	if !(frontcourt > 1.0) {
		t.Fatalf("frontcourt archetype factor=%.3f, want > 1.0", frontcourt)
	}
	if !(cheapGuard < 1.0) {
		t.Fatalf("cheapGuard archetype factor=%.3f, want < 1.0", cheapGuard)
	}
}

func TestAdjustOptimizedPowerForArchetype(t *testing.T) {
	got := adjustOptimizedPowerForArchetype(
		entity.NBAGamePlayer{Position: 0, Salary: 18},
		41.9,
		37.7,
		37.1,
		1.10,
		1.07,
		0.97,
		1.08,
	)

	if !(got > 37.7 && got < 41.9) {
		t.Fatalf("adjusted optimized=%.3f, want between original and predicted", got)
	}
}

func TestCalcRoleSecurityFactorPenalizesLowMinuteRisk(t *testing.T) {
	svc := &LineupRecommendService{}
	stable := []entity.PlayerGameStats{
		{Minutes: 35},
		{Minutes: 34},
		{Minutes: 36},
		{Minutes: 33},
		{Minutes: 34},
	}
	volatile := []entity.PlayerGameStats{
		{Minutes: 28},
		{Minutes: 8},
		{Minutes: 24},
		{Minutes: 10},
		{Minutes: 7},
		{Minutes: 22},
	}

	stableFactor := svc.calcRoleSecurityFactor(stable, &entity.PlayerSeasonStats{Minutes: 33}, 30)
	volatileFactor := svc.calcRoleSecurityFactor(volatile, &entity.PlayerSeasonStats{Minutes: 24}, 10)

	if !(stableFactor > volatileFactor) {
		t.Fatalf("expected stableFactor > volatileFactor, got stable=%.2f volatile=%.2f", stableFactor, volatileFactor)
	}
	if !(volatileFactor < 0.90) {
		t.Fatalf("volatileFactor=%.2f, want < 0.90", volatileFactor)
	}
}

func TestCalcDataReliabilityFactor(t *testing.T) {
	high := calcDataReliabilityFactor(9, &entity.Player{PowerPer10: 35}, &entity.PlayerSeasonStats{Minutes: 32}, 35)
	low := calcDataReliabilityFactor(0, nil, nil, 8)
	mid := calcDataReliabilityFactor(2, &entity.Player{PowerPer10: 30}, nil, 8)

	if math.Abs(high-1.0) > 1e-9 {
		t.Fatalf("high=%.2f, want 1.00", high)
	}
	if math.Abs(low-0.42) > 1e-9 {
		t.Fatalf("low=%.2f, want 0.42", low)
	}
	if math.Abs(mid-0.76) > 1e-9 {
		t.Fatalf("mid=%.2f, want 0.76", mid)
	}
}

func TestApplyTeamExposurePenalty(t *testing.T) {
	candidates := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{NBAPlayerID: 1, TeamName: "湖人"}, Prediction: PlayerPrediction{PredictedPower: 50, OptimizedPower: 50}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 2, TeamName: "湖人"}, Prediction: PlayerPrediction{PredictedPower: 40, OptimizedPower: 40}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 3, TeamName: "湖人"}, Prediction: PlayerPrediction{PredictedPower: 29, OptimizedPower: 29}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 4, TeamName: "湖人"}, Prediction: PlayerPrediction{PredictedPower: 20, OptimizedPower: 20}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 5, TeamName: "勇士"}, Prediction: PlayerPrediction{PredictedPower: 35, OptimizedPower: 35}},
	}

	adjusted := applyTeamExposurePenalty(candidates)
	byID := make(map[uint]PlayerPrediction, len(adjusted))
	for _, c := range adjusted {
		byID[c.Player.NBAPlayerID] = c.Prediction
	}

	if math.Abs(byID[1].TeamExposureFactor-1.0) > 1e-9 || math.Abs(byID[1].OptimizedPower-50.0) > 1e-9 {
		t.Fatalf("id=1 unexpected penalty: %+v", byID[1])
	}
	if math.Abs(byID[2].TeamExposureFactor-1.0) > 1e-9 || math.Abs(byID[2].OptimizedPower-40.0) > 1e-9 {
		t.Fatalf("id=2 unexpected penalty: %+v", byID[2])
	}
	if math.Abs(byID[3].TeamExposureFactor-0.95) > 1e-9 || math.Abs(byID[3].OptimizedPower-27.55) > 1e-9 {
		t.Fatalf("id=3 unexpected penalty: %+v", byID[3])
	}
	if math.Abs(byID[4].TeamExposureFactor-0.90) > 1e-9 || math.Abs(byID[4].OptimizedPower-18.0) > 1e-9 {
		t.Fatalf("id=4 unexpected penalty: %+v", byID[4])
	}
	if math.Abs(byID[5].TeamExposureFactor-1.0) > 1e-9 || math.Abs(byID[5].OptimizedPower-35.0) > 1e-9 {
		t.Fatalf("id=5 unexpected penalty: %+v", byID[5])
	}
}

func TestApplyTeamExposurePenaltyPenalizesSecondPlayerUnderHighPressure(t *testing.T) {
	candidates := []PlayerCandidate{
		{
			Player: entity.NBAGamePlayer{NBAPlayerID: 1, TeamName: "活塞"},
			Prediction: PlayerPrediction{
				PredictedPower:      52,
				OptimizedPower:      52,
				MatchupFactor:       0.90,
				DefenseAnchorFactor: 0.90,
				RimDeterrenceFactor: 0.90,
				OpponentFormFactor:  0.90,
			},
		},
		{
			Player: entity.NBAGamePlayer{NBAPlayerID: 2, TeamName: "活塞"},
			Prediction: PlayerPrediction{
				PredictedPower:      41,
				OptimizedPower:      41,
				MatchupFactor:       0.90,
				DefenseAnchorFactor: 0.90,
				RimDeterrenceFactor: 0.90,
				OpponentFormFactor:  0.90,
			},
		},
	}

	adjusted := applyTeamExposurePenalty(candidates)
	byID := make(map[uint]PlayerPrediction, len(adjusted))
	for _, c := range adjusted {
		byID[c.Player.NBAPlayerID] = c.Prediction
	}
	if math.Abs(byID[1].TeamExposureFactor-1.0) > 1e-9 {
		t.Fatalf("id=1 unexpected penalty: %+v", byID[1])
	}
	if math.Abs(byID[2].TeamExposureFactor-0.90) > 1e-9 {
		t.Fatalf("id=2 expected second-player penalty 0.90, got %+v", byID[2])
	}
	if math.Abs(byID[2].OptimizedPower-36.9) > 1e-9 {
		t.Fatalf("id=2 optimized power expected 36.9, got %+v", byID[2])
	}
}

func TestCalcFatigueFactorBackToBack(t *testing.T) {
	svc := &LineupRecommendService{}
	stats := []entity.PlayerGameStats{
		{GameDate: time.Date(2026, 3, 4, 8, 0, 0, 0, time.UTC)},
	}

	got := svc.calcFatigueFactor(stats, "2026-03-05")
	if math.Abs(got-0.94) > 1e-9 {
		t.Fatalf("fatigueFactor=%.2f, want 0.94", got)
	}
}

func TestSelectionPower(t *testing.T) {
	c := PlayerCandidate{
		Prediction: PlayerPrediction{
			PredictedPower: 50,
			OptimizedPower: 42,
		},
	}

	if got := selectionPower(c, false); math.Abs(got-42) > 1e-9 {
		t.Fatalf("selectionPower(recommend)=%.1f, want 42", got)
	}
	if got := selectionPower(c, true); math.Abs(got-50) > 1e-9 {
		t.Fatalf("selectionPower(backtest)=%.1f, want 50", got)
	}
}

func TestCalcLineupStructureFactor(t *testing.T) {
	lineupCheap1 := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{Salary: 40}},
		{Player: entity.NBAGamePlayer{Salary: 35}},
		{Player: entity.NBAGamePlayer{Salary: 30}},
		{Player: entity.NBAGamePlayer{Salary: 25}},
		{Player: entity.NBAGamePlayer{Salary: 10}},
	}
	lineupCheap2 := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{Salary: 40}},
		{Player: entity.NBAGamePlayer{Salary: 35}},
		{Player: entity.NBAGamePlayer{Salary: 30}},
		{Player: entity.NBAGamePlayer{Salary: 10}},
		{Player: entity.NBAGamePlayer{Salary: 8}},
	}
	lineupCheap3 := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{Salary: 40}},
		{Player: entity.NBAGamePlayer{Salary: 10}},
		{Player: entity.NBAGamePlayer{Salary: 9}},
		{Player: entity.NBAGamePlayer{Salary: 8}},
		{Player: entity.NBAGamePlayer{Salary: 7}},
	}

	if got := calcLineupStructureFactor(lineupCheap1); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("factor(cheap1)=%.2f, want 1.00", got)
	}
	if got := calcLineupStructureFactor(lineupCheap2); math.Abs(got-0.97) > 1e-9 {
		t.Fatalf("factor(cheap2)=%.2f, want 0.97", got)
	}
	if got := calcLineupStructureFactor(lineupCheap3); math.Abs(got-0.92) > 1e-9 {
		t.Fatalf("factor(cheap3)=%.2f, want 0.92", got)
	}
}

func TestCalcStabilityFactorPenalizesHighVariance(t *testing.T) {
	svc := &LineupRecommendService{}
	stable := []entity.PlayerGameStats{
		{Points: 24, Rebounds: 5, Assists: 5, Steals: 1, Blocks: 1, Turnovers: 2},
		{Points: 23, Rebounds: 5, Assists: 5, Steals: 1, Blocks: 1, Turnovers: 2},
		{Points: 25, Rebounds: 5, Assists: 5, Steals: 1, Blocks: 1, Turnovers: 2},
		{Points: 24, Rebounds: 6, Assists: 4, Steals: 1, Blocks: 1, Turnovers: 2},
		{Points: 24, Rebounds: 4, Assists: 6, Steals: 1, Blocks: 1, Turnovers: 2},
	}
	volatile := []entity.PlayerGameStats{
		{Points: 45, Rebounds: 12, Assists: 10, Steals: 2, Blocks: 2, Turnovers: 6},
		{Points: 8, Rebounds: 2, Assists: 1, Steals: 0, Blocks: 0, Turnovers: 3},
		{Points: 38, Rebounds: 10, Assists: 9, Steals: 2, Blocks: 1, Turnovers: 5},
		{Points: 5, Rebounds: 2, Assists: 1, Steals: 0, Blocks: 0, Turnovers: 2},
		{Points: 32, Rebounds: 9, Assists: 8, Steals: 2, Blocks: 1, Turnovers: 4},
	}

	stableFactor := svc.calcStabilityFactor(stable)
	volatileFactor := svc.calcStabilityFactor(volatile)
	if !(stableFactor > volatileFactor) {
		t.Fatalf("expected stableFactor > volatileFactor, got stable=%.2f volatile=%.2f", stableFactor, volatileFactor)
	}
}

func TestInferSeasonByGameDate(t *testing.T) {
	got := inferSeasonByGameDate("2026-03-05")
	if got != "2025-26" {
		t.Fatalf("inferSeasonByGameDate(2026-03-05)=%s, want 2025-26", got)
	}

	got = inferSeasonByGameDate("2026-11-10")
	if got != "2026-27" {
		t.Fatalf("inferSeasonByGameDate(2026-11-10)=%s, want 2026-27", got)
	}
}

func TestParseTodayNBATotalPrepare(t *testing.T) {
	raw := []byte(`{
		"jData": {
			"playerData": {
				"sMatchData": [
					{"iMatchId":"1","iHomeTeamId":"1610612747","iAwayTeamId":"1610612738","dtDate":"2026-03-05","dtTime":"08:00:00"}
				],
				"sContestPlayer": [
					{"id":"2544","iPlayerId":"2544","iTeamId":"1610612747","sPlayerName":"勒布朗.詹姆斯","sPlayerEnName":"LeBron.James","iPosition":"0","fCombatPower":"40.3","iSalary":"45"}
				]
			}
		}
	}`)

	matches, players, err := parseTodayNBATotalPrepare(raw)
	if err != nil {
		t.Fatalf("parseTodayNBATotalPrepare() err=%v", err)
	}
	if len(matches) != 1 || len(players) != 1 {
		t.Fatalf("unexpected parse result: matches=%d players=%d", len(matches), len(players))
	}
}

func TestParseTodayNBATotalPrepareMissingSections(t *testing.T) {
	raw := []byte(`{"jData":{"playerData":{"sMatchData":[],"sContestPlayer":[]}}}`)
	if _, _, err := parseTodayNBATotalPrepare(raw); err == nil {
		t.Fatalf("expected parseTodayNBATotalPrepare() to fail for empty sections")
	}
}

func TestTeamNameByID(t *testing.T) {
	if got := teamNameByID("1610612747"); got != "湖人" {
		t.Fatalf("teamNameByID(1610612747)=%s, want 湖人", got)
	}
	if got := teamNameByID("9999"); got != "9999" {
		t.Fatalf("teamNameByID(9999)=%s, want 9999", got)
	}
}

func TestSolveOptimalLineupAllowZero(t *testing.T) {
	svc := &LineupRecommendService{}
	candidates := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{NBAPlayerID: 1, Salary: 10}, Prediction: PlayerPrediction{PredictedPower: 0}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 2, Salary: 10}, Prediction: PlayerPrediction{PredictedPower: 0}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 3, Salary: 10}, Prediction: PlayerPrediction{PredictedPower: 0}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 4, Salary: 10}, Prediction: PlayerPrediction{PredictedPower: 0}},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 5, Salary: 10}, Prediction: PlayerPrediction{PredictedPower: 0}},
	}

	got := svc.solveOptimalLineupAllowZero(candidates, 50, 5, 1)
	if len(got) != 1 || len(got[0]) != 5 {
		t.Fatalf("allow-zero lineup result invalid: %+v", got)
	}
}

func TestBuildNBAToTxPlayerIDMap(t *testing.T) {
	players := []entity.Player{
		{NBAPlayerID: 2544, TxPlayerID: 1001},
		{NBAPlayerID: 2544, TxPlayerID: 1001},
		{NBAPlayerID: 2544, TxPlayerID: 2002},
		{NBAPlayerID: 201939, TxPlayerID: 3003},
		{NBAPlayerID: 0, TxPlayerID: 4004},
	}

	got, conflictCount := buildNBAToTxPlayerIDMap(players)
	if conflictCount != 1 {
		t.Fatalf("conflictCount=%d, want 1", conflictCount)
	}
	if len(got) != 2 {
		t.Fatalf("map len=%d, want 2", len(got))
	}
	if got[2544] != 1001 {
		t.Fatalf("mapping[2544]=%d, want 1001", got[2544])
	}
	if got[201939] != 3003 {
		t.Fatalf("mapping[201939]=%d, want 3003", got[201939])
	}
}

func TestApplyManualNBATxPlayerIDOverrides(t *testing.T) {
	origin := manualNBATxPlayerIDOverrides
	manualNBATxPlayerIDOverrides = map[uint]uint{
		1631157: 196154,
	}
	defer func() { manualNBATxPlayerIDOverrides = origin }()

	nbaToTx := map[uint]uint{
		1631097: 196122,
	}
	candidateSet := map[uint]struct{}{
		1631157: {},
		1631097: {},
	}

	applied := applyManualNBATxPlayerIDOverrides(nbaToTx, candidateSet)
	if applied != 1 {
		t.Fatalf("applied=%d, want 1", applied)
	}
	if got := nbaToTx[1631157]; got != 196154 {
		t.Fatalf("nbaToTx[1631157]=%d, want 196154", got)
	}
	if got := nbaToTx[1631097]; got != 196122 {
		t.Fatalf("existing mapping should not be overridden, got %d", got)
	}
}

func TestCollectCandidateNBAPlayerIDs(t *testing.T) {
	players := []entity.NBAGamePlayer{
		{NBAPlayerID: 2544},
		{NBAPlayerID: 201939},
		{NBAPlayerID: 2544},
		{NBAPlayerID: 0},
	}

	got := collectCandidateNBAPlayerIDs(players)
	if len(got) != 2 {
		t.Fatalf("candidate ids len=%d, want 2", len(got))
	}
	set := map[uint]struct{}{}
	for _, id := range got {
		set[id] = struct{}{}
	}
	if _, ok := set[2544]; !ok {
		t.Fatalf("expected 2544 in candidate ids")
	}
	if _, ok := set[201939]; !ok {
		t.Fatalf("expected 201939 in candidate ids")
	}
}

func TestFormatBacktestPlayers(t *testing.T) {
	ids := [5]uint{201950, 1631157, 203924, 0, 1630168}
	playerMap := map[uint]entity.NBAGamePlayer{
		201950:  {NBAPlayerID: 201950, PlayerName: "朱.霍勒迪"},
		203924:  {NBAPlayerID: 203924, PlayerName: "杰拉米.格兰特"},
		1630168: {NBAPlayerID: 1630168, PlayerName: "奥孔古"},
	}

	got := formatBacktestPlayers(ids, playerMap)
	want := "201950:朱.霍勒迪, 1631157:-, 203924:杰拉米.格兰特, 1630168:奥孔古"
	if got != want {
		t.Fatalf("formatBacktestPlayers()=%q, want %q", got, want)
	}
}

func TestFormatBacktestPlayersFromRowWithTxOnlySlot(t *testing.T) {
	payload := backtestDetailPayload{
		ResultType: "actual_optimal",
		Lineup: []backtestLineupSlot{
			{Slot: 1, TxPlayerID: 88618, NBAPlayerID: 201950, PlayerName: "朱.霍勒迪", Salary: 33, ActualPower: 55.3, IDSource: "nba_mapped"},
			{Slot: 2, TxPlayerID: 196154, NBAPlayerID: 0, PlayerName: "-", Salary: 5, ActualPower: 48.6, IDSource: "tx_only"},
		},
		LineupTxPlayerIDs:  []uint{88618, 196154},
		LineupNBAPlayerIDs: []uint{201950, 0},
	}
	detailBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal detail payload err=%v", err)
	}

	row := entity.LineupBacktestResult{
		Player1ID:  201950,
		Player2ID:  0,
		DetailJSON: string(detailBytes),
	}
	playerMap := map[uint]entity.NBAGamePlayer{
		201950: {NBAPlayerID: 201950, PlayerName: "朱.霍勒迪"},
	}

	got := formatBacktestPlayersFromRow(row, playerMap)
	want := "201950:朱.霍勒迪, 196154(tx):-"
	if got != want {
		t.Fatalf("formatBacktestPlayersFromRow()=%q, want %q", got, want)
	}
}

func TestCalcAveragePowerFromStats(t *testing.T) {
	stats := []entity.PlayerGameStats{
		{Points: 30, Rebounds: 10, Assists: 5, Steals: 1, Blocks: 1, Turnovers: 2},
		{Points: 20, Rebounds: 8, Assists: 8, Steals: 2, Blocks: 0, Turnovers: 3},
		{Points: 10, Rebounds: 5, Assists: 4, Steals: 0, Blocks: 1, Turnovers: 1},
		{Points: 40, Rebounds: 12, Assists: 7, Steals: 3, Blocks: 2, Turnovers: 4},
	}

	got := calcAveragePowerFromStats(stats, 3)
	want := roundTo((calcPowerFromStats(stats[0])+calcPowerFromStats(stats[1])+calcPowerFromStats(stats[2]))/3, 1)
	if got != want {
		t.Fatalf("calcAveragePowerFromStats()=%.1f, want %.1f", got, want)
	}
}

func TestCalcRecentPowerProfile(t *testing.T) {
	stats := []entity.PlayerGameStats{
		{Points: 40, Rebounds: 10, Assists: 8, Steals: 2, Blocks: 1, Turnovers: 3},
		{Points: 18, Rebounds: 4, Assists: 5, Steals: 1, Blocks: 0, Turnovers: 2},
		{Points: 22, Rebounds: 6, Assists: 7, Steals: 1, Blocks: 1, Turnovers: 1},
		{Points: 12, Rebounds: 3, Assists: 2, Steals: 0, Blocks: 0, Turnovers: 1},
		{Points: 28, Rebounds: 8, Assists: 6, Steals: 2, Blocks: 1, Turnovers: 3},
	}

	profile := calcRecentPowerProfile(stats)
	if profile.SampleCount != 5 {
		t.Fatalf("profile.SampleCount=%d, want 5", profile.SampleCount)
	}
	if !(profile.Avg10 > 0 && profile.Avg5 > 0 && profile.Median5 > 0) {
		t.Fatalf("profile averages should all be > 0: %+v", profile)
	}
	if profile.Volatility <= 0 {
		t.Fatalf("profile.Volatility=%.3f, want > 0", profile.Volatility)
	}
}

func TestBuildRobustBaseValuePullsTowardHistoryCenter(t *testing.T) {
	profile := recentPowerProfile{
		Avg3:        48,
		Avg5:        44,
		Avg10:       41,
		Median5:     42,
		Volatility:  0.34,
		SampleCount: 8,
	}

	got := buildRobustBaseValue(60, 58, profile)
	if !(got < 60 && got > 41) {
		t.Fatalf("robust base=%.1f, want between history anchor and raw base", got)
	}
}

func TestCalibratePredictedPowerShrinksVolatileSpike(t *testing.T) {
	profile := recentPowerProfile{
		Avg3:        50,
		Avg5:        46,
		Avg10:       42,
		Median5:     43,
		Volatility:  0.44,
		SampleCount: 8,
	}

	got := calibratePredictedPower(70, 45, profile, 8)
	if !(got < 70 && got > 42) {
		t.Fatalf("calibrated=%.1f, want shrink toward history anchor", got)
	}
}

func TestCalcPredictiveFactorConfidenceDropsForVolatileLowReliability(t *testing.T) {
	volatile := calcPredictiveFactorConfidence(
		recentPowerProfile{SampleCount: 3, Volatility: 0.46},
		0.93,
		0.78,
		0.62,
	)
	stable := calcPredictiveFactorConfidence(
		recentPowerProfile{SampleCount: 10, Volatility: 0.16},
		1.01,
		1.00,
		0.98,
	)

	if !(volatile < stable) {
		t.Fatalf("volatile confidence=%.2f, stable confidence=%.2f, want volatile < stable", volatile, stable)
	}
}

func TestApplyStableStarLiftRaisesEliteStableFrontcourt(t *testing.T) {
	got := applyStableStarLift(
		entity.NBAGamePlayer{Position: 0, Salary: 45},
		49.3,
		52.0,
		1.04,
		1.03,
		1.00,
		1.07,
		0.98,
		0.94,
		0.96,
	)
	if !(got > 49.3) {
		t.Fatalf("stable star lift=%.1f, want > 49.3", got)
	}
}

func TestBacktestBenchmarkResultType(t *testing.T) {
	if got := backtestBenchmarkResultType(3); got != entity.LineupBacktestResultTypeAvg3Benchmark {
		t.Fatalf("lookback=3 resultType=%d, want %d", got, entity.LineupBacktestResultTypeAvg3Benchmark)
	}
	if got := backtestBenchmarkResultType(5); got != entity.LineupBacktestResultTypeAvg5Benchmark {
		t.Fatalf("lookback=5 resultType=%d, want %d", got, entity.LineupBacktestResultTypeAvg5Benchmark)
	}
	if got := backtestBenchmarkResultType(7); got != 0 {
		t.Fatalf("lookback=7 resultType=%d, want 0", got)
	}
}

func TestBuildBacktestRowFromCandidatesTxOnlyPersistsDetail(t *testing.T) {
	svc := &LineupRecommendService{}
	lineup := []PlayerCandidate{
		{
			Player:             entity.NBAGamePlayer{NBAPlayerID: 201950, Salary: 33},
			Prediction:         PlayerPrediction{PredictedPower: 55.3},
			BacktestTxPlayerID: 88618,
			BacktestName:       "朱.霍勒迪",
		},
		{
			Player:             entity.NBAGamePlayer{NBAPlayerID: 0, Salary: 5},
			Prediction:         PlayerPrediction{PredictedPower: 48.6},
			BacktestTxPlayerID: 196154,
			BacktestName:       "-",
		},
		{
			Player:             entity.NBAGamePlayer{NBAPlayerID: 203924, Salary: 24},
			Prediction:         PlayerPrediction{PredictedPower: 47.2},
			BacktestTxPlayerID: 225228,
			BacktestName:       "杰拉米.格兰特",
		},
		{
			Player:             entity.NBAGamePlayer{NBAPlayerID: 1630178, Salary: 45},
			Prediction:         PlayerPrediction{PredictedPower: 52.4},
			BacktestTxPlayerID: 175316,
			BacktestName:       "泰雷塞.马克西",
		},
		{
			Player:             entity.NBAGamePlayer{NBAPlayerID: 1630168, Salary: 40},
			Prediction:         PlayerPrediction{PredictedPower: 51.7},
			BacktestTxPlayerID: 175345,
			BacktestName:       "奥耶卡.奥孔古",
		},
	}

	row := svc.buildBacktestRowFromCandidates(
		"2026-03-05",
		1,
		entity.LineupBacktestResultTypeActualOptimal,
		lineup,
		"",
		0,
		-1,
	)
	if row.Player2ID != 0 {
		t.Fatalf("row.Player2ID=%d, want 0 for tx-only candidate", row.Player2ID)
	}

	var detail backtestDetailPayload
	if err := json.Unmarshal([]byte(row.DetailJSON), &detail); err != nil {
		t.Fatalf("unmarshal detail_json err=%v", err)
	}
	if len(detail.Lineup) != 5 {
		t.Fatalf("detail lineup len=%d, want 5", len(detail.Lineup))
	}
	if detail.Lineup[1].TxPlayerID != 196154 || detail.Lineup[1].NBAPlayerID != 0 {
		t.Fatalf("detail slot2 ids mismatch: %+v", detail.Lineup[1])
	}
}

func TestBuildBacktestRowFromCandidatesUsesBenchmarkResultTypeAndActualOverride(t *testing.T) {
	svc := &LineupRecommendService{}
	lineup := []PlayerCandidate{
		{Player: entity.NBAGamePlayer{NBAPlayerID: 1, Salary: 20}, Prediction: PlayerPrediction{PredictedPower: 30}, BacktestTxPlayerID: 101, BacktestName: "A"},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 2, Salary: 20}, Prediction: PlayerPrediction{PredictedPower: 28}, BacktestTxPlayerID: 102, BacktestName: "B"},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 3, Salary: 20}, Prediction: PlayerPrediction{PredictedPower: 26}, BacktestTxPlayerID: 103, BacktestName: "C"},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 4, Salary: 20}, Prediction: PlayerPrediction{PredictedPower: 24}, BacktestTxPlayerID: 104, BacktestName: "D"},
		{Player: entity.NBAGamePlayer{NBAPlayerID: 5, Salary: 20}, Prediction: PlayerPrediction{PredictedPower: 22}, BacktestTxPlayerID: 105, BacktestName: "E"},
	}

	row := svc.buildBacktestRowFromCandidates(
		"2026-03-06",
		1,
		entity.LineupBacktestResultTypeAvg3Benchmark,
		lineup,
		"3_game_average_baseline",
		130,
		118.5,
	)
	if row.TotalActualPower != 118.5 {
		t.Fatalf("row.TotalActualPower=%.1f, want 118.5", row.TotalActualPower)
	}

	var detail backtestDetailPayload
	if err := json.Unmarshal([]byte(row.DetailJSON), &detail); err != nil {
		t.Fatalf("unmarshal detail_json err=%v", err)
	}
	if detail.ResultType != "avg3_baseline" {
		t.Fatalf("detail.ResultType=%q, want avg3_baseline", detail.ResultType)
	}
	if detail.PredictedTotalPower != 130 {
		t.Fatalf("detail.PredictedTotalPower=%.1f, want 130", detail.PredictedTotalPower)
	}
}

func TestBuildAverageRecommendationCandidatesFiltersInjuryOutInBacktest(t *testing.T) {
	outPlayer := entity.NBAGamePlayer{
		NBAPlayerID:  1,
		CombatPower:  30,
		Salary:       10,
		PlayerName:   "Out Player",
		PlayerEnName: "Out.Player",
	}
	healthyPlayer := entity.NBAGamePlayer{
		NBAPlayerID:  2,
		CombatPower:  30,
		Salary:       10,
		PlayerName:   "Healthy Player",
		PlayerEnName: "Healthy.Player",
	}

	candidates := []PlayerCandidate{
		{
			Player:     outPlayer,
			Prediction: PlayerPrediction{PredictedPower: 20},
		},
		{
			Player:     healthyPlayer,
			Prediction: PlayerPrediction{PredictedPower: 25},
		},
	}

	injuryMap := map[uint]crawler.InjuryReport{
		1: {Status: "Out"},
	}

	filtered, filteredCount := filterBenchmarkCandidatesByInjury(candidates, injuryMap)
	if filteredCount != 1 {
		t.Fatalf("filteredCount=%d, want 1", filteredCount)
	}
	if len(filtered) != 1 {
		t.Fatalf("len(filtered)=%d, want 1", len(filtered))
	}

	got := filtered[0]
	if got.Player.NBAPlayerID != 2 {
		t.Fatalf("remaining player id=%d, want 2", got.Player.NBAPlayerID)
	}
	if got.Prediction.BaseValue != 25 {
		t.Fatalf("BaseValue=%.1f, want 25.0", got.Prediction.BaseValue)
	}
	if math.Abs(got.Prediction.AvailabilityScore-1.0) > 1e-9 {
		t.Fatalf("AvailabilityScore=%.2f, want 1.0", got.Prediction.AvailabilityScore)
	}
	if got.Prediction.PredictedPower != 25 {
		t.Fatalf("PredictedPower=%.1f, want 25.0", got.Prediction.PredictedPower)
	}
}

func bruteForceTopLineups(candidates []PlayerCandidate, salaryCap, pickCount, topN int) []lineupState {
	results := make([]lineupState, 0, topN)
	indices := make([]int, 0, pickCount)

	var dfs func(start, picked, salary int, score float64)
	dfs = func(start, picked, salary int, score float64) {
		if picked == pickCount {
			results = insertLineupState(results, lineupState{
				score:   score,
				salary:  salary,
				indices: append([]int{}, indices...),
			}, topN)
			return
		}
		if start >= len(candidates) {
			return
		}
		need := pickCount - picked
		if len(candidates)-start < need {
			return
		}

		for i := start; i < len(candidates); i++ {
			nextSalary := salary + int(candidates[i].Player.Salary)
			if nextSalary > salaryCap {
				continue
			}
			indices = append(indices, i)
			dfs(i+1, picked+1, nextSalary, score+candidates[i].Prediction.PredictedPower)
			indices = indices[:len(indices)-1]
		}
	}

	dfs(0, 0, 0, 0)
	return results
}
