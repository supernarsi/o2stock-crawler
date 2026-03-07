package repositories

import (
	"testing"

	"o2stock-crawler/internal/entity"
)

func TestDedupeInjurySnapshots(t *testing.T) {
	rows := []entity.NBAGameInjurySnapshot{
		{GameDate: "2026-03-08", NBAPlayerID: 1, PlayerName: "A", Status: "Questionable"},
		{GameDate: "2026-03-08", NBAPlayerID: 2, PlayerName: "B", Status: "Out"},
		{GameDate: "2026-03-08", NBAPlayerID: 1, PlayerName: "A2", Status: "Probable"},
	}

	got := dedupeInjurySnapshots(rows)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if got[0].NBAPlayerID != 1 || got[0].PlayerName != "A2" || got[0].Status != "Probable" {
		t.Fatalf("got[0]=%+v, want latest row for player 1", got[0])
	}
	if got[1].NBAPlayerID != 2 || got[1].PlayerName != "B" {
		t.Fatalf("got[1]=%+v, want player 2 unchanged", got[1])
	}
}
