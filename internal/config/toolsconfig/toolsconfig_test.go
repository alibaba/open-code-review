package toolsconfig

import "testing"

func TestFilterByName(t *testing.T) {
	entries := []ToolConfigEntry{
		{Name: "code_search"},
		{Name: "code_graph_context"},
		{Name: "file_read"},
	}

	filtered := FilterByName(entries, "code_graph_context")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(filtered))
	}
	for _, entry := range filtered {
		if entry.Name == "code_graph_context" {
			t.Fatal("expected code_graph_context to be filtered")
		}
	}
}
