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

func TestResolveFeedbackFilePath(t *testing.T) {
	tests := []struct {
		name        string
		gameDate    string
		feedbackArg string
		want        string
	}{
		{
			name:        "default dir",
			gameDate:    "2026-03-04",
			feedbackArg: "",
			want:        "docs/data/feedback_actual/2026-03-04.json",
		},
		{
			name:        "custom dir",
			gameDate:    "2026-03-04",
			feedbackArg: "docs/data/my_feedback",
			want:        "docs/data/my_feedback/2026-03-04.json",
		},
		{
			name:        "file path",
			gameDate:    "2026-03-04",
			feedbackArg: "/tmp/a/2026-03-04.json",
			want:        "/tmp/a/2026-03-04.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFeedbackFilePath(tt.gameDate, tt.feedbackArg)
			if got != tt.want {
				t.Fatalf("resolveFeedbackFilePath() got=%s, want=%s", got, tt.want)
			}
		})
	}
}
