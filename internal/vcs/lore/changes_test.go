// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package lore

import "testing"

func TestNormalizeChange(t *testing.T) {
	patch := "move from old name.go\nmove to new.go\n--- old name.go@1\n+++ new.go@2\n@@ -1 +1 @@\n---old\n+++new\n"
	d, err := normalizeChange("new.go", patch, "move")
	if err != nil || !d.IsRenamed || d.OldPath != "old name.go" || d.NewPath != "new.go" || d.Insertions != 1 || d.Deletions != 1 {
		t.Fatalf("move: %+v, %v", d, err)
	}
	for _, p := range []string{"Binary files differ\n", "@@ -1 +1 @@\n+\x00\n"} {
		d, err := normalizeChange("data.bin", p, "delete")
		if err != nil || !d.IsBinary || !d.IsDeleted || d.NewPath != "/dev/null" {
			t.Fatalf("binary deletion: %+v, %v", d, err)
		}
	}
	for _, test := range []struct{ path, patch, action string }{{"../a", "", "keep"}, {"a", "move from ../b\n", "move"}, {"a", "", "other"}} {
		if _, err := normalizeChange(test.path, test.patch, test.action); err == nil {
			t.Errorf("accepted %+v", test)
		}
	}
}
