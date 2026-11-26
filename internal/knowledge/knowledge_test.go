// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestEvidenceAtCommitAndInDirtyWorkspace(t *testing.T) {
	ctx := t.Context()
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.org", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %s (%v)", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("contract.json", `{"properties":{"unit":{"const":"cents"}}}`)
	write("entry.go", "caller validates amount")
	write("charge.go", "charge amount")
	git("add", ".")
	git("commit", "-qm", "first")
	tree := Tree{Repo: repo, Commit: git("rev-parse", "HEAD"), Git: gitcmd.New(1)}
	facts, err := tree.Learn(ctx, "contract.json", "charge.go", "unit")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := tree.Decide(ctx, "entry.go", "charge.go", "MR-2", "add validation", "caller validates the input")
	if err != nil {
		t.Fatal(err)
	}
	facts = append(facts, decision)
	s, err := Open(ctx, filepath.Join(t.TempDir(), "db"), "repo", true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	status := func(target Tree, want ...string) {
		t.Helper()
		got, err := target.Check(ctx, facts)
		if err != nil || len(got) != len(want) {
			t.Fatalf("%v: %v", got, err)
		}
		for i := range got {
			if got[i].Status != want[i] {
				t.Fatalf("entry %d: %+v, want %s", i, got[i], want[i])
			}
		}
	}
	dirty := tree
	dirty.Workspace = true
	status(dirty, "verified", "historical")
	write("entry.go", "caller no longer validates")
	status(dirty, "verified", "changed")
	write("contract.json", `{"properties":{"unit":{"const":"dollars"}}}`)
	status(dirty, "changed", "changed")
	status(tree, "verified", "historical")
	write("contract.json", "{")
	status(dirty, "unverified", "changed")
	if err = os.Remove(filepath.Join(repo, "entry.go")); err != nil {
		t.Fatal(err)
	}
	status(dirty, "unverified", "unverified")
	if _, err = tree.Decide(ctx, "entry.go", "charge.go", "MR-2", "finding", "  "); err == nil {
		t.Fatal("accepted unexplained dismissal")
	}
	if _, err = tree.Learn(ctx, "contract.json", "charge.go", "missing"); err == nil {
		t.Fatal("learned nonexistent field")
	}
	if _, err = tree.read(ctx, "absent"); err == nil {
		t.Fatal("accepted missing blob")
	}
}

func TestConstraintsAndGroupScope(t *testing.T) {
	input := `{"properties":{"amount":{"enum":[10000000000000000001,2]},"currency":{"const":"USD"},"nested":{"properties":{"x":{"const":1}}},"ref":{"$ref":"a","const":false},"bool":true,"free":{"enum":[{}]}}}`
	cs, err := Constraints([]byte(input))
	if err != nil || len(cs) != 2 || string(cs[0].Value) != "[10000000000000000001,2]" || cs[1].Property != "currency" {
		t.Fatalf("constraints: %+v %v", cs, err)
	}
	if _, err = Constraints([]byte("invalid")); err == nil {
		t.Fatal("accepted invalid schema")
	}
	old, err := Constraints([]byte(`{"$schema":"http://json-schema.org/draft-04/schema#","properties":{"x":{"const":1,"enum":[2,1]}}}`))
	if err != nil || len(old) != 1 || old[0].Keyword != "enum" || string(old[0].Value) != "[1,2]" {
		t.Fatalf("draft-04: %+v %v", old, err)
	}
	for _, bad := range []string{"../a", ".git/config", "C:/x.json", "src/.ssh/id_rsa", "x\\y", "/b"} {
		if validPath(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
	s := Snapshot{{ID: "one", Scope: "api", Value: json.RawMessage(`"USD"`)}}
	if !strings.Contains(s.ForPaths([]string{"api/a.go"}), "one") || s.ForPaths([]string{"api2/a.go"}) != "" {
		t.Fatal("incorrect path boundary")
	}
	if s.Identity() == "" || Snapshot(nil).Identity() != "" {
		t.Fatal("invalid snapshot identity")
	}
}

func TestSQLiteIsolationAndRollback(t *testing.T) {
	ctx := t.Context()
	file := filepath.Join(t.TempDir(), "memory #1.db")
	if _, err := Open(ctx, file, "r", false); err == nil {
		t.Fatal("read-only open created a database")
	}
	s, err := Open(ctx, file, "r", true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts := []Fact{{Scope: "pkg", Property: "currency", Value: json.RawMessage(`"USD"`)}}
	if err = s.Save(ctx, facts); err != nil {
		t.Fatal(err)
	}
	invalid := []Fact{{Property: "new", Value: json.RawMessage("true")}, {Value: json.RawMessage("bad")}}
	if err = s.Save(ctx, invalid); err == nil {
		t.Fatal("save should fail atomically")
	}
	if got, err := s.List(ctx, "", ""); err != nil || len(got) != 1 {
		t.Fatalf("partial write: %+v %v", got, err)
	}
	other, err := Open(ctx, file, "other", false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if got, err := other.List(ctx, "", "currency"); err != nil || len(got) != 0 {
		t.Fatalf("cross-repo read: %+v %v", got, err)
	}
	if err = s.Revoke(ctx, "missing"); err == nil {
		t.Fatal("unexpected success revoking absent ID")
	}
	if _, err = s.List(ctx, "", "ab"); err == nil {
		t.Fatal("short search silently returned no matches")
	}
}

func TestWorkspaceRechecksEvidence(t *testing.T) {
	repo := t.TempDir()
	const p = "contract.json"
	data := []byte(`{"properties":{"unit":{"const":"cent"}}}`)
	if err := os.WriteFile(filepath.Join(repo, p), data, 0600); err != nil {
		t.Fatal(err)
	}
	tree := Tree{Repo: repo, Workspace: true}
	got, err := tree.read(t.Context(), p)
	if err != nil || string(got) != string(data) {
		t.Fatalf("read: %q %v", got, err)
	}
	if _, err = tree.read(t.Context(), "../outside"); err == nil {
		t.Fatal("accepted traversal")
	}
	if _, err = tree.read(t.Context(), "missing"); err == nil {
		t.Fatal("accepted missing source")
	}
	if _, err = tree.Learn(t.Context(), p, "api", ""); err == nil {
		t.Fatal("uncommitted input accepted for learning")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = tree.Check(ctx, []Fact{{}}); err == nil {
		t.Fatal("ignored cancellation")
	}
}
