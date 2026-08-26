// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import "testing"

func TestParseEffort_Valid(t *testing.T) {
	for _, name := range EffortNames() {
		e, err := ParseEffort(name)
		if err != nil {
			t.Errorf("ParseEffort(%q) unexpected error: %v", name, err)
		}
		if string(e) != name {
			t.Errorf("ParseEffort(%q) = %q, want %q", name, e, name)
		}
	}
}

func TestParseEffort_CaseInsensitive(t *testing.T) {
	e, err := ParseEffort("Medium")
	if err != nil {
		t.Fatalf("ParseEffort(\"Medium\") unexpected error: %v", err)
	}
	if e != EffortMedium {
		t.Errorf("ParseEffort(\"Medium\") = %q, want %q", e, EffortMedium)
	}
}

func TestParseEffort_Invalid(t *testing.T) {
	_, err := ParseEffort("extreme")
	if err == nil {
		t.Error("ParseEffort(\"extreme\") expected error, got nil")
	}
	_, err = ParseEffort("")
	if err == nil {
		t.Error("ParseEffort(\"\") expected error, got nil")
	}
}

func TestApplyEffort(t *testing.T) {
	tests := []struct {
		effort Effort
		rounds int
	}{
		{EffortLow, 1},
		{EffortMedium, 2},
		{EffortHigh, 3},
	}
	for _, tt := range tests {
		t.Run(string(tt.effort), func(t *testing.T) {
			tpl := &Template{MaxReviewRounds: 99}
			tpl.ApplyEffort(tt.effort)
			if tpl.MaxReviewRounds != tt.rounds {
				t.Errorf("ApplyEffort(%q): MaxReviewRounds = %d, want %d", tt.effort, tpl.MaxReviewRounds, tt.rounds)
			}
		})
	}
}

func TestEffortPreset_UnknownFallsToDefault(t *testing.T) {
	unknown := Effort("unknown")
	p := unknown.Preset()
	expected := EffortDefault.Preset()
	if p.MaxReviewRounds != expected.MaxReviewRounds {
		t.Errorf("unknown Effort.Preset().MaxReviewRounds = %d, want %d", p.MaxReviewRounds, expected.MaxReviewRounds)
	}
}
