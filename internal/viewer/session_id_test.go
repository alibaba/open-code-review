package viewer

import (
	"testing"
)

func TestValidateSessionID(t *testing.T) {
	valid := []string{"run-1", "abc.def_1", "20250703"}
	for _, id := range valid {
		if err := ValidateSessionID(id); err != nil {
			t.Fatalf("ValidateSessionID(%q) = %v, want nil", id, err)
		}
	}
	invalid := []string{"", "../escape", "foo/bar", `foo\bar`, "has space"}
	for _, id := range invalid {
		if err := ValidateSessionID(id); err == nil {
			t.Fatalf("ValidateSessionID(%q) = nil, want error", id)
		}
	}
}

func TestLoadSessionRejectsTraversalSessionID(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadSession(root, "repo", "../secret"); err == nil {
		t.Fatal("LoadSession() = nil, want traversal rejection")
	}
	if _, err := LoadSession(root, "repo", "evil/nested"); err == nil {
		t.Fatal("LoadSession() = nil, want slash rejection")
	}
}
