package model

import "testing"

func TestSeverity_Rank(t *testing.T) {
	tests := []struct {
		severity Severity
		wantRank int
	}{
		{SeverityCritical, 4},
		{SeverityWarning, 3},
		{SeveritySuggestion, 2},
		{SeverityNitpick, 1},
		{Severity("unknown"), 0},
		{Severity(""), 0},
	}
	for _, tt := range tests {
		if got := tt.severity.Rank(); got != tt.wantRank {
			t.Errorf("Severity(%q).Rank() = %d, want %d", tt.severity, got, tt.wantRank)
		}
	}
}

func TestSeverity_Ordering(t *testing.T) {
	if SeverityCritical.Rank() <= SeverityWarning.Rank() {
		t.Error("critical should outrank warning")
	}
	if SeverityWarning.Rank() <= SeveritySuggestion.Rank() {
		t.Error("warning should outrank suggestion")
	}
	if SeveritySuggestion.Rank() <= SeverityNitpick.Rank() {
		t.Error("suggestion should outrank nitpick")
	}
}

func TestValidSeverity(t *testing.T) {
	valid := []string{"critical", "warning", "suggestion", "nitpick"}
	for _, s := range valid {
		if !ValidSeverity(s) {
			t.Errorf("ValidSeverity(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "high", "low", "error", "Critical"}
	for _, s := range invalid {
		if ValidSeverity(s) {
			t.Errorf("ValidSeverity(%q) = true, want false", s)
		}
	}
}
