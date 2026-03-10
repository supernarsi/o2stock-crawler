package main

import "testing"

func TestParsePlayerIDs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantLen int
		wantErr bool
	}{
		{
			name:    "empty args",
			args:    []string{},
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "comma and space separated ids",
			args:    []string{"1001,1002", "1003"},
			wantLen: 3,
			wantErr: false,
		},
		{
			name:    "invalid id",
			args:    []string{"1001,abc"},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePlayerIDs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePlayerIDs() err=%v, wantErr=%v", err, tt.wantErr)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("parsePlayerIDs() len=%d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
