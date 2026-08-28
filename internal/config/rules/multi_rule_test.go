// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// unmarshalEntry builds a single ProjectRuleEntry from a JSON object so the
// unexported ruleInputs is populated via the real UnmarshalJSON path.
func unmarshalEntry(t *testing.T, j string) ProjectRuleEntry {
	t.Helper()
	var e ProjectRuleEntry
	if err := json.Unmarshal([]byte(j), &e); err != nil {
		t.Fatalf("unmarshal entry %q: %v", j, err)
	}
	return e
}

// unmarshalEntries builds a []ProjectRuleEntry from a JSON array of objects.
func unmarshalEntries(t *testing.T, j string) []ProjectRuleEntry {
	t.Helper()
	var es []ProjectRuleEntry
	if err := json.Unmarshal([]byte(j), &es); err != nil {
		t.Fatalf("unmarshal entries %q: %v", j, err)
	}
	return es
}

// writeProjectRule writes a .opencodereview/rule.json into repoDir.
func writeProjectRule(t *testing.T, repoDir, content string) {
	t.Helper()
	cfgDir := filepath.Join(repoDir, ".opencodereview")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "rule.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeRuleMD writes a rule markdown file into repoDir under name.
func writeRuleMD(t *testing.T, repoDir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newResolverAt builds a resolver rooted at repoDir with an isolated home so
// the developer's real ~/.opencodereview/rule.json never leaks in.
func newResolverAt(t *testing.T, repoDir string) Resolver {
	t.Helper()
	setTestHome(t, t.TempDir())
	r, _, err := NewResolver(repoDir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r
}

// captureStderr captures everything written to os.Stderr during fn by swapping
// the file and draining the pipe concurrently (an undrained pipe deadlocks
// once output exceeds the OS pipe buffer). The drain waits for fn to finish
// inside a deferred cleanup so os.Stderr is restored and the reader goroutine
// is unblocked even if fn calls t.Fatalf (runtime.Goexit) or panics.
//
// captureStderr swaps the process-global os.Stderr, so it must NOT be called
// from a test that also calls t.Parallel(): concurrent tests would race on
// os.Stderr and interleave or lose output. All current callers run serially.
func captureStderr(t *testing.T, fn func()) (out string) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = buf.ReadFrom(r)
	}()
	// Register cleanup before fn() so it runs under Goexit/panic: close the
	// writer (signals EOF to the reader), restore stderr, wait for the reader
	// to finish draining, then publish the captured output to the named
	// return. The drain must complete before out is read by the caller, so it
	// happens here in defer — not at the return expression.
	defer func() {
		_ = w.Close()
		os.Stderr = old
		<-done
		_ = r.Close()
		out = buf.String()
	}()
	fn()
	return
}

// --- ProjectRuleEntry.UnmarshalJSON (multi-rule merge) ---

func TestProjectRuleEntry_UnmarshalJSON_String(t *testing.T) {
	e := unmarshalEntry(t, `{"path":"**/*.kt","rule":"rules/kotlin.md"}`)
	if e.Rule != "rules/kotlin.md" {
		t.Errorf("Rule: want %q, got %q", "rules/kotlin.md", e.Rule)
	}
	if !reflect.DeepEqual(e.ruleInputs, []string{"rules/kotlin.md"}) {
		t.Errorf("ruleInputs: want [rules/kotlin.md], got %v", e.ruleInputs)
	}
}

func TestProjectRuleEntry_UnmarshalJSON_Array(t *testing.T) {
	e := unmarshalEntry(t, `{"path":"**/*.kt","rule":["a.md","b.md"]}`)
	if e.Rule != "" {
		t.Errorf("Rule: want empty (resolved later), got %q", e.Rule)
	}
	if !reflect.DeepEqual(e.ruleInputs, []string{"a.md", "b.md"}) {
		t.Errorf("ruleInputs: want [a.md b.md], got %v", e.ruleInputs)
	}
}

func TestProjectRuleEntry_UnmarshalJSON_SingleElementArray(t *testing.T) {
	e := unmarshalEntry(t, `{"path":"**/*.kt","rule":["a.md"]}`)
	// Single-element array degrades to the single-value path: Rule is set so it
	// is byte-identical to the string form.
	if e.Rule != "a.md" {
		t.Errorf("Rule: want %q, got %q", "a.md", e.Rule)
	}
	if !reflect.DeepEqual(e.ruleInputs, []string{"a.md"}) {
		t.Errorf("ruleInputs: want [a.md], got %v", e.ruleInputs)
	}
}

func TestProjectRuleEntry_UnmarshalJSON_Empty(t *testing.T) {
	// All of these are equivalent — an empty rule contributes nothing — so
	// each must leave Rule="" and ruleInputs=nil. The single-element array
	// [""] must match the string form "" (not keep a [""] slice, which would
	// make classifyRuleFiles emit a spurious "<inline>" RuleFiles entry).
	for _, j := range []string{
		`{"path":"**/*.kt","rule":""}`,
		`{"path":"**/*.kt"}`,
		`{"path":"**/*.kt","rule":null}`,
		`{"path":"**/*.kt","rule":[""]}`,
	} {
		e := unmarshalEntry(t, j)
		if e.Rule != "" {
			t.Errorf("%s: Rule want empty, got %q", j, e.Rule)
		}
		if e.ruleInputs != nil {
			t.Errorf("%s: ruleInputs want nil, got %v", j, e.ruleInputs)
		}
	}
}

func TestProjectRuleEntry_UnmarshalJSON_Invalid(t *testing.T) {
	for _, j := range []string{`{"path":"**/*.kt","rule":123}`, `{"path":"**/*.kt","rule":["a",1]}`, `{"path":"**/*.kt","rule":{}}`} {
		var e ProjectRuleEntry
		err := json.Unmarshal([]byte(j), &e)
		if err == nil {
			t.Errorf("%s: expected error, got nil (entry=%+v)", j, e)
			continue
		}
		// The error carries the generic prefix; the wrapped cause (checked in
		// TestProjectRuleEntry_UnmarshalJSON_MixedArrayErrorPinpointsElement)
		// adds the underlying decode detail for diagnosis.
		if !strings.Contains(err.Error(), "must be a string or array of strings") {
			t.Errorf("%s: error missing generic prefix, got %v", j, err)
		}
	}
}

// TestProjectRuleEntry_UnmarshalJSON_MixedArrayErrorPinpointsElement
// verifies a mixed-type array reports the offending element via the wrapped
// json error, not just the generic message.
func TestProjectRuleEntry_UnmarshalJSON_MixedArrayErrorPinpointsElement(t *testing.T) {
	var e ProjectRuleEntry
	err := json.Unmarshal([]byte(`{"path":"**/*.kt","rule":["a.md",42]}`), &e)
	if err == nil {
		t.Fatal("expected error for mixed-type array, got nil")
	}
	// The wrapped cause comes from the array decode, which references the
	// non-string element (the number 42) so the user can locate it.
	cause := errors.Unwrap(err)
	if cause == nil {
		t.Fatalf("error does not wrap a cause: %v", err)
	}
	if !strings.Contains(cause.Error(), "string") {
		t.Errorf("cause should mention the string type mismatch, got %q", cause.Error())
	}
}

// TestProjectRuleEntry_UnmarshalJSON_NonArrayErrorNamesOffendingType verifies
// that for a non-array invalid rule (e.g. a number), the wrapped cause is the
// string decode error — which names the actual offending type ("number") —
// rather than the array decode error that references the []string target the
// user never wrote. The generic prefix still states both accepted types.
func TestProjectRuleEntry_UnmarshalJSON_NonArrayErrorNamesOffendingType(t *testing.T) {
	for _, j := range []string{
		`{"path":"**/*.kt","rule":123}`,
		`{"path":"**/*.kt","rule":true}`,
		`{"path":"**/*.kt","rule":{}}`,
	} {
		var e ProjectRuleEntry
		err := json.Unmarshal([]byte(j), &e)
		if err == nil {
			t.Fatalf("%s: expected error, got nil", j)
		}
		if !strings.Contains(err.Error(), "must be a string or array of strings") {
			t.Errorf("%s: missing generic prefix, got %v", j, err)
		}
		cause := errors.Unwrap(err)
		if cause == nil {
			t.Fatalf("%s: error does not wrap a cause: %v", j, err)
		}
		// The cause must not reference the []string target for a non-array
		// input; the string decode error names the actual value type instead.
		if strings.Contains(cause.Error(), "[]string") {
			t.Errorf("%s: cause should not mention []string for a non-array input, got %q", j, cause.Error())
		}
	}
	// Spot-check: a number reports the number type via the string decode error.
	var e ProjectRuleEntry
	err := json.Unmarshal([]byte(`{"path":"**/*.kt","rule":123}`), &e)
	if err == nil {
		t.Fatal("expected error for rule:123, got nil")
	}
	cause := errors.Unwrap(err)
	if cause == nil {
		t.Fatalf("error does not wrap a cause: %v", err)
	}
	// "number" comes from encoding/json's error wording
	// ("cannot unmarshal number into Go value of type string"). This is a stdlib
	// implementation detail, stable since Go 1.0 but not part of any API
	// contract; if a future Go version changes the wording, update this
	// substring. We keep the assertion (rather than only checking cause != nil)
	// because this test's purpose is to verify the offending type is named.
	if !strings.Contains(cause.Error(), "number") {
		t.Errorf("number rule: cause should name the number type, got %q", cause.Error())
	}
}

// TestProjectRuleEntry_UnmarshalJSON_RoundTripAllFields guards the shadow
// struct in UnmarshalJSON (raw): it mirrors ProjectRuleEntry's exported JSON
// fields, and a field added to the entry but forgotten in raw would be
// silently dropped on decode. This marshals a fully-populated entry and
// asserts every JSON-tagged field survives a round-trip through the custom
// UnmarshalJSON. A future JSON field must be populated+asserted here, which
// then fails until the field is also added to raw — turning the sync
// requirement into a test-time enforcement rather than just a comment.
func TestProjectRuleEntry_UnmarshalJSON_RoundTripAllFields(t *testing.T) {
	orig := ProjectRuleEntry{
		Path:            "**/*.kt",
		Rule:            "check for null safety",
		MergeSystemRule: true,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ProjectRuleEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	if got.Path != orig.Path {
		t.Errorf("Path not preserved: want %q, got %q", orig.Path, got.Path)
	}
	if got.Rule != orig.Rule {
		t.Errorf("Rule not preserved: want %q, got %q", orig.Rule, got.Rule)
	}
	if got.MergeSystemRule != orig.MergeSystemRule {
		t.Errorf("MergeSystemRule not preserved: want %v, got %v",
			orig.MergeSystemRule, got.MergeSystemRule)
	}
	// The custom UnmarshalJSON back-fills ruleInputs for traceability.
	if !reflect.DeepEqual(got.ruleInputs, []string{orig.Rule}) {
		t.Errorf("ruleInputs not back-filled: want [%q], got %v", orig.Rule, got.ruleInputs)
	}
}

// TestProjectRuleEntry_UnmarshalJSON_InvalidTypeClearsStale verifies that
// decoding an invalid "rule" type (e.g. a number) into an already-populated
// entry returns an error AND clears every stale field the success path sets
// (Path, MergeSystemRule, Rule, ruleInputs), matching the contract honored by
// the shadow-decode-error branch and the absent/null/empty branches above.
// The re-decode deliberately sets merge_system_rule:true so the test would
// catch a half-cleared state: if MergeSystemRule survived, an entry with
// Rule="" and MergeSystemRule=true would NOT be skipped downstream
// (entry.Rule=="" && !entry.MergeSystemRule is false), causing an empty
// user rule to merge with the system rule — the exact hazard the full clear
// guards against should a future caller ignore the returned error.
func TestProjectRuleEntry_UnmarshalJSON_InvalidTypeClearsStale(t *testing.T) {
	// Seed with a string rule (Path/MergeSystemRule/Rule/ruleInputs all set).
	e := unmarshalEntry(t, `{"path":"**/*.kt","merge_system_rule":true,"rule":"stale.md"}`)
	if e.Rule != "stale.md" || len(e.ruleInputs) != 1 || !e.MergeSystemRule || e.Path != "**/*.kt" {
		t.Fatalf("seed not populated as expected: Path=%q MergeSystemRule=%v Rule=%q ruleInputs=%v",
			e.Path, e.MergeSystemRule, e.Rule, e.ruleInputs)
	}
	// Re-decode with an invalid rule type into the same populated entry.
	// The new JSON also sets merge_system_rule:true so a half-clear that left
	// MergeSystemRule set would be observable (not masked by a false default).
	err := json.Unmarshal([]byte(`{"path":"**/*.kt","merge_system_rule":true,"rule":123}`), &e)
	if err == nil {
		t.Fatal("expected error for invalid rule type 123, got nil")
	}
	if e.Path != "" {
		t.Errorf("stale Path leaked after invalid-type re-decode: %q", e.Path)
	}
	if e.MergeSystemRule {
		t.Errorf("stale MergeSystemRule leaked after invalid-type re-decode: %v", e.MergeSystemRule)
	}
	if e.Rule != "" {
		t.Errorf("stale Rule leaked after invalid-type re-decode: %q", e.Rule)
	}
	if e.ruleInputs != nil {
		t.Errorf("stale ruleInputs leaked after invalid-type re-decode: %v", e.ruleInputs)
	}
}

// TestProjectRuleEntry_UnmarshalJSON_ShadowDecodeErrorClearsStale verifies that
// a shadow-struct decode failure — the initial json.Unmarshal(data, &raw) in
// UnmarshalJSON, triggered by a field type mismatch such as "path":123 —
// returns an error AND clears every stale field the success path sets (Path,
// MergeSystemRule, Rule, ruleInputs), matching the contract honored by every
// other non-productive branch. The stdlib validates JSON syntax before
// dispatching to UnmarshalJSON, so a field type mismatch (not syntactically
// malformed JSON) is the realistic way this initial decode fails.
func TestProjectRuleEntry_UnmarshalJSON_ShadowDecodeErrorClearsStale(t *testing.T) {
	// Seed with a string rule plus a path and merge_system_rule so all four
	// fields are populated.
	e := unmarshalEntry(t, `{"path":"**/*.kt","rule":"stale.md","merge_system_rule":true}`)
	if e.Path != "**/*.kt" || e.Rule != "stale.md" || !e.MergeSystemRule || len(e.ruleInputs) != 1 {
		t.Fatalf("seed not populated as expected: Path=%q Rule=%q MergeSystemRule=%v ruleInputs=%v",
			e.Path, e.Rule, e.MergeSystemRule, e.ruleInputs)
	}
	// Re-decode with a "path" type mismatch into the same populated entry.
	// The shadow struct's Path is a string, so a number fails the initial
	// json.Unmarshal(data, &raw) before any field is read.
	err := json.Unmarshal([]byte(`{"path":123}`), &e)
	if err == nil {
		t.Fatal("expected error for path type mismatch, got nil")
	}
	if e.Path != "" {
		t.Errorf("stale Path leaked after shadow-decode-error re-decode: %q", e.Path)
	}
	if e.MergeSystemRule {
		t.Errorf("stale MergeSystemRule leaked after shadow-decode-error re-decode: %v", e.MergeSystemRule)
	}
	if e.Rule != "" {
		t.Errorf("stale Rule leaked after shadow-decode-error re-decode: %q", e.Rule)
	}
	if e.ruleInputs != nil {
		t.Errorf("stale ruleInputs leaked after shadow-decode-error re-decode: %v", e.ruleInputs)
	}
}

// TestProjectRuleEntry_UnmarshalJSON_ResetsStaleValues verifies that decoding a
// rule object without a "rule" field into an already-populated entry clears
// stale Rule/ruleInputs. encoding/json does not zero the receiver before
// calling UnmarshalJSON, so a conformant unmarshaler must self-initialize.
// It covers every non-productive branch: absent, null, empty string, empty
// array — each must clear stale values from a prior decode.
func TestProjectRuleEntry_UnmarshalJSON_ResetsStaleValues(t *testing.T) {
	// Seed an entry with a multi-element array (Rule="", ruleInputs populated).
	seed := func(t *testing.T) ProjectRuleEntry {
		e := unmarshalEntry(t, `{"path":"**/*.kt","rule":["a.md","b.md"]}`)
		if e.Rule != "" || len(e.ruleInputs) != 2 {
			t.Fatalf("seed entry not populated as expected: Rule=%q ruleInputs=%v", e.Rule, e.ruleInputs)
		}
		return e
	}
	// Also seed with a string rule (Rule set) to catch stale Rule leakage.
	seedString := func(t *testing.T) ProjectRuleEntry {
		e := unmarshalEntry(t, `{"path":"**/*.kt","rule":"stale.md"}`)
		if e.Rule != "stale.md" || len(e.ruleInputs) != 1 {
			t.Fatalf("seed string entry not populated as expected: Rule=%q ruleInputs=%v", e.Rule, e.ruleInputs)
		}
		return e
	}
	cases := []struct {
		name string
		json string
	}{
		{"absent", `{"path":"**/*.kt"}`},
		{"null", `{"path":"**/*.kt","rule":null}`},
		{"empty string", `{"path":"**/*.kt","rule":""}`},
		{"empty array", `{"path":"**/*.kt","rule":[]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// From an array-seeded entry: stale ruleInputs must be cleared.
			e := seed(t)
			if err := json.Unmarshal([]byte(c.json), &e); err != nil {
				t.Fatalf("re-decode %s: %v", c.name, err)
			}
			if e.Rule != "" {
				t.Errorf("%s: stale Rule leaked: %q", c.name, e.Rule)
			}
			if e.ruleInputs != nil {
				t.Errorf("%s: stale ruleInputs leaked: %v", c.name, e.ruleInputs)
			}
			// From a string-seeded entry: stale Rule must be cleared.
			e2 := seedString(t)
			if err := json.Unmarshal([]byte(c.json), &e2); err != nil {
				t.Fatalf("re-decode %s (string seed): %v", c.name, err)
			}
			if e2.Rule != "" {
				t.Errorf("%s: stale Rule leaked from string seed: %q", c.name, e2.Rule)
			}
			if e2.ruleInputs != nil {
				t.Errorf("%s: stale ruleInputs leaked from string seed: %v", c.name, e2.ruleInputs)
			}
		})
	}
}

// --- resolveRuleEntries merge path ---

func TestResolveRuleEntries_MultiFileMerge(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	writeRuleMD(t, dir, "b.md", "Kotlin-specific guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","b.md"]}]`)
	resolveRuleEntries(entries, dir, "")

	want := "General review guidance.\n\n---\n\nKotlin-specific guidance."
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
}

func TestResolveRuleEntries_SingleElementNoSeparator(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md"]}]`)
	resolveRuleEntries(entries, dir, "")

	// Single-element array degrades to the single-value path: no heading, no
	// separator, byte-identical to "rule":"a.md".
	if entries[0].Rule != "General review guidance." {
		t.Errorf("Rule: want %q, got %q", "General review guidance.", entries[0].Rule)
	}
}

// TestResolveRuleEntries_SubdirPathResolves verifies that a relative
// subdirectory path in the rule array (e.g. "rules/kotlin.md") resolves
// correctly against the repo root and its content merges with the other
// segments, separated by "---" with no per-file headings.
func TestResolveRuleEntries_SubdirPathResolves(t *testing.T) {
	dir := t.TempDir()
	// general.md at repo root, kotlin.md under rules/
	if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuleMD(t, dir, "general.md", "General review guidance.\n")
	writeRuleMD(t, dir, filepath.Join("rules", "kotlin.md"), "Kotlin-specific guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["general.md","rules/kotlin.md"]}]`)
	resolveRuleEntries(entries, dir, "")

	want := "General review guidance.\n\n---\n\nKotlin-specific guidance."
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
}

func TestResolveRuleEntries_MixedInlineAndFile(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","extra: check null safety"]}]`)
	resolveRuleEntries(entries, dir, "")

	want := "General review guidance.\n\n---\n\nextra: check null safety"
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
}

func TestResolveRuleEntries_AllInline(t *testing.T) {
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["first inline rule","second inline rule"]}]`)
	resolveRuleEntries(entries, t.TempDir(), "")

	want := "first inline rule\n\n---\n\nsecond inline rule"
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
}

// TestResolveRuleEntries_EmptyElementWarned verifies an empty/whitespace array
// element (e.g. from a trailing comma) is skipped but warned about, so a
// merge with fewer segments than expected stays diagnosable.
func TestResolveRuleEntries_EmptyElementWarned(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	writeRuleMD(t, dir, "b.md", "Kotlin-specific guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","","  ","b.md"]}]`)

	out := captureStderr(t, func() {
		resolveRuleEntries(entries, dir, "")
	})
	// Two empty elements ("" at index 1, "  " at index 2) => two warnings,
	// each carrying the element index and the owning entry's path so the
	// user can locate which rule entry the skipped element belongs to.
	if c := strings.Count(out, "empty rule input skipped in array"); c != 2 {
		t.Errorf("empty-input warnings: want 2, got %d (stderr=%q)", c, out)
	}
	if !strings.Contains(out, "at index 1 (path: **/*.kt)") {
		t.Errorf("warning missing index 1 / path context, got:\n%s", out)
	}
	if !strings.Contains(out, "at index 2 (path: **/*.kt)") {
		t.Errorf("warning missing index 2 / path context, got:\n%s", out)
	}
	// The two empty elements are skipped; a.md and b.md still merge.
	want := "General review guidance.\n\n---\n\nKotlin-specific guidance."
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
}

// TestResolveRuleEntries_EmptyPathWarningOmitsPathSuffix verifies that when a
// rule entry omits the "path" field (or carries a whitespace-only one —
// UnmarshalJSON stores path verbatim without trimming), none of the resolve
// warnings emit a confusing "(path: )" / "(path:   )" suffix. tryReadRuleFile
// already appends the entry path conditionally; the cap/empty-element/empty-file
// warnings now share that conditional, TrimSpace-guarded form so an empty or
// whitespace path produces a clean message with no dangling suffix. (A missing
// "path" is unusual but valid: the entry still matches by pattern defaults
// downstream.)
func TestResolveRuleEntries_EmptyPathWarningOmitsPathSuffix(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")

	t.Run("empty path empty-element warning", func(t *testing.T) {
		// No "path" field => entryPath == "" during resolution. Only 3 inputs,
		// well under the count cap, so only the empty-element warning fires.
		entries := unmarshalEntries(t, `[{"rule":["a.md","","  "]}]`)
		out := captureStderr(t, func() {
			resolveRuleEntries(entries, dir, "")
		})
		if !strings.Contains(out, "empty rule input skipped in array at index 1") {
			t.Errorf("missing empty-input warning for index 1, got:\n%s", out)
		}
		if strings.Contains(out, "(path: )") {
			t.Errorf("warning leaked a confusing \"(path: )\" suffix for an entry with no path, got:\n%s", out)
		}
	})

	t.Run("empty path count-cap warning", func(t *testing.T) {
		// No "path" field => entryPath == "". Exceed maxRuleInputs so the
		// count-cap warning fires; it must NOT leak "(path: )".
		n := maxRuleInputs + 1
		inputs := make([]string, n)
		for i := range inputs {
			inputs[i] = fmt.Sprintf("inline rule %04d", i+1)
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		entries := unmarshalEntries(t, fmt.Sprintf(`[{"rule":%s}]`, ruleJSON))
		out := captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})
		if !strings.Contains(out, "only the first") {
			t.Errorf("missing count-cap warning, got:\n%s", out)
		}
		if strings.Contains(out, "(path: )") {
			t.Errorf("count-cap warning leaked \"(path: )\" for an entry with no path, got:\n%s", out)
		}
	})

	t.Run("whitespace path no suffix on any warning", func(t *testing.T) {
		// A whitespace-only "path" => entryPath == "  ". UnmarshalJSON does
		// not trim it, so the TrimSpace guard (not a bare != "" check) is
		// what prevents a "(path:   )" suffix. Trigger the empty-element
		// warning and the count-cap warning together.
		n := maxRuleInputs + 1
		inputs := make([]string, n)
		inputs[0] = "" // empty element fires the empty-element warning
		for i := 1; i < n; i++ {
			inputs[i] = fmt.Sprintf("inline rule %04d", i)
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		entries := unmarshalEntries(t, fmt.Sprintf(`[{"path":"  ","rule":%s}]`, ruleJSON))
		out := captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})
		if !strings.Contains(out, "empty rule input skipped in array at index 0") {
			t.Errorf("missing empty-input warning for index 0, got:\n%s", out)
		}
		if !strings.Contains(out, "only the first") {
			t.Errorf("missing count-cap warning, got:\n%s", out)
		}
		if strings.Contains(out, "(path:   )") || strings.Contains(out, "(path: )") {
			t.Errorf("warning leaked a whitespace/empty path suffix, got:\n%s", out)
		}
	})
}

// TestResolveRuleEntries_TotalSizeCap verifies resolveMultiRuleParts bounds the
// merged body to maxRuleBodySize (the same 512 KB budget as a single rule file)
// rather than relying on the input count alone. The count is first capped to
// maxRuleInputs by the caller; within the survivors, once the running total of
// segment content plus "\n\n---\n\n" separators would exceed the budget, the
// overflowing input is skipped from the Rule body with a warning (the running
// total is unchanged, so a later smaller input may still fit the budget and
// contribute), but ruleInputs is left as the (already count-capped) survivors
// so "Rule Files:" reports what was processed. Each part is kept whole — never
// mid-file/mid-string truncated.
func TestResolveRuleEntries_TotalSizeCap(t *testing.T) {
	t.Run("under budget merges all without warning", func(t *testing.T) {
		// A few small inline inputs (well under maxRuleInputs and well under
		// 512 KB) all merge with no cap or size warning.
		n := 10
		inputs := make([]string, n)
		for i := range inputs {
			inputs[i] = fmt.Sprintf("inline rule %04d", i+1)
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		out := captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})
		if strings.Contains(out, "merged rule body exceeds") {
			t.Errorf("no size warning expected under budget, got:\n%s", out)
		}
		if strings.Contains(out, "only the first") {
			t.Errorf("no count-cap warning expected under maxRuleInputs, got:\n%s", out)
		}
		if !strings.Contains(entries[0].Rule, fmt.Sprintf("inline rule %04d", n)) {
			t.Errorf("expected last element %d in Rule, got:\n%s", n, entries[0].Rule)
		}
		// ruleInputs keeps all inputs (under the count cap, none dropped).
		if got := len(entries[0].ruleInputs); got != n {
			t.Errorf("ruleInputs want %d, got %d", n, got)
		}
		files := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule)
		if got := len(files); got != n {
			t.Errorf("classifyRuleFiles want %d labels, got %d", n, got)
		}
	})

	t.Run("over budget skips overflowing input with warning", func(t *testing.T) {
		// Three 200000-byte inline inputs. part1=200000 (total 200000),
		// part2=200000+7 (total 400007), part3 delta=200007 -> 600014 >
		// 524288: part3 is skipped (the running total is unchanged by the
		// skip), and there is nothing after it, so body = parts 1+2. 3 inputs
		// is under maxRuleInputs, so no count cap fires; the size cap skips
		// part3 only.
		big := strings.Repeat("a", 200000)
		inputs := []string{big, big, big}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		out := captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})
		wantWarn := fmt.Sprintf("merged rule body exceeds %d bytes at input index 2; this input skipped (path: **/*.kt)", maxRuleBodySize)
		if !strings.Contains(out, wantWarn) {
			t.Errorf("size warning missing, got:\n%s", out)
		}
		// Body holds parts 1 and 2 only: 200000 + len(rulePartSep) + 200000 bytes.
		// Use len(rulePartSep) — not the literal 7 — so this assertion tracks the
		// shared separator constant rather than hardcoding its current length.
		rule := entries[0].Rule
		if got, want := len(rule), 200000+len(rulePartSep)+200000; got != want {
			t.Errorf("Rule body length want %d, got %d", want, got)
		}
		// ruleInputs keeps all 3 (under the count cap; the size cap skips from
		// the body only, not ruleInputs).
		if got := len(entries[0].ruleInputs); got != 3 {
			t.Errorf("ruleInputs want 3, got %d", got)
		}
		files := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule)
		if got := len(files); got != 3 {
			t.Errorf("classifyRuleFiles want 3 labels, got %d", got)
		}
	})

	t.Run("huge inline element skipped, later smaller input kept", func(t *testing.T) {
		// 2-element array where the first element alone exceeds the budget:
		// it is skipped whole (no truncation, total unchanged), so the second
		// (small) input still fits the budget and is kept. ruleInputs lists
		// both (under the count cap).
		huge := strings.Repeat("b", maxRuleBodySize+1)
		inputs := []string{huge, "second"}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		out := captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})
		wantWarn := fmt.Sprintf("merged rule body exceeds %d bytes at input index 0; this input skipped (path: **/*.kt)", maxRuleBodySize)
		if !strings.Contains(out, wantWarn) {
			t.Errorf("size warning missing, got:\n%s", out)
		}
		if entries[0].Rule != "second" {
			t.Errorf("overflowing first part should be skipped, keeping the second; got %q", entries[0].Rule)
		}
		if got := len(entries[0].ruleInputs); got != 2 {
			t.Errorf("ruleInputs want 2, got %d", got)
		}
	})

	t.Run("huge first input skipped, later small inputs still fit", func(t *testing.T) {
		// The motivating scenario for `continue` over `break`: a huge first
		// input would cause a `break` to drop everything after it, but with
		// `continue` the huge part is skipped (total unchanged) and the two
		// small inputs that follow still fit the budget and are kept.
		// 2 inputs is under maxRuleInputs, so no count cap fires.
		huge := strings.Repeat("c", maxRuleBodySize+1)
		small := strings.Repeat("d", 10000) // 10 KB
		inputs := []string{huge, small, small}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		out := captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})
		// Only the huge input overflows; it is skipped at index 0. The two
		// small inputs fit (0+10000, then 10000+10007), so no further warning.
		wantWarn := fmt.Sprintf("merged rule body exceeds %d bytes at input index 0; this input skipped (path: **/*.kt)", maxRuleBodySize)
		if !strings.Contains(out, wantWarn) {
			t.Errorf("size warning for index 0 missing, got:\n%s", out)
		}
		// Body holds the two small inputs joined by one separator.
		rule := entries[0].Rule
		if got, want := len(rule), 10000+len(rulePartSep)+10000; got != want {
			t.Errorf("Rule body length want %d, got %d", want, got)
		}
		// ruleInputs keeps all 3 (under the count cap; the size cap skips from
		// the body only, not ruleInputs).
		if got := len(entries[0].ruleInputs); got != 3 {
			t.Errorf("ruleInputs want 3, got %d", got)
		}
		files := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule)
		if got := len(files); got != 3 {
			t.Errorf("classifyRuleFiles want 3 labels, got %d", got)
		}
	})
}

// TestResolveRuleEntries_InputCountCap verifies the maxRuleInputs count cap:
// resolveRuleEntries caps a multi-input array's inputs to maxRuleInputs BEFORE
// any file I/O, so at most maxRuleInputs files are read per entry and ruleInputs
// retains at most maxRuleInputs elements. Every input — valid file, inline text,
// or an empty "" element — counts toward the limit. Inputs beyond the cap are
// dropped from both the resolved Rule body and ruleInputs (with a warning naming
// the first dropped index), so they cost no I/O and do not appear in "Rule Files:".
// This bounds both the file-read I/O and the retained ruleInputs memory that the
// byte cap maxRuleBodySize alone cannot reach (empty/missing/empty-file inputs are
// skipped without growing the running total).
func TestResolveRuleEntries_InputCountCap(t *testing.T) {
	t.Run("inputs beyond cap are dropped before I/O", func(t *testing.T) {
		// maxRuleInputs+20 real files. Only the first maxRuleInputs are read;
		// the rest are dropped before any file I/O, with a single warning
		// naming the first dropped index (maxRuleInputs). ruleInputs is capped
		// to maxRuleInputs, so "Rule Files:" lists only the resolved files.
		dir := t.TempDir()
		n := maxRuleInputs + 20
		inputs := make([]string, n)
		for i := range inputs {
			name := fmt.Sprintf("rule%04d.md", i)
			writeRuleMD(t, dir, name, fmt.Sprintf("guidance %d\n", i))
			inputs[i] = name
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		warned := captureStderr(t, func() {
			resolveRuleEntries(entries, dir, "")
		})
		// The count-cap warning names the first dropped index (maxRuleInputs)
		// and the total/original count.
		wantWarn := fmt.Sprintf("rule array has %d inputs; only the first %d are resolved, the rest (from index %d) are dropped (path: **/*.kt)", n, maxRuleInputs, maxRuleInputs)
		if !strings.Contains(warned, wantWarn) {
			t.Errorf("count-cap warning missing, want:\n%s\ngot:\n%s", wantWarn, warned)
		}
		// ruleInputs is capped to maxRuleInputs.
		if got := len(entries[0].ruleInputs); got != maxRuleInputs {
			t.Errorf("ruleInputs want %d (capped), got %d", maxRuleInputs, got)
		}
		// The last retained file (index maxRuleInputs-1) is in the body; the
		// first dropped file (index maxRuleInputs) is not.
		if !strings.Contains(entries[0].Rule, fmt.Sprintf("guidance %d", maxRuleInputs-1)) {
			t.Errorf("expected last retained file guidance %d in Rule", maxRuleInputs-1)
		}
		if strings.Contains(entries[0].Rule, fmt.Sprintf("guidance %d", maxRuleInputs)) {
			t.Errorf("first dropped file guidance %d should not be in Rule", maxRuleInputs)
		}
		// classifyRuleFiles reflects the capped ruleInputs.
		files := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule)
		if got := len(files); got != maxRuleInputs {
			t.Errorf("classifyRuleFiles want %d labels, got %d", maxRuleInputs, got)
		}
	})

	t.Run("empty elements count toward the cap", func(t *testing.T) {
		// 31 empty "" elements followed by 32 real files (63 total). Empty
		// elements count toward maxRuleInputs (32) but contribute nothing to
		// the body, so only the inputs within the first 32 are processed: the
		// 31 empties plus the 1st real file. The 1st real file is the only one
		// read; the other 31 real files (indices 32-62) are dropped by the cap
		// before any I/O. This is the user's own misconfiguration (padding with
		// junk starves their own budget), so it is left to them to fix.
		dir := t.TempDir()
		var inputs []string
		for i := 0; i < 31; i++ {
			inputs = append(inputs, "")
		}
		for i := 0; i < 32; i++ {
			name := fmt.Sprintf("real%04d.md", i)
			writeRuleMD(t, dir, name, fmt.Sprintf("real guidance %d\n", i))
			inputs = append(inputs, name)
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		warned := captureStderr(t, func() {
			resolveRuleEntries(entries, dir, "")
		})
		// The count cap fires at index maxRuleInputs (32), dropping inputs
		// 32-62. ruleInputs is capped to 32 (31 empties + real0000.md).
		wantWarn := fmt.Sprintf("rule array has %d inputs; only the first %d are resolved, the rest (from index %d) are dropped (path: **/*.kt)", len(inputs), maxRuleInputs, maxRuleInputs)
		if !strings.Contains(warned, wantWarn) {
			t.Errorf("count-cap warning missing, want:\n%s\ngot:\n%s", wantWarn, warned)
		}
		if got := len(entries[0].ruleInputs); got != maxRuleInputs {
			t.Errorf("ruleInputs want %d (capped), got %d", maxRuleInputs, got)
		}
		// Only real0000.md was read (the one real file within the first 32);
		// its content is the body. The 31 empties contributed nothing.
		if !strings.Contains(entries[0].Rule, "real guidance 0") {
			t.Errorf("expected real0000.md content in Rule, got:\n%s", entries[0].Rule)
		}
		if strings.Contains(entries[0].Rule, "real guidance 1") {
			t.Errorf("real0001.md (index 32, beyond cap) should not be in Rule")
		}
		// classifyRuleFiles omits the empty elements, so only real0000.md
		// appears in "Rule Files:" (the empties are skipped from the display).
		files := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule)
		if got, want := files, []string{"real0000.md"}; !reflect.DeepEqual(got, want) {
			t.Errorf("classifyRuleFiles want %v (empties omitted), got %v", want, got)
		}
	})

	// cappedRuleInputsCopies verifies the count cap copies survivors into a
	// fresh slice rather than subslicing the original. A plain subslice
	// aliases the backing array populated by UnmarshalJSON, so the dropped
	// inputs' strings stay alive (Go retains the whole backing array); copying
	// lets the GC reclaim them. We assert the survivors live in an independent
	// backing array from the original ruleInputs.
	t.Run("cappedRuleInputsCopies", func(t *testing.T) {
		// maxRuleInputs+5 distinct inline inputs. inline (not files) so the
		// retained memory is the string data itself — exactly the surface the
		// copy must release.
		n := maxRuleInputs + 5
		inputs := make([]string, n)
		for i := range inputs {
			inputs[i] = fmt.Sprintf("inline-input-%04d", i)
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)
		// Capture the original backing array before the cap copies away.
		original := entries[0].ruleInputs

		_ = captureStderr(t, func() {
			resolveRuleEntries(entries, t.TempDir(), "")
		})

		capped := entries[0].ruleInputs
		if got := len(capped); got != maxRuleInputs {
			t.Fatalf("ruleInputs want %d (capped), got %d", maxRuleInputs, got)
		}
		// The capped slice must have an independent backing array: mutating
		// the original must not affect the capped copy. If they shared the
		// backing array (a plain subslice, the bug this copy fixes), the
		// mutation would leak through. This avoids uintptr(unsafe.Pointer)
		// address comparison, which is outside the sanctioned unsafe patterns
		// and could go stale under a future moving GC.
		if len(original) > 0 && len(capped) > 0 {
			original[0] = ""
			if capped[0] == "" {
				t.Errorf("capped ruleInputs aliases the original backing array; survivors must be copied to a fresh slice so dropped inputs can be GC'd")
			}
		}
		// Survivor content is the first maxRuleInputs inputs, unchanged.
		for i := 0; i < maxRuleInputs; i++ {
			if capped[i] != inputs[i] {
				t.Errorf("survivor[%d]: want %q, got %q", i, inputs[i], capped[i])
			}
		}
	})

	t.Run("exactly at cap is not capped", func(t *testing.T) {
		// Exactly maxRuleInputs real files: no count-cap warning, all retained
		// and resolved.
		dir := t.TempDir()
		n := maxRuleInputs
		inputs := make([]string, n)
		for i := range inputs {
			name := fmt.Sprintf("rule%04d.md", i)
			writeRuleMD(t, dir, name, fmt.Sprintf("guidance %d\n", i))
			inputs[i] = name
		}
		ruleJSON, err := json.Marshal(inputs)
		if err != nil {
			t.Fatalf("marshal inputs: %v", err)
		}
		j := fmt.Sprintf(`[{"path":"**/*.kt","rule":%s}]`, ruleJSON)
		entries := unmarshalEntries(t, j)

		warned := captureStderr(t, func() {
			resolveRuleEntries(entries, dir, "")
		})
		if strings.Contains(warned, "only the first") {
			t.Errorf("no count-cap warning expected at exactly maxRuleInputs, got:\n%s", warned)
		}
		if got := len(entries[0].ruleInputs); got != n {
			t.Errorf("ruleInputs want %d, got %d", n, got)
		}
	})
}

func TestResolveRuleEntries_PartialMissing(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	writeRuleMD(t, dir, "c.md", "Android domain guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","missing.md","c.md"]}]`)

	warned := captureStderr(t, func() {
		resolveRuleEntries(entries, dir, "")
	})

	// missing.md is skipped; a and c remain, joined by a separator (no headings).
	want := "General review guidance.\n\n---\n\nAndroid domain guidance."
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
	// ruleInputs keep the user's original inputs verbatim (resolveRuleEntries
	// preserves them except when the maxRuleInputs cap trims to survivors):
	// missing.md is NOT dropped, so the "Rule Files:" display reports what
	// the user wrote even though missing.md contributed nothing to Rule.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"a.md", "missing.md", "c.md"}) {
		t.Errorf("ruleInputs: want [a.md missing.md c.md], got %v", entries[0].ruleInputs)
	}
	// The missing-file warning carries the owning entry's path, consistent
	// with the empty-element and empty-file warnings in the same function.
	if !strings.Contains(warned, "rule file not found: missing.md (path: **/*.kt)") {
		t.Errorf("missing-file warning lacks entry path, got:\n%s", warned)
	}
}

// TestResolveRuleEntries_EmptyFileSkipped verifies that an existing but
// empty rule file (zero bytes, or only newlines) is skipped in the multi-input
// merge path — parallel to a missing file. Without this, the empty file would
// add an empty segment to the merged parts, producing a blank between
// separators in the merged output. The file still appears in ruleInputs (and
// thus "Rule Files:") because ruleInputs keeps the user's original inputs
// verbatim; it just contributes no text to Rule.
func TestResolveRuleEntries_EmptyFileSkipped(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	writeRuleMD(t, dir, "empty.md", "")       // zero-byte file
	writeRuleMD(t, dir, "nl.md", "\n\n\n")    // only newlines → trims to ""
	writeRuleMD(t, dir, "ws.md", "   \t  \n") // only spaces/tabs → TrimSpace ""
	writeRuleMD(t, dir, "c.md", "Android domain guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","empty.md","nl.md","ws.md","c.md"]}]`)

	warned := captureStderr(t, func() {
		resolveRuleEntries(entries, dir, "")
	})

	// The three empty/whitespace files (empty.md, nl.md, ws.md) are skipped;
	// a and c remain, joined by a separator (no headings) — identical to the
	// partial-missing case.
	want := "General review guidance.\n\n---\n\nAndroid domain guidance."
	if entries[0].Rule != want {
		t.Errorf("Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
	// No per-file heading for the empty files (nor for any file — the body
	// is heading-free; traceability lives in the "Rule Files:" line).
	if strings.Contains(entries[0].Rule, "## Rule: empty.md") {
		t.Errorf("Rule has dangling heading for empty.md: %q", entries[0].Rule)
	}
	if strings.Contains(entries[0].Rule, "## Rule: nl.md") {
		t.Errorf("Rule has dangling heading for nl.md: %q", entries[0].Rule)
	}
	if strings.Contains(entries[0].Rule, "## Rule: ws.md") {
		t.Errorf("Rule has dangling heading for ws.md: %q", entries[0].Rule)
	}
	// ruleInputs keep the user's original inputs verbatim: the
	// empty/whitespace files are NOT dropped, so "Rule Files:" reports what
	// the user wrote even though those files contributed nothing to Rule.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"a.md", "empty.md", "nl.md", "ws.md", "c.md"}) {
		t.Errorf("ruleInputs: want [a.md empty.md nl.md ws.md c.md], got %v", entries[0].ruleInputs)
	}
	// All three empty/whitespace files are warned with the owning entry's
	// path, parallel to the missing-file warning and the empty-element warning.
	for _, name := range []string{"empty.md", "nl.md", "ws.md"} {
		want := "rule file is empty: " + name + " (path: **/*.kt)"
		if !strings.Contains(warned, want) {
			t.Errorf("missing %q warning, got:\n%s", want, warned)
		}
	}
}

func TestResolveRuleEntries_AllMissing(t *testing.T) {
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["x.md","y.md"]}]`)
	resolveRuleEntries(entries, t.TempDir(), "")

	if entries[0].Rule != "" {
		t.Errorf("all-missing array: Rule want empty (entry skipped), got %q", entries[0].Rule)
	}
	// ruleInputs keep the user's original inputs verbatim: even though
	// all files are missing and Rule is empty, the inputs are not cleared,
	// so "Rule Files:" still reports what the user wrote.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"x.md", "y.md"}) {
		t.Errorf("ruleInputs: want [x.md y.md], got %v", entries[0].ruleInputs)
	}
}

// TestResolveRuleEntries_IdempotentOnReResolve verifies that calling
// resolveRuleEntries twice on the same entries leaves the merged Rule
// unchanged. Because resolveRuleEntries keeps the user's original inputs
// verbatim (except when the maxRuleInputs cap trims to survivors), a
// second call re-reads the same inputs and rebuilds the same Rule — the
// function is naturally idempotent, with no resolved flag needed.
func TestResolveRuleEntries_IdempotentOnReResolve(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "General review guidance.\n")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","inline rule text"]}]`)
	resolveRuleEntries(entries, dir, "")

	want := entries[0].Rule
	if want == "" {
		t.Fatal("first resolve produced empty Rule")
	}
	// A second call must reproduce the same Rule. Because resolveRuleEntries
	// keeps the user's original inputs verbatim, a repeat call re-reads
	// the same inputs and rebuilds the same Rule — the function is naturally
	// idempotent, with no resolved flag needed.
	resolveRuleEntries(entries, dir, "")
	if entries[0].Rule != want {
		t.Errorf("re-resolve corrupted Rule:\n want %q\n got  %q", want, entries[0].Rule)
	}
	if strings.Contains(entries[0].Rule, "<inline>") {
		t.Errorf("re-resolve leaked the <inline> sentinel into Rule: %q", entries[0].Rule)
	}
}

// TestResolveRuleEntries_IdempotentWhenReducedToOnePart covers the edge case
// where the multi-input merge path reduces to a single contributing part
// (all other inputs missing). Because resolveRuleEntries keeps ruleInputs
// verbatim, it stays at the user's original inputs ([a.md, missing.md],
// still len >= 2), so a repeat call re-enters the multi-input path and
// rebuilds the same Rule from the same original inputs — naturally
// idempotent, with no resolved flag. The lone file's content here
// ("looks-like-a-path.md") happens to look like a file path; under the old
// resolved-flag design the reduction made ruleInputs len 1 and a re-resolve
// fell through to the single-value path, which would re-read/clear it.
// Keeping ruleInputs verbatim avoids that fall-through entirely because
// ruleInputs is never reduced.
func TestResolveRuleEntries_IdempotentWhenReducedToOnePart(t *testing.T) {
	dir := t.TempDir()
	// a.md's content is a single .md-looking line with no spaces, so
	// looksLikeFilePath treats it as a file path. It is NOT a real file in
	// dir, so if the single-value path ever ran on it, tryReadRuleFile would
	// return nil and clear Rule to "".
	writeRuleMD(t, dir, "a.md", "looks-like-a-path.md")
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","missing.md"]}]`)
	resolveRuleEntries(entries, dir, "")

	want := "looks-like-a-path.md"
	if entries[0].Rule != want {
		t.Fatalf("first resolve: Rule want %q, got %q", want, entries[0].Rule)
	}
	// ruleInputs keeps the original inputs verbatim (missing.md is not
	// dropped), so the entry stays on the multi-input path on re-resolve.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"a.md", "missing.md"}) {
		t.Fatalf("first resolve: ruleInputs want [a.md missing.md], got %v", entries[0].ruleInputs)
	}

	// Re-resolve: ruleInputs is unchanged, so the multi-input path runs
	// again and rebuilds the same Rule — the path-looking content is never
	// fed to the single-value path.
	resolveRuleEntries(entries, dir, "")
	if entries[0].Rule != want {
		t.Errorf("re-resolve corrupted Rule: want %q, got %q", want, entries[0].Rule)
	}
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"a.md", "missing.md"}) {
		t.Errorf("re-resolve corrupted ruleInputs: want [a.md missing.md], got %v", entries[0].ruleInputs)
	}
}

// TestProjectRuleEntry_UnmarshalJSON_ReDecodeReResolvesNewArray verifies that
// re-decoding an already-resolved entry (in place) as a different multi-element
// array discards the prior resolution and re-resolves the new inputs.
// encoding/json does not zero the receiver before UnmarshalJSON, so
// UnmarshalJSON must clear the stale Rule/ruleInputs itself; otherwise the
// old merged Rule would survive the re-decode and the new inputs would never
// be read. (The earlier resolved-flag variant of this test is obsolete now
// that there is no resolved flag — the re-decode simply repopulates
// ruleInputs with the new array and resolveRuleEntries rebuilds Rule from it.)
func TestProjectRuleEntry_UnmarshalJSON_ReDecodeReResolvesNewArray(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "a.md", "first file body\n")
	writeRuleMD(t, dir, "b.md", "second file body\n")
	writeRuleMD(t, dir, "c.md", "third file body\n")
	writeRuleMD(t, dir, "d.md", "fourth file body\n")

	// 1. Decode + resolve a multi-element array entry.
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["a.md","b.md"]}]`)
	resolveRuleEntries(entries, dir, "")
	first := entries[0].Rule
	if first == "" {
		t.Fatal("first resolve produced empty Rule")
	}
	if !strings.Contains(first, "first file body") {
		t.Fatalf("first resolve missing a.md content: %q", first)
	}

	// 2. Re-decode the SAME entry (in place) as a different multi-element
	//    array. UnmarshalJSON must clear the stale Rule/ruleInputs so the
	//    prior resolution is discarded.
	if err := json.Unmarshal(
		[]byte(`[{"path":"**/*.kt","rule":["c.md","d.md"]}]`), &entries); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if entries[0].Rule != "" {
		t.Errorf("re-decode should clear stale Rule, got %q", entries[0].Rule)
	}

	// 3. Re-resolve: must produce the NEW merge (c.md + d.md), not reuse the
	//    stale Rule left from a.md/b.md.
	resolveRuleEntries(entries, dir, "")
	got := entries[0].Rule
	if got == "" {
		t.Fatal("re-resolve after re-decode produced empty Rule")
	}
	if strings.Contains(got, "first file body") || strings.Contains(got, "second file body") {
		t.Errorf("re-resolve used stale a.md/b.md content: %q", got)
	}
	if !strings.Contains(got, "third file body") || !strings.Contains(got, "fourth file body") {
		t.Errorf("re-resolve missing c.md/d.md content: %q", got)
	}
}

// TestResolveRuleEntries_SingleMissingKeepsRuleInputs verifies the single-value
// path keeps ruleInputs at the user's original input when its file is missing.
// Rule is cleared (legacy: the entry is skipped and falls through), but
// ruleInputs is not cleared, so "Rule Files:" still reports the missing file
// the user wrote — consistent with the multi-file path.
func TestResolveRuleEntries_SingleMissingKeepsRuleInputs(t *testing.T) {
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":"missing.md"}]`)
	resolveRuleEntries(entries, t.TempDir(), "")

	if entries[0].Rule != "" {
		t.Errorf("single-missing: Rule want empty, got %q", entries[0].Rule)
	}
	// ruleInputs keeps the original input: the missing file is reported
	// in "Rule Files:" even though it contributed nothing to Rule.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"missing.md"}) {
		t.Errorf("single-missing: ruleInputs want [missing.md], got %v", entries[0].ruleInputs)
	}
	if got := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule); !reflect.DeepEqual(got, []string{"missing.md"}) {
		t.Errorf("single-missing: classifyRuleFiles want [missing.md], got %v", got)
	}
}

// TestResolveRuleEntries_SingleEmptyFileKeepsRuleInputs is the empty/whitespace
// counterpart of TestResolveRuleEntries_SingleMissingKeepsRuleInputs. A rule
// file that exists but is empty (or whitespace-only) contributes no rule text;
// ruleInputs still keeps the user's original input, so "Rule Files:"
// reports the file even though it contributed nothing. Rule resolution stays
// byte-identical to legacy: an empty file resolves to "" and a whitespace-only
// file still wins its layer (e.Rule keeps the content verbatim).
func TestResolveRuleEntries_SingleEmptyFileKeepsRuleInputs(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "empty.md", "")       // zero-byte
	writeRuleMD(t, dir, "ws.md", "   \t  \n") // whitespace-only → TrimSpace ""
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":"empty.md"},{"path":"**/*.java","rule":"ws.md"}]`)
	resolveRuleEntries(entries, dir, "")

	// empty.md: Rule="" (legacy — entry skipped unless MergeSystemRule).
	if entries[0].Rule != "" {
		t.Errorf("empty file: Rule want %q, got %q", "", entries[0].Rule)
	}
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"empty.md"}) {
		t.Errorf("empty file: ruleInputs want [empty.md], got %v", entries[0].ruleInputs)
	}
	if got := classifyRuleFiles(entries[0].ruleInputs, entries[0].Rule); !reflect.DeepEqual(got, []string{"empty.md"}) {
		t.Errorf("empty file: classifyRuleFiles want [empty.md], got %v", got)
	}

	// ws.md: Rule keeps the whitespace content (legacy — entry still wins).
	if strings.TrimSpace(entries[1].Rule) != "" {
		t.Errorf("ws file: Rule want whitespace-only, got %q", entries[1].Rule)
	}
	if !reflect.DeepEqual(entries[1].ruleInputs, []string{"ws.md"}) {
		t.Errorf("ws file: ruleInputs want [ws.md], got %v", entries[1].ruleInputs)
	}
	// ruleInputs keeps the original input: classifyRuleFiles reports the
	// file the user wrote, even though it contributed only whitespace.
	if got := classifyRuleFiles(entries[1].ruleInputs, entries[1].Rule); !reflect.DeepEqual(got, []string{"ws.md"}) {
		t.Errorf("ws file: classifyRuleFiles want [ws.md], got %v", got)
	}
}

func TestResolveRuleEntries_MultiFileConfinedToRepo(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "inside.md", "inside\n")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("OUTSIDE SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Array referencing an in-repo file plus an absolute path outside the repo.
	// json.Marshal the absolute path so backslashes (Windows t.TempDir paths)
	// are properly escaped into a valid JSON string, not raw-embedded.
	outsideJSON, err := json.Marshal(outside)
	if err != nil {
		t.Fatal(err)
	}
	entries := unmarshalEntries(t, `[{"path":"**/*.kt","rule":["inside.md",`+string(outsideJSON)+`]}]`)
	resolveRuleEntries(entries, repoDir, canonicalRoot(t, repoDir))

	// Only the in-repo file survives; the outside path is rejected by #1100.
	if entries[0].Rule != "inside" {
		t.Errorf("Rule: want %q, got %q", "inside", entries[0].Rule)
	}
	if strings.Contains(entries[0].Rule, "OUTSIDE SECRET") {
		t.Errorf("outside secret leaked into rule: %q", entries[0].Rule)
	}
	// ruleInputs keeps the user's original input: the rejected outside
	// path is still reported in "Rule Files:" even though #1100 dropped it
	// from Rule. The secret never leaks because the file was never read.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"inside.md", outside}) {
		t.Errorf("ruleInputs: want [inside.md <outside>], got %v", entries[0].ruleInputs)
	}
}

func TestResolveRuleEntries_GoLiteralStillWorks(t *testing.T) {
	dir := t.TempDir()
	writeRuleMD(t, dir, "foo.md", "General review guidance.\n")
	// Go-literal-constructed entry: ruleInputs is nil, takes the legacy path.
	entries := []ProjectRuleEntry{{Path: "**/*.kt", Rule: "foo.md"}}
	resolveRuleEntries(entries, dir, "")

	if entries[0].Rule != "General review guidance." {
		t.Errorf("Rule: want %q, got %q", "General review guidance.", entries[0].Rule)
	}
	// ruleInputs is back-filled so RuleFiles can still trace the file.
	if !reflect.DeepEqual(entries[0].ruleInputs, []string{"foo.md"}) {
		t.Errorf("ruleInputs: want [foo.md], got %v", entries[0].ruleInputs)
	}
}

// --- Resolve: merge_system_rule + first-match-wins ---

func TestResolve_MergeSystemRuleWithArray(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "a.md", "User rule A.\n")
	writeRuleMD(t, repoDir, "b.md", "User rule B.\n")
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.go","merge_system_rule":true,"rule":["a.md","b.md"]}]}`)
	r := newResolverAt(t, repoDir)

	got := r.Resolve("src/foo.go")
	if !strings.Contains(got, "## System-Specific Rules") {
		t.Errorf("merged rule missing system section: %q", truncate(got, 120))
	}
	if !strings.Contains(got, "User rule A.") || !strings.Contains(got, "User rule B.") {
		t.Errorf("merged rule missing user content: %q", truncate(got, 200))
	}
}

func TestResolve_FirstMatchWinsUnchangedWithArray(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "a.md", "General review guidance.\n")
	writeRuleMD(t, repoDir, "b.md", "Kotlin-specific guidance.\n")
	writeRuleMD(t, repoDir, "other.md", "OTHER\n")
	// First entry is an array that matches; second entry also matches but must
	// not participate (first-match-wins is unchanged).
	writeProjectRule(t, repoDir, `{"rules":[
		{"path":"**/*.go","rule":["a.md","b.md"]},
		{"path":"**/*.go","rule":"other.md"}
	]}`)
	r := newResolverAt(t, repoDir)

	got := r.Resolve("src/foo.go")
	if !strings.Contains(got, "General review guidance.") || !strings.Contains(got, "Kotlin-specific guidance.") {
		t.Errorf("expected merged a.md+b.md, got %q", truncate(got, 120))
	}
	if strings.Contains(got, "OTHER") {
		t.Errorf("second entry must not win under first-match-wins, got %q", truncate(got, 120))
	}
}

// TestResolve_CustomArrayOverridesProject is the multi-md dual of
// TestNewResolver_CustomOverridesProject: a custom-layer array entry must win
// over a project-layer single-value entry for the same path, and Resolve must
// return the merged multi-file body (not the project single value). This
// guards that an array entry is a first-class participant in inter-layer
// priority and that the winner's merged body is what reaches the caller.
func TestResolve_CustomArrayOverridesProject(t *testing.T) {
	repoDir := t.TempDir()
	// Project layer: single-value rule that would win if custom were absent.
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.java","rule":"project-java-rule"}]}`)

	// Custom layer (--rule): array of two files, same path pattern.
	customDir := t.TempDir()
	writeRuleMD(t, customDir, "cust-a.md", "Custom rule A.\n")
	writeRuleMD(t, customDir, "cust-b.md", "Custom rule B.\n")
	customPath := filepath.Join(customDir, "custom_rules.json")
	customJSON := `{"rules":[{"path":"**/*.java","rule":["cust-a.md","cust-b.md"]}]}`
	if err := os.WriteFile(customPath, []byte(customJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	setTestHome(t, t.TempDir()) // isolate global layer
	r, _, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	got := r.Resolve("src/Foo.java")
	if !strings.Contains(got, "Custom rule A.") || !strings.Contains(got, "Custom rule B.") {
		t.Errorf("expected merged cust-a.md+cust-b.md, got %q", truncate(got, 160))
	}
	if !strings.Contains(got, "\n\n---\n\n") {
		t.Errorf("expected segment separator in merged body, got %q", truncate(got, 160))
	}
	if strings.Contains(got, "project-java-rule") {
		t.Errorf("custom array must override project, got project rule: %q", truncate(got, 160))
	}
}

// TestResolve_ProjectArrayOverridesSystem is the multi-md dual of
// TestNewResolver_ProjectRuleReplacesSystemRuleByDefault: a project-layer
// array entry (without merge_system_rule) must replace the system rule, and
// Resolve must return the merged multi-file body rather than the system rule.
// Guards that an array entry wins the layer and its merged body replaces
// (does not merge with) the system default.
func TestResolve_ProjectArrayOverridesSystem(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "proj-a.md", "Project rule A.\n")
	writeRuleMD(t, repoDir, "proj-b.md", "Project rule B.\n")
	// No merge_system_rule: a winning project entry replaces the system rule.
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.go","rule":["proj-a.md","proj-b.md"]}]}`)
	r := newResolverAt(t, repoDir)

	got := r.Resolve("main.go")
	if !strings.Contains(got, "Project rule A.") || !strings.Contains(got, "Project rule B.") {
		t.Errorf("expected merged proj-a.md+proj-b.md, got %q", truncate(got, 160))
	}
	if strings.Contains(got, "## System-Specific Rules") {
		t.Errorf("project array without merge must replace system, got system section: %q", truncate(got, 160))
	}
}

// TestResolve_ArrayAllMissingWithMergeReturnsSystemOnly is the multi-md dual
// of TestNewResolver_EmptyRuleMergeSystemRuleReturnsSystemOnly: when every
// array element fails to resolve (all missing), the merged Rule is empty, but
// because merge_system_rule is set the entry is NOT skipped (the skip guard
// is `Rule == "" && !MergeSystemRule`) — Resolve must return the system rule
// only, with no User-Specific Rules section. This counter-intuitive boundary
// (merge bypasses the empty-rule skip) is the point most easily broken by a
// naive "drop empty array entries" refactor, so it is pinned explicitly.
func TestResolve_ArrayAllMissingWithMergeReturnsSystemOnly(t *testing.T) {
	repoDir := t.TempDir()
	// Both referenced files are intentionally absent; merge_system_rule is set
	// so the empty merged Rule still triggers mergeWithSystemRule.
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.go","merge_system_rule":true,"rule":["missing-a.md","missing-b.md"]}]}`)
	r := newResolverAt(t, repoDir)

	systemRule, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	want := systemRule.Resolve("main.go")

	got := r.Resolve("main.go")
	if got != want {
		t.Fatalf("all-missing array + merge: want system rule only, got %q", truncate(got, 160))
	}
	if strings.Contains(got, "User-Specific Rules") {
		t.Fatalf("all-missing array + merge: must not contain User-Specific Rules, got %q", truncate(got, 160))
	}
}

// TestResolveDetail_CustomArrayOverridesAll is the multi-md dual of
// TestResolveDetail_CustomOverridesAll: a custom-layer array entry must win
// via the ResolveDetail path, report Source "custom", carry the merged
// multi-file body in Rule, and list both files in RuleFiles. Guards that the
// detail-resolution chain (matchProjectRuleDetail -> matchProjectRuleEntry,
// unchanged by the feature) treats an array entry identically to a
// single-value entry at the selection layer, and that RuleFiles traceability
// survives an inter-layer win.
func TestResolveDetail_CustomArrayOverridesAll(t *testing.T) {
	repoDir := t.TempDir()
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.java","rule":"project-java-rule"}]}`)

	customDir := t.TempDir()
	writeRuleMD(t, customDir, "cust-a.md", "Custom rule A.\n")
	writeRuleMD(t, customDir, "cust-b.md", "Custom rule B.\n")
	customPath := filepath.Join(customDir, "custom_rules.json")
	customJSON := `{"rules":[{"path":"**/*.java","rule":["cust-a.md","cust-b.md"]}]}`
	if err := os.WriteFile(customPath, []byte(customJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	setTestHome(t, t.TempDir())
	r, _, err := NewResolver(repoDir, customPath, ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr := r.(DetailResolver)

	d := dr.ResolveDetail("src/foo.java")
	if d.Source != "custom" {
		t.Errorf("expected source 'custom', got %q", d.Source)
	}
	if !strings.Contains(d.Rule, "Custom rule A.") || !strings.Contains(d.Rule, "Custom rule B.") {
		t.Errorf("expected merged custom body, got %q", truncate(d.Rule, 160))
	}
	if !reflect.DeepEqual(d.RuleFiles, []string{"cust-a.md", "cust-b.md"}) {
		t.Errorf("RuleFiles: want [cust-a.md cust-b.md], got %v", d.RuleFiles)
	}
	if strings.Contains(d.Rule, "project-java-rule") {
		t.Errorf("custom array must override project in detail path, got project rule: %q", truncate(d.Rule, 160))
	}
}

func TestCanonicalConfig_StableForMerged(t *testing.T) {
	build := func(t *testing.T, repoDir string) []string {
		r := newResolverAt(t, repoDir).(*composedResolver)
		return r.CanonicalConfig()
	}

	withFiles := func(t *testing.T) string {
		repoDir := t.TempDir()
		writeRuleMD(t, repoDir, "a.md", "General review guidance.\n")
		writeRuleMD(t, repoDir, "b.md", "Kotlin-specific guidance.\n")
		writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.go","rule":["a.md","b.md"]}]}`)
		return repoDir
	}

	// Same merged content => identical canonical config.
	c1 := build(t, withFiles(t))
	c2 := build(t, withFiles(t))
	if !reflect.DeepEqual(c1, c2) {
		t.Errorf("same merged config must be stable:\n c1=%v\n c2=%v", c1, c2)
	}

	// Different merged content => different canonical config.
	repoDir2 := t.TempDir()
	writeRuleMD(t, repoDir2, "a.md", "DIFFERENT\n")
	writeRuleMD(t, repoDir2, "b.md", "Kotlin-specific guidance.\n")
	writeProjectRule(t, repoDir2, `{"rules":[{"path":"**/*.go","rule":["a.md","b.md"]}]}`)
	c3 := build(t, repoDir2)
	if reflect.DeepEqual(c1, c3) {
		t.Errorf("different merged content must yield different config:\n c1=%v\n c3=%v", c1, c3)
	}
}

// --- ResolveDetail.RuleFiles ---

func TestResolveDetail_RuleFilesPopulated_Array(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "a.md", "General review guidance.\n")
	writeRuleMD(t, repoDir, "b.md", "Kotlin-specific guidance.\n")
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":["a.md","b.md"]}]}`)
	dr := newResolverAt(t, repoDir).(DetailResolver)

	d := dr.ResolveDetail("src/Foo.kt")
	if !reflect.DeepEqual(d.RuleFiles, []string{"a.md", "b.md"}) {
		t.Errorf("RuleFiles: want [a.md b.md], got %v", d.RuleFiles)
	}
}

func TestResolveDetail_RuleFilesPopulated_SingleFile(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "kotlin.md", "Kotlin review guidance.\n")
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":"kotlin.md"}]}`)
	dr := newResolverAt(t, repoDir).(DetailResolver)

	d := dr.ResolveDetail("src/Foo.kt")
	if !reflect.DeepEqual(d.RuleFiles, []string{"kotlin.md"}) {
		t.Errorf("RuleFiles: want [kotlin.md], got %v", d.RuleFiles)
	}
}

func TestResolveDetail_RuleFilesPopulated_Inline(t *testing.T) {
	repoDir := t.TempDir()
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":"watch for null safety"}]}`)
	dr := newResolverAt(t, repoDir).(DetailResolver)

	d := dr.ResolveDetail("src/Foo.kt")
	if !reflect.DeepEqual(d.RuleFiles, []string{"<inline>"}) {
		t.Errorf("RuleFiles: want [<inline>], got %v", d.RuleFiles)
	}
}

// TestResolveDetail_RuleFiles_SingleEmptyFileShownAsWritten verifies the
// scenario from the review: a single-value rule pointing at an empty (or
// whitespace-only) file lists that file in "Rule Files:" as the user wrote it
// (ruleInputs keeps the original input). The file contributed nothing to
// Rule, but it is still the source the user named, so it is reported honestly.
// When merge_system_rule keeps the empty entry as the winner, the file is
// shown; when a whitespace-only file wins under legacy resolution, it is shown
// too. The Rule body itself stays byte-identical to legacy.
func TestResolveDetail_RuleFiles_SingleEmptyFileShownAsWritten(t *testing.T) {
	t.Run("empty file with merge_system_rule", func(t *testing.T) {
		repoDir := t.TempDir()
		writeRuleMD(t, repoDir, "empty.md", "")
		writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":"empty.md","merge_system_rule":true}]}`)
		dr := newResolverAt(t, repoDir).(DetailResolver)

		d := dr.ResolveDetail("src/Foo.kt")
		if d.Source != "project" {
			t.Fatalf("expected the project entry to win (merge_system_rule), got Source=%q", d.Source)
		}
		if !reflect.DeepEqual(d.RuleFiles, []string{"empty.md"}) {
			t.Errorf("empty file: RuleFiles want [empty.md], got %v", d.RuleFiles)
		}
	})
	t.Run("whitespace-only file shown as written", func(t *testing.T) {
		repoDir := t.TempDir()
		writeRuleMD(t, repoDir, "ws.md", "   \t  \n")
		writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":"ws.md"}]}`)
		dr := newResolverAt(t, repoDir).(DetailResolver)

		d := dr.ResolveDetail("src/Foo.kt")
		if d.Source != "project" {
			t.Fatalf("expected the whitespace-file entry to win (legacy), got Source=%q", d.Source)
		}
		// The whitespace-only file contributed no real content to Rule, but
		// ruleInputs keeps the user's input, so "Rule Files:" reports the
		// file by name — neither nil (hiding the source) nor "<inline>"
		// (mislabeling a file as inline).
		if !reflect.DeepEqual(d.RuleFiles, []string{"ws.md"}) {
			t.Errorf("whitespace file: RuleFiles want [ws.md], got %v", d.RuleFiles)
		}
	})
}

// TestClassifyRuleFiles_PreservesFullInput verifies that the "Rule Files:"
// display label keeps each input verbatim — the full path as the user wrote
// it in the rule array (e.g. "rules/kotlin.md"), not a basename, so same-named
// files in different directories stay distinguishable. This is local
// ocr-rules-check output only (RuleFiles has json:"-" and never reaches the
// LLM/delegate); path-traversal safety is already enforced by #1100's
// confineRoot on the untrusted project layer, so no extra sanitization is
// applied and the input is preserved as-is. Inline inputs become "<inline>".
func TestClassifyRuleFiles_PreservesFullInput(t *testing.T) {
	// Relative file path kept verbatim (full path, not basename).
	got := classifyRuleFiles([]string{"rules/kotlin.md"}, "x")
	if !reflect.DeepEqual(got, []string{"rules/kotlin.md"}) {
		t.Errorf("relative input: want [rules/kotlin.md], got %v", got)
	}
	// A bare basename is kept verbatim too.
	got = classifyRuleFiles([]string{"a.md"}, "x")
	if !reflect.DeepEqual(got, []string{"a.md"}) {
		t.Errorf("basename input: want [a.md], got %v", got)
	}
	// Mixed file + inline inputs labeled per-element.
	got = classifyRuleFiles([]string{"rules/kotlin.md", "watch for null safety"}, "x")
	if !reflect.DeepEqual(got, []string{"rules/kotlin.md", "<inline>"}) {
		t.Errorf("mixed input: want [rules/kotlin.md <inline>], got %v", got)
	}
	// An empty/whitespace array element is skipped from the Rule body by
	// resolveMultiRuleParts (warned, contributes nothing), so it is omitted
	// from the "Rule Files:" display entirely — it is not a file and carries
	// no inline text, so a label would be misleading.
	got = classifyRuleFiles([]string{"a.md", "", "b.md"}, "x")
	if !reflect.DeepEqual(got, []string{"a.md", "b.md"}) {
		t.Errorf("empty element omitted: want [a.md b.md], got %v", got)
	}
	got = classifyRuleFiles([]string{"a.md", "  ", "b.md"}, "x")
	if !reflect.DeepEqual(got, []string{"a.md", "b.md"}) {
		t.Errorf("whitespace element omitted: want [a.md b.md], got %v", got)
	}
	// Fallback (Go-literal, ruleInputs nil) infers from the resolved Rule.
	got = classifyRuleFiles(nil, "rules/kotlin.md")
	if !reflect.DeepEqual(got, []string{"rules/kotlin.md"}) {
		t.Errorf("relative fallback: want [rules/kotlin.md], got %v", got)
	}
	got = classifyRuleFiles(nil, "inline rule text")
	if !reflect.DeepEqual(got, []string{"<inline>"}) {
		t.Errorf("inline fallback: want [<inline>], got %v", got)
	}
	// Fallback with an empty resolved Rule yields nil (no file, no text).
	got = classifyRuleFiles(nil, "")
	if got != nil {
		t.Errorf("empty fallback: want nil, got %v", got)
	}
	got = classifyRuleFiles(nil, "  ")
	if got != nil {
		t.Errorf("whitespace fallback: want nil, got %v", got)
	}
}

// TestClassifyRuleFiles_AllEmptyReturnsNil verifies that when every element of
// a multi-element array is an empty/whitespace element (all omitted from the
// "Rule Files:" display), the inputs branch returns nil — the same "no rule
// files" representation the fallback path uses for an empty/whitespace Rule —
// so the two paths never diverge (a caller comparing != nil sees the same
// result either way). The production caller uses len()>0, which already
// handles both nil and a non-nil empty slice identically, but aligning on nil
// keeps the convention consistent for any future != nil check.
func TestClassifyRuleFiles_AllEmptyReturnsNil(t *testing.T) {
	if got := classifyRuleFiles([]string{"", ""}, "x"); got != nil {
		t.Errorf("all empty: want nil, got %v", got)
	}
	if got := classifyRuleFiles([]string{"  ", "\t"}, "x"); got != nil {
		t.Errorf("all whitespace: want nil, got %v", got)
	}
	if got := classifyRuleFiles([]string{"", "  ", ""}, ""); got != nil {
		t.Errorf("mixed empty/whitespace: want nil, got %v", got)
	}
	// A single (non-multi) whitespace input is the whole rule and wins its
	// layer (legacy), so it stays "<inline>", NOT nil — only multi-element
	// arrays omit empty elements. This guards against the fix over-reaching.
	if got := classifyRuleFiles([]string{"  "}, "x"); !reflect.DeepEqual(got, []string{"<inline>"}) {
		t.Errorf("single whitespace input: want [<inline>], got %v", got)
	}
}

func TestResolveDetail_RuleFiles_GoLiteral(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "a.md", "General review guidance.\n")
	// Construct a project rule by Go literal (no UnmarshalJSON => ruleInputs nil),
	// resolve, then match via a composedResolver.
	pr := &ProjectRule{Rules: []ProjectRuleEntry{{Path: "**/*.kt", Rule: "a.md"}}}
	resolveRuleEntries(pr.Rules, repoDir, "")
	c := &composedResolver{project: pr}

	d := c.matchProjectRuleDetail(pr, "src/Foo.kt", "project")
	if d == nil {
		t.Fatal("expected a match")
	}
	if !reflect.DeepEqual(d.RuleFiles, []string{"a.md"}) {
		t.Errorf("RuleFiles (Go literal): want [a.md], got %v", d.RuleFiles)
	}
}

// TestResolveDetail_RuleFiles_MixedInlineAndFileLabels verifies that a
// multi-element array mixing file and inline inputs labels each correctly:
// files keep their path, inline parts become "<inline>". ruleInputs keeps the
// user's original inputs verbatim, and classifyRuleFiles labels each
// element from those original inputs.
func TestResolveDetail_RuleFiles_MixedInlineAndFileLabels(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "a.md", "General review guidance.\n")
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":["a.md","watch for null safety"]}]}`)
	dr := newResolverAt(t, repoDir).(DetailResolver)

	d := dr.ResolveDetail("src/Foo.kt")
	if !reflect.DeepEqual(d.RuleFiles, []string{"a.md", "<inline>"}) {
		t.Errorf("RuleFiles: want [a.md <inline>], got %v", d.RuleFiles)
	}
}

// TestResolveDetail_RuleFiles_EmptyElementOmitted verifies that an empty
// (or whitespace-only) array element — e.g. a trailing comma producing "" —
// is skipped from the Rule body by resolveMultiRuleParts (warned, contributes
// nothing) and omitted from "Rule Files:" by classifyRuleFiles. The empty
// element is not a file and carries no inline text, so a label would be
// misleading; the display reports only the inputs that are files or inline
// text. The merged Rule body omits the empty element (no dangling separator).
func TestResolveDetail_RuleFiles_EmptyElementOmitted(t *testing.T) {
	repoDir := t.TempDir()
	writeRuleMD(t, repoDir, "a.md", "General review guidance.\n")
	writeRuleMD(t, repoDir, "b.md", "Kotlin-specific guidance.\n")
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":["a.md","","b.md"]}]}`)
	dr := newResolverAt(t, repoDir).(DetailResolver)

	d := dr.ResolveDetail("src/Foo.kt")
	// The empty element is omitted from RuleFiles (it is neither a file nor
	// inline text); only a.md and b.md are reported.
	if !reflect.DeepEqual(d.RuleFiles, []string{"a.md", "b.md"}) {
		t.Errorf("RuleFiles: want [a.md b.md] (empty omitted), got %v", d.RuleFiles)
	}
	// The empty element contributes nothing to the Rule body: a.md and b.md
	// merge with a single separator, no dangling heading for the empty slot.
	want := "General review guidance.\n\n---\n\nKotlin-specific guidance."
	if d.Rule != want {
		t.Errorf("Rule body:\n want %q\n got  %q", want, d.Rule)
	}
}

// TestResolveDetail_RuleFiles_WhitespaceIsInline verifies that a
// whitespace-only single-value rule (legacy: it wins first-match-wins and its
// Rule is left byte-identical by resolveRuleEntries) is labeled "<inline>":
// an inline rule is inline regardless of its content, so whitespace content
// is still an inline (non-file) source. Rule stays "  " (byte-identical
// legacy); RuleFiles reports [<inline>].
func TestResolveDetail_RuleFiles_WhitespaceIsInline(t *testing.T) {
	repoDir := t.TempDir()
	writeProjectRule(t, repoDir, `{"rules":[{"path":"**/*.kt","rule":"  "}]}`)
	dr := newResolverAt(t, repoDir).(DetailResolver)

	d := dr.ResolveDetail("src/Foo.kt")
	if d.Rule != "  " {
		t.Errorf("Rule: want \"  \" (byte-identical legacy, whitespace wins first-match-wins), got %q", d.Rule)
	}
	if !reflect.DeepEqual(d.RuleFiles, []string{"<inline>"}) {
		t.Errorf("RuleFiles: want [<inline>] for whitespace inline rule, got %v", d.RuleFiles)
	}
}

// TestResolveDetail_RuleFiles_WhitespaceGoLiteralIsNil verifies the
// ruleInputs-nil (Go-literal) fallback path: a whitespace-only Rule yields
// nil, not ["<inline>"]. classifyRuleFiles cannot tell a Go-literal inline
// whitespace rule from a file that resolved to whitespace (both have
// ruleInputs nil and a whitespace Rule); since whitespace contributes no
// meaningful source either way, the fallback reports nothing. (A JSON
// inline whitespace rule like `rule: "  "` still goes through the inputs
// branch with ruleInputs=["  "] and is labeled "<inline>" — see
// TestResolveDetail_RuleFiles_WhitespaceIsInline.) Rule stays "  "
// (byte-identical legacy: the entry still wins first-match-wins).
func TestResolveDetail_RuleFiles_WhitespaceGoLiteralIsNil(t *testing.T) {
	pr := &ProjectRule{Rules: []ProjectRuleEntry{{Path: "**/*.kt", Rule: "  "}}}
	resolveRuleEntries(pr.Rules, t.TempDir(), "")
	c := &composedResolver{project: pr}

	d := c.matchProjectRuleDetail(pr, "src/Foo.kt", "project")
	if d == nil {
		t.Fatal("expected a match")
	}
	if d.Rule != "  " {
		t.Errorf("Rule: want \"  \" (byte-identical legacy), got %q", d.Rule)
	}
	if d.RuleFiles != nil {
		t.Errorf("RuleFiles: want nil for whitespace Go-literal rule, got %v", d.RuleFiles)
	}
}
