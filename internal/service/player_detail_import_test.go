package service

import "testing"

func TestExtractJSONObjectBlocks(t *testing.T) {
	t.Parallel()

	content := `[1] URL = https://example.com
{
  "code": 0,
  "data": [{"PlayerTemplateId":"1747","value":{"a":1}}]
}
[2] URL = https://example.com
{
  "code": 0,
  "data": [{"PlayerTemplateId":"1044"}]
}`

	blocks := extractJSONObjectBlocks(content)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestParsePlayerIDFromDetailJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantID  uint
		wantErr bool
	}{
		{
			name:    "ok",
			raw:     `{"code":0,"data":[{"PlayerTemplateId":"1747"}]}`,
			wantID:  1747,
			wantErr: false,
		},
		{
			name:    "empty_data",
			raw:     `{"code":0,"data":[]}`,
			wantErr: true,
		},
		{
			name:    "bad_player_id",
			raw:     `{"code":0,"data":[{"PlayerTemplateId":"abc"}]}`,
			wantErr: true,
		},
		{
			name:    "invalid_json",
			raw:     `{"code":0`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotID, err := parsePlayerIDFromDetailJSON(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (id=%d)", gotID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotID != tc.wantID {
				t.Fatalf("expected id %d, got %d", tc.wantID, gotID)
			}
		})
	}
}
