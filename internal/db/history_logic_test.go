package db

import (
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
	"testing"
	"time"
)

func TestSelectDailyRecords(t *testing.T) {
	repo := &repositories.HistoryRepository{}

	// Helper to create a row
	createRow := func(dateStr, timeStr string, price int) models.PlayerPriceHistory {
		d, _ := time.Parse("2006-01-02", dateStr)
		dh := dateStr[0:4] + dateStr[5:7] + dateStr[8:10] + timeStr

		return models.PlayerPriceHistory{
			AtDate:           d,
			AtDateHour:       dh,
			PriceCurrentSale: price,
		}
	}

	tests := []struct {
		name      string
		rows      []models.PlayerPriceHistory
		startDate string
		endDate   string
		expected  []int // expected prices
	}{
		{
			name:      "Mixed scenarios",
			startDate: "2023-01-01",
			endDate:   "2023-01-03",
			rows: []models.PlayerPriceHistory{
				createRow("2023-01-01", "1000", 100),
				createRow("2023-01-01", "1200", 120),
				createRow("2023-01-01", "1400", 140),
				createRow("2023-01-02", "0900", 200),
				createRow("2023-01-02", "1100", 210),
				createRow("2023-01-03", "1300", 300),
				createRow("2023-01-03", "1500", 310),
			},
			expected: []int{120, 210, 300},
		},
		{
			name:      "Missing middle day",
			startDate: "2023-01-01",
			endDate:   "2023-01-03",
			rows: []models.PlayerPriceHistory{
				createRow("2023-01-01", "1000", 100),
				createRow("2023-01-03", "1000", 300),
			},
			expected: []int{100, 300},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd, _ := time.Parse("2006-01-02", tt.startDate)
			ed, _ := time.Parse("2006-01-02", tt.endDate)
			got := repo.SelectDailyRecords(tt.rows, sd, ed)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d records, got %d", len(tt.expected), len(got))
				return
			}
			for i, r := range got {
				if r.PriceCurrentSale != tt.expected[i] {
					t.Errorf("record %d: expected price %d, got %d (time: %s)", i, tt.expected[i], r.PriceCurrentSale, r.AtDateHour)
				}
			}
		})
	}
}
