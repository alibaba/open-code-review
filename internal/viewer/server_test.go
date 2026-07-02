package viewer

import (
	"testing"
	"time"
)

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name string
		n    int
		s    string
		want string
	}{
		{"short string unchanged", 10, "hello", "hello"},
		{"exact length unchanged", 5, "hello", "hello"},
		{"truncated with ellipsis", 3, "hello", "hel…"},
		{"empty string", 5, "", ""},
		{"n=0 always truncates non-empty", 0, "hi", "…"},
		{"unicode shorter than n bytes", 20, "你好世界", "你好世界"},
		{"unicode truncated at byte boundary", 6, "你好世界", "你好…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.n, tt.s)
			if got != tt.want {
				t.Errorf("truncateText(%d, %q) = %q, want %q", tt.n, tt.s, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    string
	}{
		{"zero", 0, "0.0s"},
		{"sub-second", 0.5, "0.5s"},
		{"seconds only", 45.3, "45.3s"},
		{"exactly one minute", 60, "1m0s"},
		{"minutes and seconds", 125, "2m5s"},
		{"large value", 3661, "61m1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.seconds)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	input := time.Date(2025, 3, 15, 14, 30, 0, 0, cst)
	got := formatTime(input)
	want := "2025-03-15 14:30"
	if got != want {
		t.Errorf("formatTime() = %q, want %q", got, want)
	}
}

func TestFormatTime_UTC(t *testing.T) {
	input := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := formatTime(input)
	want := "2025-01-01 08:00"
	if got != want {
		t.Errorf("formatTime(UTC midnight) = %q, want %q (should be +8h)", got, want)
	}
}
