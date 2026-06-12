package template

import (
	"strings"
	"testing"
)

func TestLoadScanDefault_BudgetParsed(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	// Scan should have a meaningfully larger per-file tool-call budget than
	// the diff-review default (30), since whole-file review usually surfaces
	// multiple findings.
	if tpl.MaxToolRequestTimes < 60 {
		t.Errorf("scan MaxToolRequestTimes(%d) should be >= 60", tpl.MaxToolRequestTimes)
	}
	if len(tpl.MainTask.Messages) == 0 {
		t.Fatal("scan MainTask must be populated from the embedded scan_template.json")
	}
	if tpl.MaxFileSizeBytes <= 0 {
		t.Errorf("scan MaxFileSizeBytes(%d) should be > 0 (defaults to 2 MiB in JSON)", tpl.MaxFileSizeBytes)
	}
}

func TestApplyLanguage_ScanTemplate(t *testing.T) {
	tpl, err := LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	tpl.ApplyLanguage("Spanish")

	for _, m := range tpl.MainTask.Messages {
		if m.Role != "system" {
			continue
		}
		if !strings.Contains(m.Content, "Always respond in Spanish.") {
			t.Errorf("language directive missing from scan MainTask system message")
		}
	}
}

func TestLoadDefault_HasNoScanFields(t *testing.T) {
	// Regression: scan fields must live exclusively in ScanTemplate so the
	// diff-review template can evolve without touching scan config.
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if len(tpl.MainTask.Messages) == 0 {
		t.Fatal("review MainTask must be populated")
	}
	if tpl.MaxToolRequestTimes <= 0 {
		t.Errorf("review MaxToolRequestTimes invalid: %d", tpl.MaxToolRequestTimes)
	}
}
