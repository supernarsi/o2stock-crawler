package service

import (
	"fmt"
	"math"
	"testing"
	"time"

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
	dbPlayerMap := map[uint]*entity.Player{
		1: {NBAPlayerID: 1, TxPlayerID: 101},
		2: {NBAPlayerID: 2, TxPlayerID: 102},
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

	dvpMap := svc.buildDVPFactorMap(allPlayers, dbPlayerMap, gameStatsMap)
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
	dbPlayerMap := map[uint]*entity.Player{
		2001: {NBAPlayerID: 2001, TxPlayerID: 9001},
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

	frontcourtFactor := svc.calcOpponentDefenseAnchorFactor(playerFrontcourt, allPlayers, dbPlayerMap, gameStatsMap)
	guardFactor := svc.calcOpponentDefenseAnchorFactor(playerGuard, allPlayers, dbPlayerMap, gameStatsMap)

	if !(frontcourtFactor < 0.94) {
		t.Fatalf("frontcourtFactor=%.3f, want < 0.94", frontcourtFactor)
	}
	if !(guardFactor < 1.0 && guardFactor > frontcourtFactor) {
		t.Fatalf("guardFactor=%.3f, expected between (frontcourtFactor,1.0), frontcourtFactor=%.3f", guardFactor, frontcourtFactor)
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
	if math.Abs(low-0.62) > 1e-9 {
		t.Fatalf("low=%.2f, want 0.62", low)
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

func TestResolveActualFeedbackItemsOnlySupportsLineupList(t *testing.T) {
	lineupJSON := []byte(`{
		"game_date":"2026-03-04",
		"source":"manual",
		"list":[
			{"rank":1,"players":[{"nba_player_id":2544,"salary":45,"actual_power":43.1}]},
			{"rank":2,"players":[{"nba_player_id":201939,"actual_power":40.3}]}
		]
	}`)
	date, items, err := resolveActualFeedbackItems(lineupJSON)
	if err != nil {
		t.Fatalf("resolve lineup-list err=%v", err)
	}
	if date != "2026-03-04" || len(items) != 2 {
		t.Fatalf("lineup-list parse failed: date=%s items=%+v", date, items)
	}
	if items[0].Source != "manual" || items[1].Source != "manual" {
		t.Fatalf("lineup-list source propagation failed: %+v", items)
	}
	if items[0].Salary == nil || *items[0].Salary != 45 {
		t.Fatalf("lineup-list salary parse failed: %+v", items[0])
	}
	if items[0].Rank != 1 || items[1].Rank != 2 {
		t.Fatalf("lineup-list rank parse failed: %+v", items)
	}
}

func TestResolveActualFeedbackItemsRejectsNonLineupJSON(t *testing.T) {
	notSupported := []byte(`[{"nba_player_id":201939,"actual_power":39.9}]`)
	if _, _, err := resolveActualFeedbackItems(notSupported); err == nil {
		t.Fatalf("expected non-lineup json to fail")
	}
}

func TestResolveActualFeedbackItemsRejectsInvalidRank(t *testing.T) {
	invalidRank := []byte(`{
		"game_date":"2026-03-04",
		"list":[
			{"rank":4,"players":[{"nba_player_id":2544,"actual_power":43.1}]}
		]
	}`)
	if _, _, err := resolveActualFeedbackItems(invalidRank); err == nil {
		t.Fatalf("expected invalid rank to fail")
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

func TestDedupeFeedbackActualMap(t *testing.T) {
	rows := []entity.NBAGamePlayerActual{
		{Rank: 1, NBAPlayerID: 2544, ActualPower: 43.16},
		{Rank: 2, NBAPlayerID: 2544, ActualPower: 41.8},
		{Rank: 1, NBAPlayerID: 201939, ActualPower: 39.92},
	}

	got, dupCount := dedupeFeedbackActualMap(rows)
	if dupCount != 1 {
		t.Fatalf("dupCount=%d, want 1", dupCount)
	}
	if len(got) != 2 {
		t.Fatalf("map len=%d, want 2", len(got))
	}
	if got[2544] != 43.2 {
		t.Fatalf("actual[2544]=%.1f, want 43.2", got[2544])
	}
	if got[201939] != 39.9 {
		t.Fatalf("actual[201939]=%.1f, want 39.9", got[201939])
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
