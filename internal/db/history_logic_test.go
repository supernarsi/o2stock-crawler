package db

import (
	"o2stock-crawler/internal/model"
	"testing"
	"time"
)

func TestSelectDailyRecords(t *testing.T) {
	// Helper to create a row
	createRow := func(dateStr, timeStr string, price int) *model.PriceHistoryRow {
		// dateStr: "2023-01-01", timeStr: "1000" (HHMM)
		// AtDateHourStr: "202301011000"
		d, _ := time.Parse("2006-01-02", dateStr)

		dh := dateStr[0:4] + dateStr[5:7] + dateStr[8:10] + timeStr

		return &model.PriceHistoryRow{
			AtDate:           d,
			AtDateHourStr:    dh,
			PriceCurrentSale: int32(price),
		}
	}

	tests := []struct {
		name      string
		rows      []*model.PriceHistoryRow
		startDate string
		endDate   string
		expected  []int // expected prices
	}{
		{
			name:      "Mixed scenarios",
			startDate: "2023-01-01",
			endDate:   "2023-01-03",
			rows: []*model.PriceHistoryRow{
				// Day 1: 10:00, 12:00, 14:00. Should pick 12:00
				createRow("2023-01-01", "1000", 100),
				createRow("2023-01-01", "1200", 120),
				createRow("2023-01-01", "1400", 140),

				// Day 2: 09:00, 11:00. Should pick 11:00 (latest because no 12:00+)
				createRow("2023-01-02", "0900", 200),
				createRow("2023-01-02", "1100", 210),

				// Day 3: 13:00, 15:00. Should pick 13:00 (first after 12:00)
				createRow("2023-01-03", "1300", 300),
				createRow("2023-01-03", "1500", 310),
			},
			expected: []int{120, 210, 300},
		},
		{
			name:      "Missing middle day",
			startDate: "2023-01-01",
			endDate:   "2023-01-03",
			rows: []*model.PriceHistoryRow{
				// Day 1
				createRow("2023-01-01", "1000", 100),
				// Day 3
				createRow("2023-01-03", "1000", 300),
			},
			expected: []int{100, 300}, // Day 2 is skipped
		},
		{
			name:      "Empty input",
			startDate: "2023-01-01",
			endDate:   "2023-01-03",
			rows:      []*model.PriceHistoryRow{},
			expected:  []int{},
		},
		{
			name:      "Exact 1200 boundary",
			startDate: "2023-01-01",
			endDate:   "2023-01-01",
			rows: []*model.PriceHistoryRow{
				createRow("2023-01-01", "1159", 10),
				createRow("2023-01-01", "1200", 20),
				createRow("2023-01-01", "1201", 30),
			},
			expected: []int{20}, // Should pick 1200
		},
		{
			name:      "Cross Month/Year",
			startDate: "2023-12-31",
			endDate:   "2024-01-01",
			rows: []*model.PriceHistoryRow{
				// 2023-12-31
				createRow("2023-12-31", "1000", 100),
				createRow("2023-12-31", "1300", 110),
				// 2024-01-01
				createRow("2024-01-01", "1000", 200),
				createRow("2024-01-01", "1300", 210),
			},
			expected: []int{110, 210}, // Both days have >1200 data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd, _ := time.Parse("2006-01-02", tt.startDate)
			ed, _ := time.Parse("2006-01-02", tt.endDate)
			got := selectDailyRecords(tt.rows, sd, ed)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d records, got %d", len(tt.expected), len(got))
				return
			}
			for i, r := range got {
				if int(r.PriceCurrentSale) != tt.expected[i] {
					t.Errorf("record %d: expected price %d, got %d (time: %s)", i, tt.expected[i], r.PriceCurrentSale, r.AtDateHourStr)
				}
			}
		})
	}
}
