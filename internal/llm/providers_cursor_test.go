package llm

import "testing"

func TestLookupProvider_CursorDetails(t *testing.T) {
	p, ok := LookupProvider("cursor")
	if !ok {
		t.Fatal("cursor not found in registry")
	}
	if p.Protocol != "cursor" {
		t.Errorf("Protocol = %q, want cursor", p.Protocol)
	}
	if p.EnvVar != "CURSOR_API_KEY" {
		t.Errorf("EnvVar = %q, want CURSOR_API_KEY", p.EnvVar)
	}
	if !p.IsCursorAgent() {
		t.Error("IsCursorAgent() = false, want true")
	}
	if p.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty for cursor agent backend", p.BaseURL)
	}
	if len(p.Models) == 0 {
		t.Fatal("expected cursor models")
	}
}
