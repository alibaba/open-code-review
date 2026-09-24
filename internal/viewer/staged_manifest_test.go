// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"path/filepath"
	"testing"
)

func TestPeekSessionStagedManifestCoverage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staged.jsonl")
	writeJSONL(t, path,
		`{"type":"session_start","timestamp":"2026-09-21T00:00:00Z","review_mode":"staged"}`,
		`{"type":"session_end","files_reviewed":["unselected.go"],"run_manifest":{"schema_version":"ocr.run-manifest/v2","run_id":"staged","operation":"review","terminal_state":"partial","input":{"mode":"staged","snapshot_tree":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"coverage":{"selected":[{"item_id":"a","path":"a.go"},{"item_id":"b","path":"b.go"}],"completed":[{"item_id":"a","path":"a.go"}],"reused":[],"failed":[{"item_id":"b","path":"b.go","classification":"provider"}],"waived":[]}}}`)
	s, err := peekSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Legacy || s.Aborted || s.RunManifest == nil || s.SelectedCount != 2 || s.CompletedCount != 1 || s.FailedCount != 1 || s.TerminalState != "partial" {
		t.Fatalf("staged coverage misrepresented: %+v", s)
	}
}
