// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestResumeScopeMatchesEnumeratedSelectors(t *testing.T) {
	repo := pathSelectorRepo(t)
	cases := []struct {
		name     string
		recorded []string
		current  []string
		matches  bool
	}{
		{"omitted to dot", nil, []string{"."}, true},
		{"dot to omitted", []string{"."}, nil, true},
		{"recorded root with narrower selector", []string{"internal/scan", "."}, nil, true},
		{"current root with narrower selector", nil, []string{"././", "internal/scan"}, true},
		{"recorded repeated dot prefix", []string{"././internal/scan/"}, []string{"internal/scan"}, true},
		{"current repeated dot prefix", []string{"internal/scan"}, []string{"././internal/scan/"}, true},
		{"native separators", []string{filepath.FromSlash("./internal/scan/")}, []string{"internal/scan"}, true},
		{"empty selectors do not widen scope", []string{"", "  ", "internal/scan"}, []string{"internal/scan"}, true},
		{"selector order and duplicates", []string{"main.go", "./internal/scan/", "main.go"}, []string{"internal/scan", "main.go"}, true},
		{"whole repo to narrower scope", nil, []string{"internal/scan"}, false},
		{"narrower scope to root", []string{"internal/scan"}, []string{"."}, false},
		{"different narrow scopes", []string{"internal/scan"}, []string{"main.go"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous, err := NewProvider(repo, tc.recorded, nil, 0).Enumerate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			current, err := NewProvider(repo, tc.current, nil, 0).Enumerate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			paths := func(items []model.ScanItem) []string {
				out := make([]string, 0, len(items))
				for _, item := range items {
					out = append(out, item.Path)
				}
				sort.Strings(out)
				return out
			}
			if got := reflect.DeepEqual(paths(previous), paths(current)); got != tc.matches {
				t.Fatalf("enumerated paths %q and %q: equivalent = %v, want %v", paths(previous), paths(current), got, tc.matches)
			}

			sh := session.New(repo, "main", "test-model", session.SessionOptions{
				ReviewMode: session.ReviewModeFullScan,
				ScanPaths:  tc.recorded,
			})
			for _, item := range previous {
				sh.RecordReviewItemDone(item.Path, item.Path, item.Path, scanItemFingerprint(item), nil)
			}
			if err := sh.Finalize(); err != nil {
				t.Fatal(err)
			}
			state, err := session.LoadResumeState(repo, sh.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if !state.HasScanPathScope {
				t.Fatal("recorded scope is missing")
			}
			if err := state.ValidateScanOptions(tc.current); (err == nil) != tc.matches {
				t.Fatalf("ValidateScanOptions(%q): %v, want matching scope = %v", tc.current, err, tc.matches)
			}
			if tc.matches {
				if len(current) == 0 {
					t.Fatal("fixture must enumerate reviewable files")
				}
				for _, item := range current {
					if _, ok := state.Item(scanItemFingerprint(item)); !ok {
						t.Errorf("completed checkpoint for %q is unavailable", item.Path)
					}
				}
			}
		})
	}
}
