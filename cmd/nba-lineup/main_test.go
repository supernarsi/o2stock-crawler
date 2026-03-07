package main

import "testing"

func TestParseGameDateArg(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "ok", input: "2026-03-04", want: "2026-03-04"},
		{name: "empty", input: "", wantErr: true},
		{name: "invalid", input: "20260304", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGameDateArg(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGameDateArg() err=%v, wantErr=%v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parseGameDateArg() got=%s, want=%s", got, tt.want)
			}
		})
	}
}
