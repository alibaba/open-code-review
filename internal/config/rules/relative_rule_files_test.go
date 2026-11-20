// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewResolverCustomRuleFilesFromCurrentDirectory(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("docs", 0700); err != nil {
		t.Fatal(err)
	}
	const want = "Check the actual custom rule body."
	for _, name := range []string{"review.md", "docs/review.txt"} {
		if err := os.WriteFile(name, []byte(want+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, configPath := range []string{"rules.json", "./rules.json", "docs/../rules.json", filepath.Join(dir, "rules.json")} {
		for _, rulePath := range []string{"review.md", "./review.md", "docs/review.txt", "docs/../review.md"} {
			t.Run(configPath+"/"+rulePath, func(t *testing.T) {
				data, err := json.Marshal(ProjectRule{Rules: []ProjectRuleEntry{{Path: "**/*.go", Rule: rulePath}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile("rules.json", data, 0600); err != nil {
					t.Fatal(err)
				}
				resolver, _, err := NewResolver(dir, configPath, ResolverOptions{})
				if err != nil {
					t.Fatal(err)
				}
				detail := resolver.(DetailResolver).ResolveDetail("src/main.go")
				if detail.Source != "custom" || detail.Rule != want {
					t.Fatalf("relative rule resolved as source %q with body %.80q", detail.Source, detail.Rule)
				}
			})
		}
	}
}

func TestCustomRelativeRuleFilesStillRejectTraversal(t *testing.T) {
	setTestHome(t, t.TempDir())
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "outside.md"), []byte("outside rule"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, configPath := range []string{"rules.json", filepath.Join(dir, "rules.json")} {
		t.Run(configPath, func(t *testing.T) {
			if err := os.WriteFile("rules.json", []byte(`{"rules":[{"path":"**/*.go","rule":"../outside.md"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			resolver, _, err := NewResolver(dir, configPath, ResolverOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if detail := resolver.(DetailResolver).ResolveDetail("src/main.go"); detail.Source == "custom" || detail.Rule == "outside rule" {
				t.Fatalf("relative traversal was accepted: %+v", detail)
			}
		})
	}
}

func TestReadRuleFileWithVolumeRootBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.md")
	if err := os.WriteFile(path, []byte("root-relative rule\n"), 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(dir) + string(filepath.Separator)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := tryReadRuleFile(rel, root, ""); got == nil || *got != "root-relative rule" {
		t.Fatalf("rule within the volume root was not read: %v", got)
	}
}
