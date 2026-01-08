package model

import (
	"testing"
	"time"
)

func TestUserPlayerOwn_ToOwnInfo(t *testing.T) {
	now := time.Date(2026, 1, 6, 23, 12, 31, 0, time.UTC)
	soldTime := time.Date(2026, 1, 8, 14, 1, 10, 0, time.UTC)

	tests := []struct {
		name string
		own  UserPlayerOwn
		want OwnInfo
	}{
		{
			name: "with sold time",
			own: UserPlayerOwn{
				PlayerID: 1056,
				PriceIn:  200025,
				PriceOut: 300010,
				OwnSta:   2,
				NumIn:    64,
				DtIn:     now,
				DtOut:    &soldTime,
			},
			want: OwnInfo{
				PlayerID: 1056,
				PriceIn:  200025,
				PriceOut: 300010,
				OwnSta:   2,
				OwnNum:   64,
				DtIn:     "2026-01-06 23:12:31",
				DtOut:    "2026-01-08 14:01:10",
			},
		},
		{
			name: "without sold time",
			own: UserPlayerOwn{
				PlayerID: 1056,
				PriceIn:  200025,
				PriceOut: 0,
				OwnSta:   1,
				NumIn:    64,
				DtIn:     now,
				DtOut:    nil,
			},
			want: OwnInfo{
				PlayerID: 1056,
				PriceIn:  200025,
				PriceOut: 0,
				OwnSta:   1,
				OwnNum:   64,
				DtIn:     "2026-01-06 23:12:31",
				DtOut:    "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.own.ToOwnInfo()
			if got.PlayerID != tt.want.PlayerID {
				t.Errorf("PlayerID = %v, want %v", got.PlayerID, tt.want.PlayerID)
			}
			if got.PriceIn != tt.want.PriceIn {
				t.Errorf("PriceIn = %v, want %v", got.PriceIn, tt.want.PriceIn)
			}
			if got.PriceOut != tt.want.PriceOut {
				t.Errorf("PriceOut = %v, want %v", got.PriceOut, tt.want.PriceOut)
			}
			if got.OwnSta != tt.want.OwnSta {
				t.Errorf("OwnSta = %v, want %v", got.OwnSta, tt.want.OwnSta)
			}
			if got.OwnNum != tt.want.OwnNum {
				t.Errorf("OwnNum = %v, want %v", got.OwnNum, tt.want.OwnNum)
			}
			if got.DtIn != tt.want.DtIn {
				t.Errorf("DtIn = %v, want %v", got.DtIn, tt.want.DtIn)
			}
			if got.DtOut != tt.want.DtOut {
				t.Errorf("DtOut = %v, want %v", got.DtOut, tt.want.DtOut)
			}
		})
	}
}
