// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os/exec"
	"testing"
)

// Review's --max-tools is a cap. Before #935 a value under the template default
// was accepted, clamped to the minimum, and then ignored, so a cost guard looked
// active while the run still used the full budget.
func TestMaxToolsCapLowersTheBudget(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Skip("git not available")
	}

	cc, err := loadCommonContext(dir, "", "", 60, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	applyMaxToolsCap(cc.Template, 60)

	if cc.Template.MaxToolRequestTimes != 60 {
		t.Errorf("MaxToolRequestTimes = %d, want 60 — --max-tools was ignored",
			cc.Template.MaxToolRequestTimes)
	}
}

func TestMaxToolsCapStillRaises(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Skip("git not available")
	}

	cc, err := loadCommonContext(dir, "", "", 500, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	applyMaxToolsCap(cc.Template, 500)

	if cc.Template.MaxToolRequestTimes != 500 {
		t.Errorf("MaxToolRequestTimes = %d, want 500", cc.Template.MaxToolRequestTimes)
	}
}

func TestMaxToolsZeroKeepsTemplateDefault(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Skip("git not available")
	}

	cc, err := loadCommonContext(dir, "", "", 0, 0, false)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	before := cc.Template.MaxToolRequestTimes
	applyMaxToolsCap(cc.Template, 0)

	if cc.Template.MaxToolRequestTimes != before {
		t.Errorf("MaxToolRequestTimes = %d, want the template default %d",
			cc.Template.MaxToolRequestTimes, before)
	}
}
