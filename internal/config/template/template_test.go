package template

import (
	"strings"
	"testing"
)

func TestLoadDefault_FullScanBudgetParsed(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if tpl.FullScanMaxToolRequestTimes <= tpl.MaxToolRequestTimes {
		t.Errorf("FullScanMaxToolRequestTimes(%d) must exceed MaxToolRequestTimes(%d)",
			tpl.FullScanMaxToolRequestTimes, tpl.MaxToolRequestTimes)
	}
	if tpl.FullScanTask == nil || len(tpl.FullScanTask.Messages) == 0 {
		t.Fatal("FullScanTask must be populated from the embedded template")
	}
}

func TestApplyLanguage_AppliesToFullScanTask(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	tpl.ApplyLanguage("Spanish")

	for _, m := range tpl.FullScanTask.Messages {
		if m.Role != "system" {
			continue
		}
		if !strings.Contains(m.Content, "Always respond in Spanish.") {
			t.Errorf("language directive missing from FullScanTask system message")
		}
	}
}