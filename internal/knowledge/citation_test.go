// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"strings"
	"testing"
)

func TestGenericMethodMoveRequiresAnUnambiguousBuildContext(t *testing.T) {
	r := newDiscoveryRepo(t)
	const original = "policy/limits_linux.go"
	const moved = "policy/capacity_linux.go"
	const code = `//go:build linux

package policy

import "math"

type Pair[K comparable, V ~int] struct{ Max V }
type Box[T ~int] struct{ Value T }
const floor, step = 0, 1

func (box Box[T]) Get() T { return box.Value }
func (p *Pair[K, V]) Allows(v V) bool {
    // Values above the configured maximum wait for a later batch.
    return float64(v) <= math.Max(float64(p.Max), floor)
}
`
	r.write(original, code)
	base := r.commit("add platform batch limit")
	method, err := base.Cite(t.Context(), original, 14, 14)
	if err != nil || method.Anchor != "method Pair.Allows" {
		t.Fatalf("generic receiver: %+v %v", method, err)
	}
	for line, name := range map[int]string{7: "type Pair", 8: "type Box", 9: "const floor,step", 11: "method Box.Get"} {
		c, err := base.Cite(t.Context(), original, line, line)
		if err != nil || c.Anchor != name {
			t.Fatalf("line %d: %+v %v, want %s", line, c, err, name)
		}
	}
	f := prepareFact(t, base, Fact{Category: CategoryBusinessRule, Authority: AuthorityConfirmed,
		Source: original, Scope: "policy", Claim: "Only values at or below the limit enter the batch.", Citations: []Citation{method}})
	r.git("mv", original, moved)
	renamed := r.commit("organize platform policy")
	got := checkedFacts(t, renamed, []Fact{f})[0]
	if got.Status != StatusCurrent || got.Citations[0].CurrentPath != moved {
		t.Fatalf("unchanged method move: %+v", got)
	}
	r.write(moved, strings.Replace(code, "//go:build linux", "//go:build linux && cgo", 1))
	restricted := r.commit("require cgo for this implementation")
	got = checkedFacts(t, restricted, []Fact{f})[0]
	if got.Status != StatusChanged || got.Citations[0].CurrentPath != moved {
		t.Fatalf("changed build directive: %+v", got)
	}
	r.write("policy/capacity_windows.go", strings.Replace(code, "//go:build linux", "//go:build windows", 1))
	variants := r.commit("add an alternative platform method")
	got = checkedFacts(t, variants, []Fact{f})[0]
	if got.Status != StatusUnverified || got.Citations[0].CurrentPath != "" {
		t.Fatalf("assigned an ambiguous move to a platform implementation: %+v", got)
	}
}

func TestRepeatedInitFallsBackToFileContext(t *testing.T) {
	r := newDiscoveryRepo(t)
	const raw = "package hooks\n\nfunc init() { register(\"fast\") }\nfunc init() { register(\"slow\") }\n"
	r.write("hooks.go", raw)
	base := r.commit("register two package hooks")
	c, err := base.Cite(t.Context(), "hooks.go", 4, 4)
	if err != nil || c.Anchor != "" || c.ContextDigest == "" {
		t.Fatalf("duplicate init declaration needs file context: %+v %v", c, err)
	}
	f := prepareFact(t, base, Fact{Category: CategoryInvariant, Authority: AuthorityConfirmed, Source: "hooks.go",
		Scope: "hooks.go", Claim: "The slow hook is registered during initialization.", Citations: []Citation{c}})
	r.write("hooks.go", strings.Replace(raw, "func init()", "\nfunc init()", 1))
	spaced := r.commit("space initialization functions")
	got := checkedFacts(t, spaced, []Fact{f})[0]
	if got.Status != StatusCurrent || got.Citations[0].CurrentStartLine != 5 {
		t.Fatalf("unique quote should follow formatting-only move: %+v", got)
	}
	r.write("hooks.go", strings.Replace(raw, `register("slow")`, `register("manual")`, 1))
	changed := r.commit("replace the slow hook")
	if c := checkedFacts(t, changed, []Fact{f})[0]; c.Status != StatusChanged {
		t.Fatalf("changed initialization credited as current: %+v", c)
	}
}

func TestPremisesRequireTraceableSupplementalSources(t *testing.T) {
	r := newDiscoveryRepo(t)
	r.write("policy.py", "def ready(count):\n    return count > 0\n")
	r.write("binary.py", "\x00data")
	base := r.commit("add the readiness policy")
	for _, tc := range []struct {
		path string
		line int
	}{{"binary.py", 1}, {"policy.py", 3}, {"unknown.py", 1}} {
		if _, err := base.Cite(t.Context(), tc.path, tc.line, tc.line); err == nil {
			t.Fatalf("accepted missing or nontext source %s:%d", tc.path, tc.line)
		}
	}
	c, err := base.Cite(t.Context(), "policy.py", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	f := Fact{Category: CategoryBusinessRule, Authority: AuthorityInferred, Source: c.Path, Scope: c.Path,
		Claim: "A nonempty queue is ready.", Citations: []Citation{c}}
	note := Citation{Kind: "discussion", Reference: "note-23", Quote: "Process only a nonempty queue.",
		StartLine: 1, EndLine: 1, ObservedAt: "2024-03-04T12:00:00Z"}
	note.Digest = Digest([]byte(note.Quote))
	for _, tc := range []struct {
		name string
		edit func(*Citation)
	}{
		{"missing identity", func(c *Citation) { c.Reference = "" }},
		{"floating revision", func(c *Citation) { c.Revision = "main" }},
		{"unbound historical commit", func(c *Citation) { c.Kind, c.Revision = "history", base.Commit }},
		{"non-source role", func(c *Citation) { c.Kind = "confirmation" }},
		{"credential-bearing URL", func(c *Citation) { c.URL = "https://user@example.org/comment/23" }},
		{"unknown observation time", func(c *Citation) { c.ObservedAt = "recently" }},
		{"false body range", func(c *Citation) { c.EndLine++ }},
		{"unversioned body", func(c *Citation) { c.Digest = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := note
			tc.edit(&bad)
			claim := f
			claim.Citations = []Citation{c, bad}
			if _, err := base.Prepare(t.Context(), claim); err == nil {
				t.Fatal("accepted invalid supplemental evidence")
			}
		})
	}
	f.Citations = []Citation{c, note}
	if valid := prepareFact(t, base, f); len(valid.Citations) != 2 {
		t.Fatal("lost legitimately versioned discussion")
	}
	f.Conditions = []Condition{{Description: "The queue is nonempty.", Citation: &note}}
	if _, err := base.Prepare(t.Context(), f); err == nil {
		t.Fatal("used historical discussion as a current condition")
	}
}
