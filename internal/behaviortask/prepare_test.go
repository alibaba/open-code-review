// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
)

func TestPrepareFixedInput(test *testing.T) {
	input := fixture(test)
	if err := os.WriteFile(filepath.Join(input.RepoDir, "added.go"), []byte("new source\n"), 0600); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input.RepoDir, ".env"), []byte("NEW SECRET\n"), 0600); err != nil {
		test.Fatal(err)
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Add source and secret")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	if len(prepared.Diffs) != 5 || len(code.diffs) != 4 {
		test.Fatalf("changes=%+v visible=%d", prepared.Diffs, len(code.diffs))
	}
	for _, changed := range prepared.Diffs {
		if changed.NewPath == ".env" && changed.Diff != "" {
			test.Fatal("secret diff was loaded")
		}
		if changed.NewPath != ".env" && changed.Diff == "" {
			test.Fatalf("missing diff: %+v", changed)
		}
	}
	if !code.diffs["added.go"].IsNew || !code.diffs["deleted.go"].IsDeleted || !code.diffs["renamed.go"].IsRenamed {
		test.Fatalf("change kinds: %+v", code.diffs)
	}
	input.Resolution.ResolvedHead = strings.Repeat("f", 40)
	if _, err := PrepareInput(test.Context(), input); err == nil {
		test.Fatal("missing revision accepted")
	}
	ctx, cancel := context.WithCancel(test.Context())
	cancel()
	if _, err := PrepareInput(ctx, prepared); err == nil {
		test.Fatal("canceled prepare succeeded")
	}
}

func TestPrepareRootAndEmptyRange(test *testing.T) {
	input := fixture(test)
	input.Mode = "commit"
	input.Resolution = diff.InputResolution{ResolvedHead: input.Resolution.ResolvedBase, BaseIsEmpty: true}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	for _, changed := range prepared.Diffs {
		if !changed.IsNew {
			test.Fatalf("root change: %+v", changed)
		}
	}
	input.Mode = "range"
	input.Resolution = diff.InputResolution{ResolvedBase: input.Resolution.ResolvedHead, ResolvedHead: input.Resolution.ResolvedHead}
	prepared, err = PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != 0 {
		test.Fatalf("empty diff: %+v %v", prepared, err)
	}
}

func TestPrepareRenameExclusion(test *testing.T) {
	input := fixture(test)
	input.ExcludePatterns = []string{"**/consumer.go"}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	for _, changed := range code.scope {
		if changed.Renamed && changed.ExcludedReason == "" {
			test.Fatalf("rename bypassed old exclusion: %+v", changed)
		}
	}
}

func TestParseInventory(test *testing.T) {
	for _, raw := range []string{"M\x00", "\x00path\x00", "R100\x00old\x00", "X\x00path\x00"} {
		if _, err := parseInventory(raw); err == nil {
			test.Fatalf("accepted %q", raw)
		}
	}
	if changes, err := parseInventory("T\x00space name\x00"); err != nil || len(changes) != 1 || changes[0].NewPath != "space name" {
		test.Fatalf("inventory: %+v %v", changes, err)
	}
}
