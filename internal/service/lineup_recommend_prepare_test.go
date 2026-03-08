package service

import (
	"testing"

	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/entity"
)

func TestBuildGamePlayersFromSchedule(t *testing.T) {
	games := []crawler.NBAScheduleGame{
		{
			GameID: "0022500921",
			HomeTeam: crawler.NBAScheduleTeam{
				TeamID: 1610612739,
			},
			AwayTeam: crawler.NBAScheduleTeam{
				TeamID: 1610612738,
			},
		},
	}

	salaryRows := []entity.NBAPlayerSalary{
		{
			NBAPlayerID:  1,
			NBATeamID:    "1610612739",
			PlayerName:   "主队球员",
			PlayerEnName: "Home Player",
			TeamName:     "骑士",
			Salary:       35,
			CombatPower:  42.5,
			Position:     1,
		},
		{
			NBAPlayerID:  2,
			NBATeamID:    "1610612738",
			PlayerName:   "客队球员",
			PlayerEnName: "Away Player",
			TeamName:     "凯尔特人",
			Salary:       38,
			CombatPower:  45.3,
			Position:     0,
		},
	}

	players, missingTeamIDs := buildGamePlayersFromSchedule("2026-03-08", games, salaryRows)
	if len(players) != 2 {
		t.Fatalf("players len = %d, want 2", len(players))
	}
	if len(missingTeamIDs) != 0 {
		t.Fatalf("missingTeamIDs = %v, want none", missingTeamIDs)
	}

	if !players[0].IsHome {
		t.Fatalf("expected first player to be home")
	}
	if players[1].IsHome {
		t.Fatalf("expected second player to be away")
	}
	if players[0].MatchID != "0022500921" || players[1].MatchID != "0022500921" {
		t.Fatalf("unexpected match ids: %+v", players)
	}
}

func TestBuildGamePlayersFromScheduleTracksMissingTeams(t *testing.T) {
	games := []crawler.NBAScheduleGame{
		{
			GameID: "g1",
			HomeTeam: crawler.NBAScheduleTeam{
				TeamID: 1610612737,
			},
			AwayTeam: crawler.NBAScheduleTeam{
				TeamID: 1610612755,
			},
		},
	}

	salaryRows := []entity.NBAPlayerSalary{
		{
			NBAPlayerID: 1,
			NBATeamID:   "1610612737",
			PlayerName:  "老鹰球员",
			Salary:      12,
		},
	}

	players, missingTeamIDs := buildGamePlayersFromSchedule("2026-03-08", games, salaryRows)
	if len(players) != 1 {
		t.Fatalf("players len = %d, want 1", len(players))
	}
	if len(missingTeamIDs) != 1 || missingTeamIDs[0] != "1610612755" {
		t.Fatalf("missingTeamIDs = %v, want [1610612755]", missingTeamIDs)
	}
}
