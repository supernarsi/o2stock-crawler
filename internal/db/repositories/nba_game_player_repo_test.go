package repositories

import "testing"

func TestNormalizeSalarySourceGameDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "rfc3339",
			input: "2026-03-07T00:00:00+08:00",
			want:  "2026-03-07",
		},
		{
			name:  "plain_date",
			input: "2026-03-08",
			want:  "2026-03-08",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSalarySourceGameDate(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeSalarySourceGameDate(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if len(got) > 10 {
				t.Fatalf("normalizeSalarySourceGameDate(%q) returned overlong value %q", tt.input, got)
			}
		})
	}
}
