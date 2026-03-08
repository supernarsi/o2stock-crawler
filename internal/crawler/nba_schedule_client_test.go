package crawler

import "testing"

func TestToOfficialScheduleDateKey(t *testing.T) {
	got, err := toOfficialScheduleDateKey("2026-03-08")
	if err != nil {
		t.Fatalf("toOfficialScheduleDateKey returned error: %v", err)
	}

	want := "03/08/2026 00:00:00"
	if got != want {
		t.Fatalf("toOfficialScheduleDateKey = %q, want %q", got, want)
	}
}

func TestToOfficialScheduleDateKeyInvalid(t *testing.T) {
	if _, err := toOfficialScheduleDateKey("2026/03/08"); err == nil {
		t.Fatalf("expected error for invalid date")
	}
}
