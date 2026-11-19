// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
)

// --max-tools is a cap, not a floor (#935). review got this via resolveMaxTools;
// scan kept an "only raise" override, so a value below the scan template's
// default was accepted on the command line and then ignored. A cost guard that
// reports nothing and does nothing is worse than one that refuses the flag.
//
// This goes through loadScanTemplate rather than calling resolveMaxTools
// directly: the helper was already correct, and what broke was scan not using
// it. A test of the helper alone would pass with the old override back in place.
func TestScanTemplateAppliesMaxToolsAsACap(t *testing.T) {
	def, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	if def.MaxToolRequestTimes <= 1 {
		t.Fatalf("scan template default is %d, too small to test a lower cap against",
			def.MaxToolRequestTimes)
	}

	t.Run("lowers the budget", func(t *testing.T) {
		want := def.MaxToolRequestTimes - 1
		tpl, err := loadScanTemplate(want, "")
		if err != nil {
			t.Fatalf("loadScanTemplate: %v", err)
		}
		if tpl.MaxToolRequestTimes != want {
			t.Errorf("MaxToolRequestTimes = %d, want %d — --max-tools was ignored",
				tpl.MaxToolRequestTimes, want)
		}
	})

	t.Run("still raises it", func(t *testing.T) {
		want := def.MaxToolRequestTimes + 100
		tpl, err := loadScanTemplate(want, "")
		if err != nil {
			t.Fatalf("loadScanTemplate: %v", err)
		}
		if tpl.MaxToolRequestTimes != want {
			t.Errorf("MaxToolRequestTimes = %d, want %d", tpl.MaxToolRequestTimes, want)
		}
	})

	t.Run("unset keeps the template default", func(t *testing.T) {
		tpl, err := loadScanTemplate(0, "")
		if err != nil {
			t.Fatalf("loadScanTemplate: %v", err)
		}
		if tpl.MaxToolRequestTimes != def.MaxToolRequestTimes {
			t.Errorf("MaxToolRequestTimes = %d, want the template default %d",
				tpl.MaxToolRequestTimes, def.MaxToolRequestTimes)
		}
	})
}
