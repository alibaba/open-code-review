# Phoenix Security Scan Integration — Implementation Plan (ocr side)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ocr` the orchestrator of a PR's security review — ingest external SAST/SCA findings, split them by confidence, adjudicate the uncertain ones in a dedicated LLM triage stage, and emit one merged output with one verdict.

**Architecture:** A vendor-neutral `internal/findings` domain (SARIF ingestion, diff scoping, confidence policy, dedup) plus a new `internal/triage` stage that drives the existing shared `llmloop.Runner` — the same runner `review` and `scan` already use. Phoenix-specific behaviour lives behind a `findings.Provider` interface in `internal/findings/providers/phoenix`, so nothing in the core couples to Phoenix. The plugin and skill trees gain a `ocr-review-security-phx` variant.

**Tech Stack:** Go 1.x (standard library `testing`, no assertion libraries), cobra CLI, SARIF 2.1.0, `net/http/httptest` for provider tests, Markdown skills, JSON plugin manifests.

**Spec:** [`docs/superpowers/specs/2026-08-24-phoenix-security-scan-integration-design.md`](../specs/2026-08-24-phoenix-security-scan-integration-design.md)

**Scope note:** This plan covers the `open-code-review-phx` repo only. The Phoenix-side Kotlin work (the `/api/v1/external/sca/pr-delta` endpoint, manifest-delta resolution, per-CVE reachability, and the `sca_pr_delta` / `sca_finding_context` MCP tools — spec §5) is a separate plan in `agent-code-analyzer-r2`. Tasks 14 and 15 here build against a recorded contract fixture, so this plan never blocks on that one.

## Global Constraints

- **Module path:** `github.com/alibaba/open-code-review`. All internal imports use this prefix.
- **License header:** every new `.go` file starts with exactly these two lines, then a blank line:
  ```go
  // SPDX-License-Identifier: Apache-2.0
  // Copyright 2026 alibaba/open-code-review Contributors
  ```
  Enforced by `make license-check` (`scripts/verify-license.sh`).
- **English only** in all source, comments, and commit messages. Enforced by `make english-check`.
- **Tests:** standard-library `testing` only. No testify, no gomock. Follow the existing style: `t.Fatalf`/`t.Errorf` with a message naming the operation.
- **Coverage:** 90% threshold, enforced by `make coverage`. Every task must leave it green.
- **Verification commands:** `make check` (mod tidy + gofmt + vet + license + english), `make test` (`go test -v -race -count=1`), `make coverage`.
- **Line endings:** LF. Run `git add --renormalize .` if in doubt.
- **Three-state invariant:** `Reachability`, `KEV`, `Malware`, and `ExploitEvidence` are never booleans. Absence is `unknown`, and `unknown` never resolves as the negative. This is the single most important correctness rule in this plan.
- **Schema version:** any change to `internal/session` record shapes requires bumping `ocr.run-manifest/v1` (CLAUDE.md §10). Task 9 owns this.
- **Commit format:** `<type>(<scope>): <subject>` in English, matching recent history (`fix(scan):`, `feat(cli):`).

---

## File Structure

**New — vendor-neutral core:**

| File | Responsibility |
|---|---|
| `internal/findings/model.go` | `ExternalFinding` DTO, the three-state enums, `Normalize`, `ComputeFingerprint`. |
| `internal/findings/sarif.go` | SARIF 2.1.0 → `[]ExternalFinding` ingestion. |
| `internal/findings/scope.go` | `ChangedLines` — maps findings onto lines the diff actually touched. |
| `internal/findings/policy.go` | The confidence split: `pass-through` / `triage` / `drop`. |
| `internal/findings/provider.go` | `Provider` interface, `ScanRequest`, `Result`. |
| `internal/findings/convert.go` | `ExternalFinding` → `model.LlmComment`. |
| `internal/findings/dedup.go` | Fingerprint dedup + line-proximity merge against LLM comments. |
| `internal/findings/manifest.go` | Dependency-manifest detection for SCA scoping. |
| `internal/findings/providers/sarif/provider.go` | File-backed provider for `--findings <file>`. |
| `internal/findings/providers/phoenix/client.go` | HTTP plumbing, auth, polling. |
| `internal/findings/providers/phoenix/sast.go` | `pr-scan` resolve → execute → poll → SARIF. |
| `internal/findings/providers/phoenix/sca.go` | Manifest delta → `/sca/pr-delta` → CVEs + reachability. |
| `internal/findings/providers/phoenix/provider.go` | Assembles SAST + SCA into one `Result`. |
| `internal/triage/cluster.go` | Groups findings into units of work. |
| `internal/triage/verdict.go` | `Verdict` type and the `finding_verdict` tool provider. |
| `internal/triage/triage.go` | The stage: builds messages (prompt assembly included), drives `llmloop.Runner`, reconciles verdicts. |

**New — prompts and config:**

| File | Responsibility |
|---|---|
| `internal/config/template/prompts/triage_task_system.md` | Triage system prompt. |
| `internal/config/template/prompts/triage_task_user.md` | Triage user prompt with placeholders. |

**Modified:**

| File | Change |
|---|---|
| `internal/model/review.go` | `LlmComment` gains provenance fields. |
| `internal/config/toolsconfig/tools.json` | Adds the `finding_verdict` tool. |
| `internal/config/template/template.go` | Loads the triage templates. |
| `internal/session/persist.go`, `manifest.go` | New record types, `schema_version` bump. |
| `internal/agent/identity.go` | Security profile folded into the resume identity hash. |
| `cmd/opencodereview/review_cmd.go` | Prefetch → policy → triage → merge wiring. |
| `cmd/opencodereview/shared_flags.go` | `--findings`, `--security`, `--no-triage`. |
| `cmd/opencodereview/sarif.go` | Emits provenance and rule id. |
| `cmd/opencodereview/root.go` | Registers `ocr triage`. |

**New — CLI:**

| File | Responsibility |
|---|---|
| `cmd/opencodereview/security.go` | `securityOptions` and `runSecurityPipeline` — prefetch, policy, triage, merge. |
| `cmd/opencodereview/gate.go` | `ComputeGate` — the run's PASS/WARN/BLOCK verdict. |
| `cmd/opencodereview/security_config.go` | Provider and policy resolution from config and flags. |
| `cmd/opencodereview/triage_cmd.go` | The `ocr triage` command. |

**Distribution surfaces:**

| File | Change |
|---|---|
| `skills/open-code-review-security-phx/SKILL.md` | New. |
| `plugins/open-code-review/skills/open-code-review-security-phx/SKILL.md` | New — mirror. |
| `plugins/open-code-review/claude-code/commands/ocr-review-security-phx.md` | New. |
| `plugins/open-code-review/claude-code/.claude-plugin/plugin.json` | Version bump. |
| `.claude-plugin/marketplace.json` | Version bump. |
| `plugins/open-code-review/.codex-plugin/plugin.json` | Security prompts. |
| `plugins/open-code-review/.cursor-plugin/plugin.json` | Keywords, version. |
| `plugins/open-code-review/opencode/open-code-review.ts` | `security` / `findings` inputs. |
| `scripts/sync-skills.sh`, `Makefile` | Skill-tree sync + drift check. |

---

## Task 1: The `ExternalFinding` model

**Files:**
- Create: `internal/findings/model.go`
- Test: `internal/findings/model_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `findings.ExternalFinding`, `findings.Kind` (`KindSAST`/`KindSCA`/`KindSecret`/`KindIaC`), `findings.Tristate` (`TriYes`/`TriNo`/`TriUnknown`), `findings.Reachability` (`Reachable`/`Unreachable`/`ReachUnknown`), `findings.Confidence` (`ConfHigh`/`ConfMedium`/`ConfLow`/`ConfUnknown`), `findings.EvidenceStep`, `func Normalize(f *ExternalFinding)`, `func (f ExternalFinding) ComputeFingerprint() string`.

- [ ] **Step 1: Write the failing test**

Create `internal/findings/model_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import "testing"

func TestNormalize_ZeroValuesBecomeUnknown(t *testing.T) {
	f := ExternalFinding{Source: "phoenix", RuleID: "go.sqli", Path: "a.go"}
	Normalize(&f)

	if f.Reachability != ReachUnknown {
		t.Errorf("Reachability = %q, want %q", f.Reachability, ReachUnknown)
	}
	if f.KEV != TriUnknown {
		t.Errorf("KEV = %q, want %q", f.KEV, TriUnknown)
	}
	if f.Malware != TriUnknown {
		t.Errorf("Malware = %q, want %q", f.Malware, TriUnknown)
	}
	if f.ExploitEvidence != TriUnknown {
		t.Errorf("ExploitEvidence = %q, want %q", f.ExploitEvidence, TriUnknown)
	}
	if f.Confidence != ConfUnknown {
		t.Errorf("Confidence = %q, want %q", f.Confidence, ConfUnknown)
	}
	if f.Kind != KindSAST {
		t.Errorf("Kind = %q, want %q", f.Kind, KindSAST)
	}
}

func TestNormalize_PreservesExplicitNegatives(t *testing.T) {
	f := ExternalFinding{KEV: TriNo, Reachability: Unreachable}
	Normalize(&f)

	if f.KEV != TriNo {
		t.Errorf("KEV = %q, want %q — an explicit negative must not be widened", f.KEV, TriNo)
	}
	if f.Reachability != Unreachable {
		t.Errorf("Reachability = %q, want %q", f.Reachability, Unreachable)
	}
}

func TestComputeFingerprint_StableAcrossLineShift(t *testing.T) {
	a := ExternalFinding{Source: "phoenix", RuleID: "go.sqli", Path: "a.go", StartLine: 10, Snippet: "db.Query(q)"}
	b := ExternalFinding{Source: "phoenix", RuleID: "go.sqli", Path: "a.go", StartLine: 42, Snippet: "  db.Query(q)  "}

	if a.ComputeFingerprint() != b.ComputeFingerprint() {
		t.Error("fingerprint changed when only the line number and surrounding whitespace differed")
	}
}

func TestComputeFingerprint_DiffersOnRule(t *testing.T) {
	a := ExternalFinding{Source: "phoenix", RuleID: "go.sqli", Path: "a.go", Snippet: "x"}
	b := ExternalFinding{Source: "phoenix", RuleID: "go.xss", Path: "a.go", Snippet: "x"}

	if a.ComputeFingerprint() == b.ComputeFingerprint() {
		t.Error("fingerprint collided across different rule ids")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/findings/... -run 'TestNormalize|TestComputeFingerprint' -v`
Expected: FAIL — the package does not compile (`undefined: ExternalFinding`).

- [ ] **Step 3: Write the implementation**

Create `internal/findings/model.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package findings models security findings produced by external scanners and
// prepares them for review. Every field whose absence could be mistaken for a
// negative result is three-state: "we did not check" is never "we checked and
// it is clean".
package findings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Kind classifies the scanner that produced a finding.
type Kind string

const (
	KindSAST   Kind = "sast"
	KindSCA    Kind = "sca"
	KindSecret Kind = "secret"
	KindIaC    Kind = "iac"
)

// Tristate is a yes/no answer that can also be "we did not check".
// The zero value is invalid; Normalize converts it to TriUnknown.
type Tristate string

const (
	TriYes     Tristate = "yes"
	TriNo      Tristate = "no"
	TriUnknown Tristate = "unknown"
)

// Reachability says whether the vulnerable code is reachable from the change.
type Reachability string

const (
	Reachable    Reachability = "reachable"
	Unreachable  Reachability = "unreachable"
	ReachUnknown Reachability = "unknown"
)

// Confidence is the scanner's own confidence in the finding.
type Confidence string

const (
	ConfHigh    Confidence = "high"
	ConfMedium  Confidence = "medium"
	ConfLow     Confidence = "low"
	ConfUnknown Confidence = "unknown"
)

// EvidenceStep is one hop in a taint path or dependency chain.
type EvidenceStep struct {
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Description string `json:"description,omitempty"`
}

// ExternalFinding is one security finding from a scanner outside ocr.
type ExternalFinding struct {
	ID          string `json:"id,omitempty"`
	Fingerprint string `json:"fingerprint"`

	Source string `json:"source"`
	RuleID string `json:"rule_id"`
	Kind   Kind   `json:"kind"`

	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Snippet   string `json:"snippet,omitempty"`

	Message  string `json:"message"`
	Severity string `json:"severity"`

	Confidence Confidence `json:"confidence"`

	CWE  string `json:"cwe,omitempty"`
	CVE  string `json:"cve,omitempty"`
	PURL string `json:"purl,omitempty"`

	Reachability    Reachability `json:"reachability"`
	KEV             Tristate     `json:"kev"`
	Malware         Tristate     `json:"malware"`
	ExploitEvidence Tristate     `json:"exploit_evidence"`

	Evidence []EvidenceStep `json:"evidence,omitempty"`

	// Raw is the untouched vendor payload, carried into the session JSONL so a
	// finding stays traceable to what the scanner actually said.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// Normalize fills zero-value fields with their safe defaults. Every unset
// three-state field becomes "unknown" — never the negative.
func Normalize(f *ExternalFinding) {
	if f.Kind == "" {
		f.Kind = KindSAST
	}
	if f.Confidence == "" {
		f.Confidence = ConfUnknown
	}
	if f.Reachability == "" {
		f.Reachability = ReachUnknown
	}
	if f.KEV == "" {
		f.KEV = TriUnknown
	}
	if f.Malware == "" {
		f.Malware = TriUnknown
	}
	if f.ExploitEvidence == "" {
		f.ExploitEvidence = TriUnknown
	}
	if f.Severity == "" {
		f.Severity = "medium"
	}
	if f.EndLine == 0 {
		f.EndLine = f.StartLine
	}
	if f.Fingerprint == "" {
		f.Fingerprint = f.ComputeFingerprint()
	}
}

// ComputeFingerprint returns a stable identity for the finding. It deliberately
// excludes line numbers: a finding that shifted because unrelated lines were
// added above it is the same finding, and resume must not treat it as new.
func (f ExternalFinding) ComputeFingerprint() string {
	h := sha256.New()
	for _, part := range []string{
		f.Source,
		f.RuleID,
		f.Path,
		f.CVE,
		f.PURL,
		normalizeSnippet(f.Snippet),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// normalizeSnippet collapses whitespace so cosmetic reformatting does not
// change a finding's identity.
func normalizeSnippet(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/findings/... -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Verify the repo gates**

Run: `make check`
Expected: `check passed`.

- [ ] **Step 6: Commit**

```bash
git add internal/findings/model.go internal/findings/model_test.go
git commit -m "feat(findings): add ExternalFinding model with three-state fields"
```

---

## Task 2: SARIF ingestion

**Files:**
- Create: `internal/findings/sarif.go`
- Test: `internal/findings/sarif_test.go`
- Create: `internal/findings/testdata/phoenix_pr_scan.sarif`, `internal/findings/testdata/semgrep.sarif`, `internal/findings/testdata/truncated.sarif`

**Interfaces:**
- Consumes: `findings.ExternalFinding`, `findings.Normalize` (Task 1).
- Produces: `func IngestSARIF(data []byte, defaultSource string) ([]ExternalFinding, error)`.

- [ ] **Step 1: Write the fixtures**

Create `internal/findings/testdata/phoenix_pr_scan.sarif`:

```json
{
  "version": "2.1.0",
  "runs": [
    {
      "tool": { "driver": { "name": "Phoenix Security", "rules": [
        { "id": "go.lang.security.sqli", "properties": { "security-severity": "8.8", "cwe": "CWE-89" } }
      ] } },
      "results": [
        {
          "ruleId": "go.lang.security.sqli",
          "level": "error",
          "message": { "text": "Possible SQL injection from unsanitised input." },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": { "uri": "internal/store/query.go" },
                "region": { "startLine": 42, "endLine": 44, "snippet": { "text": "db.Query(userInput)" } }
              }
            }
          ]
        }
      ]
    }
  ]
}
```

Create `internal/findings/testdata/semgrep.sarif`:

```json
{
  "version": "2.1.0",
  "runs": [
    {
      "tool": { "driver": { "name": "semgrep" } },
      "results": [
        {
          "ruleId": "generic.hardcoded-secret",
          "level": "warning",
          "message": { "text": "Hardcoded credential." },
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": { "uri": "config/app.go" },
                "region": { "startLine": 7 }
              }
            }
          ]
        }
      ]
    }
  ]
}
```

Create `internal/findings/testdata/truncated.sarif` (deliberately invalid — the closing braces are missing):

```json
{ "version": "2.1.0", "runs": [ { "tool": { "driver": { "name": "x" } }, "results": [
```

- [ ] **Step 2: Write the failing test**

Create `internal/findings/sarif_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestIngestSARIF_Phoenix(t *testing.T) {
	got, err := IngestSARIF(readFixture(t, "phoenix_pr_scan.sarif"), "")
	if err != nil {
		t.Fatalf("IngestSARIF: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	f := got[0]
	if f.Source != "Phoenix Security" {
		t.Errorf("Source = %q, want %q", f.Source, "Phoenix Security")
	}
	if f.RuleID != "go.lang.security.sqli" {
		t.Errorf("RuleID = %q", f.RuleID)
	}
	if f.Path != "internal/store/query.go" {
		t.Errorf("Path = %q", f.Path)
	}
	if f.StartLine != 42 || f.EndLine != 44 {
		t.Errorf("lines = %d..%d, want 42..44", f.StartLine, f.EndLine)
	}
	if f.Severity != "high" {
		t.Errorf("Severity = %q, want high (level=error)", f.Severity)
	}
	if f.CWE != "CWE-89" {
		t.Errorf("CWE = %q, want CWE-89", f.CWE)
	}
	if f.Reachability != ReachUnknown {
		t.Errorf("Reachability = %q, want unknown — SARIF carries no reachability", f.Reachability)
	}
	if f.Fingerprint == "" {
		t.Error("Fingerprint was not computed")
	}
}

func TestIngestSARIF_MissingEndLineDefaultsToStart(t *testing.T) {
	got, err := IngestSARIF(readFixture(t, "semgrep.sarif"), "")
	if err != nil {
		t.Fatalf("IngestSARIF: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].StartLine != 7 || got[0].EndLine != 7 {
		t.Errorf("lines = %d..%d, want 7..7", got[0].StartLine, got[0].EndLine)
	}
	if got[0].Severity != "medium" {
		t.Errorf("Severity = %q, want medium (level=warning)", got[0].Severity)
	}
}

func TestIngestSARIF_DefaultSourceWhenToolNameAbsent(t *testing.T) {
	doc := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{}},"results":[
		{"ruleId":"r","message":{"text":"m"},"locations":[{"physicalLocation":{
		"artifactLocation":{"uri":"a.go"},"region":{"startLine":1}}}]}]}]}`)
	got, err := IngestSARIF(doc, "fallback")
	if err != nil {
		t.Fatalf("IngestSARIF: %v", err)
	}
	if got[0].Source != "fallback" {
		t.Errorf("Source = %q, want %q", got[0].Source, "fallback")
	}
}

func TestIngestSARIF_TruncatedDocumentErrors(t *testing.T) {
	_, err := IngestSARIF(readFixture(t, "truncated.sarif"), "x")
	if err == nil {
		t.Fatal("expected an error for a truncated document — a malformed SARIF must never read as zero findings")
	}
}

func TestIngestSARIF_ResultWithoutLocationIsSkipped(t *testing.T) {
	doc := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},
		"results":[{"ruleId":"r","message":{"text":"m"},"locations":[]}]}]}`)
	got, err := IngestSARIF(doc, "t")
	if err != nil {
		t.Fatalf("IngestSARIF: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d findings, want 0 — a result with no physical location cannot be anchored", len(got))
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/findings/... -run TestIngestSARIF -v`
Expected: FAIL — `undefined: IngestSARIF`.

- [ ] **Step 4: Write the implementation**

Create `internal/findings/sarif.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// sarifDoc is the subset of SARIF 2.1.0 that ocr consumes. Unknown fields are
// ignored, but a document that does not parse at all is an error: a malformed
// scanner report must never be reported as "no findings".
type sarifDoc struct {
	Runs []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool struct {
		Driver struct {
			Name  string      `json:"name"`
			Rules []sarifRule `json:"rules"`
		} `json:"driver"`
	} `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifRule struct {
	ID         string          `json:"id"`
	Properties json.RawMessage `json:"properties"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
				EndLine   int `json:"endLine"`
				Snippet   struct {
					Text string `json:"text"`
				} `json:"snippet"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
	Properties json.RawMessage `json:"properties"`
}

type sarifRuleProps struct {
	SecuritySeverity string `json:"security-severity"`
	CWE              string `json:"cwe"`
}

// IngestSARIF converts a SARIF 2.1.0 document into ExternalFindings.
// defaultSource names the scanner when the document's tool driver does not.
func IngestSARIF(data []byte, defaultSource string) ([]ExternalFinding, error) {
	var doc sarifDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse SARIF: %w", err)
	}

	var out []ExternalFinding
	for _, run := range doc.Runs {
		source := run.Tool.Driver.Name
		if source == "" {
			source = defaultSource
		}

		props := make(map[string]sarifRuleProps, len(run.Tool.Driver.Rules))
		for _, r := range run.Tool.Driver.Rules {
			var p sarifRuleProps
			if len(r.Properties) > 0 {
				_ = json.Unmarshal(r.Properties, &p)
			}
			props[r.ID] = p
		}

		for _, res := range run.Results {
			if len(res.Locations) == 0 {
				continue
			}
			loc := res.Locations[0].PhysicalLocation
			if loc.ArtifactLocation.URI == "" {
				continue
			}

			f := ExternalFinding{
				Source:    source,
				RuleID:    res.RuleID,
				Kind:      KindSAST,
				Path:      loc.ArtifactLocation.URI,
				StartLine: loc.Region.StartLine,
				EndLine:   loc.Region.EndLine,
				Snippet:   loc.Region.Snippet.Text,
				Message:   res.Message.Text,
				Severity:  severityFromSARIF(res.Level, props[res.RuleID].SecuritySeverity),
				CWE:       props[res.RuleID].CWE,
				Raw:       res.Properties,
			}
			Normalize(&f)
			out = append(out, f)
		}
	}
	return out, nil
}

// severityFromSARIF prefers the numeric security-severity property (a CVSS
// score) and falls back to the SARIF level.
func severityFromSARIF(level, securitySeverity string) string {
	if score, err := strconv.ParseFloat(securitySeverity, 64); err == nil {
		switch {
		case score >= 9.0:
			return "critical"
		case score >= 7.0:
			return "high"
		case score >= 4.0:
			return "medium"
		default:
			return "low"
		}
	}
	switch level {
	case "error":
		return "high"
	case "warning":
		return "medium"
	case "note", "none":
		return "low"
	default:
		return "medium"
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/findings/... -v`
Expected: PASS (9 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/findings/sarif.go internal/findings/sarif_test.go internal/findings/testdata
git commit -m "feat(findings): ingest SARIF 2.1.0 documents into ExternalFinding"
```

---

## Task 3: Diff scoping

**Files:**
- Create: `internal/findings/scope.go`
- Test: `internal/findings/scope_test.go`

**Interfaces:**
- Consumes: `findings.ExternalFinding` (Task 1), `model.Diff`, `diff.ParseHunks`.
- Produces: `type ChangedLines map[string]map[int]struct{}`, `func BuildChangedLines(diffs []model.Diff) ChangedLines`, `func (c ChangedLines) Touches(f ExternalFinding) bool`.

Note: `diff.ParseHunks(rawDiffText string) []diff.Hunk` already exists in `internal/diff/hunk.go`. A `Hunk` has `NewStart`, `NewCount`, and `Lines []HunkLine` where `HunkLine.Type` is `HunkContext`, `HunkAdded`, or `HunkDeleted`. Walk the lines, advancing a new-file line counter for `HunkContext` and `HunkAdded` only.

- [ ] **Step 1: Write the failing test**

Create `internal/findings/scope_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

const sampleDiff = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -8,4 +8,6 @@
 func handler() {
-	old()
+	added1()
+	added2()
 	tail()
 }
`

func TestBuildChangedLines_MarksOnlyAddedLines(t *testing.T) {
	c := BuildChangedLines([]model.Diff{{NewPath: "a.go", Diff: sampleDiff}})

	if _, ok := c["a.go"]; !ok {
		t.Fatal("a.go missing from changed lines")
	}
	// Hunk starts at new line 8: 8=" func handler() {", 9="+added1()", 10="+added2()".
	for _, want := range []int{9, 10} {
		if _, ok := c["a.go"][want]; !ok {
			t.Errorf("line %d not marked changed", want)
		}
	}
	for _, notWant := range []int{8, 11, 12} {
		if _, ok := c["a.go"][notWant]; ok {
			t.Errorf("context line %d was marked changed", notWant)
		}
	}
}

func TestTouches_OverlapWithFindingRange(t *testing.T) {
	c := BuildChangedLines([]model.Diff{{NewPath: "a.go", Diff: sampleDiff}})

	cases := []struct {
		name  string
		f     ExternalFinding
		want  bool
	}{
		{"exact hit", ExternalFinding{Path: "a.go", StartLine: 9, EndLine: 9}, true},
		{"range spans a changed line", ExternalFinding{Path: "a.go", StartLine: 7, EndLine: 12}, true},
		{"entirely in context", ExternalFinding{Path: "a.go", StartLine: 11, EndLine: 12}, false},
		{"different file", ExternalFinding{Path: "b.go", StartLine: 9, EndLine: 9}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.Touches(tc.f); got != tc.want {
				t.Errorf("Touches() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildChangedLines_NewFileMarksEveryLine(t *testing.T) {
	d := model.Diff{NewPath: "n.go", IsNew: true, NewFileContent: "one\ntwo\nthree\n"}
	c := BuildChangedLines([]model.Diff{d})

	for _, want := range []int{1, 2, 3} {
		if _, ok := c["n.go"][want]; !ok {
			t.Errorf("line %d of a new file not marked changed", want)
		}
	}
}

func TestBuildChangedLines_SkipsDeletedAndBinary(t *testing.T) {
	c := BuildChangedLines([]model.Diff{
		{NewPath: "d.go", IsDeleted: true, Diff: sampleDiff},
		{NewPath: "b.bin", IsBinary: true, Diff: sampleDiff},
	})
	if len(c) != 0 {
		t.Errorf("got %d paths, want 0 — deleted and binary files carry no reviewable lines", len(c))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/findings/... -run 'TestBuildChangedLines|TestTouches' -v`
Expected: FAIL — `undefined: BuildChangedLines`.

- [ ] **Step 3: Write the implementation**

Create `internal/findings/scope.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"strings"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
)

// ChangedLines maps a repo-relative path to the set of new-file line numbers
// the diff actually added. Context lines are excluded: a finding sitting on an
// untouched line was not introduced by this change.
type ChangedLines map[string]map[int]struct{}

// BuildChangedLines derives the changed-line set from the run's diffs.
// Deleted and binary files contribute nothing. A new file contributes every line.
func BuildChangedLines(diffs []model.Diff) ChangedLines {
	out := make(ChangedLines)
	for _, d := range diffs {
		if d.IsDeleted || d.IsBinary || d.NewPath == "" {
			continue
		}

		lines := make(map[int]struct{})
		if d.IsNew && d.Diff == "" {
			for i := range strings.Split(strings.TrimRight(d.NewFileContent, "\n"), "\n") {
				lines[i+1] = struct{}{}
			}
		} else {
			for _, h := range diff.ParseHunks(d.Diff) {
				newLine := h.NewStart
				for _, hl := range h.Lines {
					switch hl.Type {
					case diff.HunkAdded:
						lines[newLine] = struct{}{}
						newLine++
					case diff.HunkContext:
						newLine++
					case diff.HunkDeleted:
						// Consumes no new-file line.
					}
				}
			}
		}

		if len(lines) > 0 {
			out[d.NewPath] = lines
		}
	}
	return out
}

// Touches reports whether any line in the finding's range was changed.
func (c ChangedLines) Touches(f ExternalFinding) bool {
	lines, ok := c[f.Path]
	if !ok {
		return false
	}
	start, end := f.StartLine, f.EndLine
	if end < start {
		end = start
	}
	for ln := start; ln <= end; ln++ {
		if _, hit := lines[ln]; hit {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/findings/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/findings/scope.go internal/findings/scope_test.go
git commit -m "feat(findings): scope findings to lines the diff actually changed"
```

---

## Task 4: The confidence policy

**Files:**
- Create: `internal/findings/policy.go`
- Test: `internal/findings/policy_test.go`

**Interfaces:**
- Consumes: `findings.ExternalFinding` and its enums (Task 1).
- Produces: `type Disposition string` (`DispPassThrough`/`DispTriage`/`DispDrop`), `type Policy struct`, `func DefaultPolicy() Policy`, `func (p Policy) Decide(f ExternalFinding, inDiff bool) Disposition`, `func (p Policy) Partition(fs []ExternalFinding, c ChangedLines) Partitioned`, `type Partitioned struct { PassThrough, Triage, Dropped []ExternalFinding }`.

This task implements spec §4.3 and carries the fail-closed invariant.

- [ ] **Step 1: Write the failing test**

Create `internal/findings/policy_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import "testing"

func TestPolicy_Decide(t *testing.T) {
	p := DefaultPolicy()

	cases := []struct {
		name   string
		f      ExternalFinding
		inDiff bool
		want   Disposition
	}{
		{
			name:   "outside the diff is dropped regardless of severity",
			f:      ExternalFinding{Severity: "critical", KEV: TriYes, Confidence: ConfHigh},
			inDiff: false,
			want:   DispDrop,
		},
		{
			name:   "critical and KEV passes through",
			f:      ExternalFinding{Severity: "critical", KEV: TriYes, Confidence: ConfUnknown, Reachability: ReachUnknown},
			inDiff: true,
			want:   DispPassThrough,
		},
		{
			name:   "high and high-confidence passes through",
			f:      ExternalFinding{Severity: "high", KEV: TriUnknown, Confidence: ConfHigh, Reachability: ReachUnknown},
			inDiff: true,
			want:   DispPassThrough,
		},
		{
			name:   "high and reachable passes through",
			f:      ExternalFinding{Severity: "high", KEV: TriUnknown, Confidence: ConfLow, Reachability: Reachable},
			inDiff: true,
			want:   DispPassThrough,
		},
		{
			name:   "high severity alone is triaged",
			f:      ExternalFinding{Severity: "high", KEV: TriUnknown, Confidence: ConfUnknown, Reachability: ReachUnknown},
			inDiff: true,
			want:   DispTriage,
		},
		{
			name:   "medium severity with every strong signal is still triaged",
			f:      ExternalFinding{Severity: "medium", KEV: TriYes, Confidence: ConfHigh, Reachability: Reachable},
			inDiff: true,
			want:   DispTriage,
		},
		{
			name:   "low severity is triaged, never dropped, when it is in the diff",
			f:      ExternalFinding{Severity: "low", Confidence: ConfLow, Reachability: Unreachable},
			inDiff: true,
			want:   DispTriage,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			Normalize(&f)
			if got := p.Decide(f, tc.inDiff); got != tc.want {
				t.Errorf("Decide() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPolicy_UnknownNeverPassesThrough is the fail-closed invariant from the
// spec: "we did not check" must never resolve as "we checked and it is clean",
// and must never be promoted to an unreviewed pass-through either.
func TestPolicy_UnknownNeverPassesThrough(t *testing.T) {
	p := DefaultPolicy()
	f := ExternalFinding{Severity: "critical"} // every three-state field unset
	Normalize(&f)

	if f.Reachability != ReachUnknown || f.KEV != TriUnknown || f.Confidence != ConfUnknown {
		t.Fatalf("precondition: Normalize did not produce all-unknown, got %+v", f)
	}
	if got := p.Decide(f, true); got != DispTriage {
		t.Errorf("Decide() = %q, want %q — an all-unknown finding must be adjudicated, not passed through", got, DispTriage)
	}
}

func TestPolicy_Partition(t *testing.T) {
	p := DefaultPolicy()
	c := ChangedLines{"a.go": {5: struct{}{}}}

	in := []ExternalFinding{
		{Path: "a.go", StartLine: 5, EndLine: 5, Severity: "critical", KEV: TriYes},
		{Path: "a.go", StartLine: 5, EndLine: 5, Severity: "medium"},
		{Path: "b.go", StartLine: 5, EndLine: 5, Severity: "critical", KEV: TriYes},
	}
	for i := range in {
		Normalize(&in[i])
	}

	got := p.Partition(in, c)
	if len(got.PassThrough) != 1 {
		t.Errorf("PassThrough = %d, want 1", len(got.PassThrough))
	}
	if len(got.Triage) != 1 {
		t.Errorf("Triage = %d, want 1", len(got.Triage))
	}
	if len(got.Dropped) != 1 {
		t.Errorf("Dropped = %d, want 1", len(got.Dropped))
	}
}

func TestPolicy_DisabledSendsEverythingToPassThrough(t *testing.T) {
	p := DefaultPolicy()
	p.TriageEnabled = false
	c := ChangedLines{"a.go": {5: struct{}{}}}

	in := []ExternalFinding{{Path: "a.go", StartLine: 5, EndLine: 5, Severity: "low"}}
	Normalize(&in[0])

	got := p.Partition(in, c)
	if len(got.Triage) != 0 {
		t.Errorf("Triage = %d, want 0 when triage is disabled", len(got.Triage))
	}
	if len(got.PassThrough) != 1 {
		t.Errorf("PassThrough = %d, want 1 — disabling triage must report, never suppress", len(got.PassThrough))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/findings/... -run TestPolicy -v`
Expected: FAIL — `undefined: DefaultPolicy`.

- [ ] **Step 3: Write the implementation**

Create `internal/findings/policy.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

// Disposition is what the pipeline does with one finding.
type Disposition string

const (
	// DispPassThrough reports the finding verbatim. The agent may add an
	// explanation and a fix, but may not dismiss it.
	DispPassThrough Disposition = "pass-through"
	// DispTriage sends the finding to the triage stage for adjudication.
	DispTriage Disposition = "triage"
	// DispDrop records the finding in the session but does not report it.
	// Reserved for findings outside the diff.
	DispDrop Disposition = "drop"
)

// Policy splits findings by confidence. See the design doc, section 4.3.
type Policy struct {
	// PassThroughSeverities are the severities eligible to skip adjudication.
	PassThroughSeverities []string
	// TriageEnabled is false under --no-triage. Findings that would have been
	// adjudicated are reported instead — disabling triage must never suppress.
	TriageEnabled bool
}

// DefaultPolicy returns the shipped policy.
func DefaultPolicy() Policy {
	return Policy{
		PassThroughSeverities: []string{"critical", "high"},
		TriageEnabled:         true,
	}
}

// Partitioned is the result of applying a Policy to a finding set.
type Partitioned struct {
	PassThrough []ExternalFinding
	Triage      []ExternalFinding
	Dropped     []ExternalFinding
}

// Decide returns the disposition for one finding.
//
// A finding outside the diff is dropped. Otherwise it passes through only when
// its severity is eligible AND at least one strong signal is affirmatively
// present: it is a known exploited vulnerability, the scanner is highly
// confident, or the vulnerable code is reachable. An absent signal is
// "unknown", which is never a strong signal — so an unchecked finding is
// always adjudicated rather than reported unreviewed.
func (p Policy) Decide(f ExternalFinding, inDiff bool) Disposition {
	if !inDiff {
		return DispDrop
	}
	if !p.TriageEnabled {
		return DispPassThrough
	}
	if !p.severityEligible(f.Severity) {
		return DispTriage
	}
	if f.KEV == TriYes || f.Confidence == ConfHigh || f.Reachability == Reachable {
		return DispPassThrough
	}
	return DispTriage
}

// Partition applies Decide to every finding.
func (p Policy) Partition(fs []ExternalFinding, c ChangedLines) Partitioned {
	var out Partitioned
	for _, f := range fs {
		switch p.Decide(f, c.Touches(f)) {
		case DispPassThrough:
			out.PassThrough = append(out.PassThrough, f)
		case DispTriage:
			out.Triage = append(out.Triage, f)
		default:
			out.Dropped = append(out.Dropped, f)
		}
	}
	return out
}

func (p Policy) severityEligible(sev string) bool {
	for _, s := range p.PassThroughSeverities {
		if s == sev {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/findings/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/findings/policy.go internal/findings/policy_test.go
git commit -m "feat(findings): add fail-closed confidence policy for finding dispositions"
```

---

## Task 5: The `Provider` interface and the SARIF file provider

**Files:**
- Create: `internal/findings/provider.go`
- Create: `internal/findings/providers/sarif/provider.go`
- Test: `internal/findings/providers/sarif/provider_test.go`

**Interfaces:**
- Consumes: `findings.ExternalFinding`, `findings.IngestSARIF` (Tasks 1–2).
- Produces: `findings.ScanRequest`, `findings.Result`, `findings.Provider`, and `sarifprov.New(path string) *sarifprov.Provider` (package `sarif`, imported as `sarifprov "github.com/alibaba/open-code-review/internal/findings/providers/sarif"`).

- [ ] **Step 1: Write the interface**

Create `internal/findings/provider.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import "context"

// ScanRequest describes the change a provider should scan.
type ScanRequest struct {
	// RepoDir is the absolute path to the repository root.
	RepoDir string
	// BaseRef and HeadRef bound the change. Either may be empty for a
	// workspace review, in which case the provider scans the working tree.
	BaseRef string
	HeadRef string
	// ChangedFiles lists every repo-relative path the review covers.
	ChangedFiles []string
	// ChangedManifests is the subset of ChangedFiles that are dependency
	// manifests or lockfiles. Providers that do not do SCA ignore it.
	ChangedManifests []string
	// PRNumber is optional and only used for provider-side correlation.
	PRNumber int
}

// Result is what a provider returns for one ScanRequest.
type Result struct {
	Findings []ExternalFinding
	// UpstreamVerdict is the provider's own gate decision, if it has one:
	// "PASS", "WARN", or "BLOCK". Empty when the provider does not gate.
	UpstreamVerdict string
	// Degraded reports that the scan did not complete as intended. A degraded
	// result must never be rendered as a clean security pass.
	Degraded bool
	// DegradedReason explains Degraded in one human-readable sentence.
	DegradedReason string
}

// Provider fetches security findings for a change.
type Provider interface {
	// Name identifies the provider in logs, session records, and warnings.
	Name() string
	// Fetch returns the findings for req. Implementations must return a
	// Degraded result rather than an empty clean one when they cannot
	// complete: "we did not scan" is never "we scanned and found nothing".
	Fetch(ctx context.Context, req ScanRequest) (Result, error)
}
```

- [ ] **Step 2: Write the failing test for the file provider**

Create `internal/findings/providers/sarif/provider_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package sarif

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/findings"
)

const validDoc = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"semgrep"}},
"results":[{"ruleId":"r1","level":"error","message":{"text":"boom"},
"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},
"region":{"startLine":3}}}]}]}]}`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan.sarif")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp sarif: %v", err)
	}
	return path
}

func TestProvider_Name(t *testing.T) {
	if got := New("x.sarif").Name(); got != "sarif-file" {
		t.Errorf("Name() = %q, want %q", got, "sarif-file")
	}
}

func TestProvider_Fetch(t *testing.T) {
	p := New(writeTemp(t, validDoc))

	res, err := p.Fetch(context.Background(), findings.ScanRequest{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(res.Findings))
	}
	if res.Findings[0].RuleID != "r1" {
		t.Errorf("RuleID = %q, want r1", res.Findings[0].RuleID)
	}
	if res.Degraded {
		t.Error("a successful read must not be marked degraded")
	}
	if res.UpstreamVerdict != "" {
		t.Errorf("UpstreamVerdict = %q, want empty — a file provider does not gate", res.UpstreamVerdict)
	}
}

func TestProvider_FetchMissingFileErrors(t *testing.T) {
	p := New(filepath.Join(t.TempDir(), "absent.sarif"))

	_, err := p.Fetch(context.Background(), findings.ScanRequest{})
	if err == nil {
		t.Fatal("expected an error for a missing findings file — a missing file must not read as zero findings")
	}
}

func TestProvider_FetchMalformedFileErrors(t *testing.T) {
	p := New(writeTemp(t, `{"runs":[`))

	_, err := p.Fetch(context.Background(), findings.ScanRequest{})
	if err == nil {
		t.Fatal("expected an error for a malformed SARIF document")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/findings/providers/sarif/... -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 4: Write the implementation**

Create `internal/findings/providers/sarif/provider.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package sarif provides a findings.Provider backed by a SARIF file on disk.
// It is the vendor-neutral path: any scanner that emits SARIF works with
// `ocr review --findings <file>` without ocr knowing anything about it.
package sarif

import (
	"context"
	"fmt"
	"os"

	"github.com/alibaba/open-code-review/internal/findings"
)

// Provider reads findings from a SARIF file.
type Provider struct {
	path string
}

// New returns a Provider reading the SARIF document at path.
func New(path string) *Provider { return &Provider{path: path} }

// Name implements findings.Provider.
func (p *Provider) Name() string { return "sarif-file" }

// Fetch implements findings.Provider. Read and parse failures are returned as
// errors rather than degraded empty results: the caller asked for a specific
// file, and silently reporting no findings would misrepresent an unread scan.
func (p *Provider) Fetch(_ context.Context, _ findings.ScanRequest) (findings.Result, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return findings.Result{}, fmt.Errorf("read findings file %q: %w", p.path, err)
	}
	fs, err := findings.IngestSARIF(data, "external-scanner")
	if err != nil {
		return findings.Result{}, fmt.Errorf("parse findings file %q: %w", p.path, err)
	}
	return findings.Result{Findings: fs}, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/findings/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/findings/provider.go internal/findings/providers/sarif
git commit -m "feat(findings): add Provider interface and SARIF file provider"
```

---

## Task 6: Comment provenance, conversion, and merge

**Files:**
- Modify: `internal/model/review.go`
- Create: `internal/findings/convert.go`
- Create: `internal/findings/dedup.go`
- Test: `internal/findings/convert_test.go`, `internal/findings/dedup_test.go`

**Interfaces:**
- Consumes: `findings.ExternalFinding` (Task 1), `model.LlmComment`.
- Produces: `model.LlmComment` fields `Provenance`, `Source`, `RuleID`, `CVE`, `Fingerprint`, `Verdict`; constants `model.ProvenanceLLM`, `model.ProvenanceScanner`, `model.ProvenanceScannerConfirmed`; `func ToComment(f ExternalFinding) model.LlmComment`; `func Dedup(fs []ExternalFinding) []ExternalFinding`; `func MergeComments(llmComments, scannerComments []model.LlmComment, proximity int) []model.LlmComment`.

- [ ] **Step 1: Write the failing tests**

Create `internal/findings/convert_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestToComment_CarriesProvenance(t *testing.T) {
	f := ExternalFinding{
		Source: "Phoenix Security", RuleID: "go.sqli", Path: "a.go",
		StartLine: 10, EndLine: 12, Message: "SQL injection", Severity: "high",
		CVE: "CVE-2024-1234",
	}
	Normalize(&f)

	cm := ToComment(f)

	if cm.Provenance != model.ProvenanceScanner {
		t.Errorf("Provenance = %q, want %q", cm.Provenance, model.ProvenanceScanner)
	}
	if cm.Category != "security" {
		t.Errorf("Category = %q, want security", cm.Category)
	}
	if cm.Path != "a.go" || cm.StartLine != 10 || cm.EndLine != 12 {
		t.Errorf("location = %s:%d-%d, want a.go:10-12", cm.Path, cm.StartLine, cm.EndLine)
	}
	if cm.Severity != "high" {
		t.Errorf("Severity = %q, want high", cm.Severity)
	}
	if cm.Source != "Phoenix Security" || cm.RuleID != "go.sqli" || cm.CVE != "CVE-2024-1234" {
		t.Errorf("provenance fields not carried: %+v", cm)
	}
	if cm.Fingerprint != f.Fingerprint {
		t.Error("Fingerprint not carried onto the comment")
	}
	if !strings.Contains(cm.Content, "SQL injection") {
		t.Errorf("Content = %q, want it to contain the scanner message", cm.Content)
	}
}

func TestToComment_UnknownReachabilityIsStatedNotOmitted(t *testing.T) {
	f := ExternalFinding{Kind: KindSCA, CVE: "CVE-1", PURL: "pkg:npm/x@1.0.0", Message: "vuln"}
	Normalize(&f)

	cm := ToComment(f)

	if !strings.Contains(strings.ToLower(cm.Content), "not determined") {
		t.Errorf("Content = %q, want an explicit statement that reachability was not determined", cm.Content)
	}
}
```

Create `internal/findings/dedup_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestDedup_CollapsesIdenticalFingerprints(t *testing.T) {
	a := ExternalFinding{Source: "s", RuleID: "r", Path: "a.go", StartLine: 10, Snippet: "x"}
	b := ExternalFinding{Source: "s", RuleID: "r", Path: "a.go", StartLine: 30, Snippet: "x"}
	c := ExternalFinding{Source: "s", RuleID: "other", Path: "a.go", StartLine: 10, Snippet: "x"}
	for _, f := range []*ExternalFinding{&a, &b, &c} {
		Normalize(f)
	}

	got := Dedup([]ExternalFinding{a, b, c})

	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2", len(got))
	}
	if got[0].StartLine != 10 {
		t.Errorf("kept StartLine = %d, want 10 — the first occurrence wins", got[0].StartLine)
	}
}

func TestMergeComments_ScannerWinsOnCollision(t *testing.T) {
	llm := []model.LlmComment{{
		Path: "a.go", StartLine: 10, EndLine: 10,
		Content: "This query looks unsafe.", Provenance: model.ProvenanceLLM,
	}}
	scanner := []model.LlmComment{{
		Path: "a.go", StartLine: 11, EndLine: 11,
		Content: "SQL injection.", Provenance: model.ProvenanceScanner,
		RuleID: "go.sqli",
	}}

	got := MergeComments(llm, scanner, 3)

	if len(got) != 1 {
		t.Fatalf("got %d comments, want 1 — nearby duplicates should collapse", len(got))
	}
	if got[0].Provenance != model.ProvenanceScanner {
		t.Errorf("Provenance = %q, want %q — the scanner comment carries a rule id and wins", got[0].Provenance, model.ProvenanceScanner)
	}
}

func TestMergeComments_KeepsDistantComments(t *testing.T) {
	llm := []model.LlmComment{{Path: "a.go", StartLine: 10, EndLine: 10, Provenance: model.ProvenanceLLM}}
	scanner := []model.LlmComment{{Path: "a.go", StartLine: 90, EndLine: 90, Provenance: model.ProvenanceScanner}}

	if got := MergeComments(llm, scanner, 3); len(got) != 2 {
		t.Fatalf("got %d comments, want 2", len(got))
	}
}

func TestMergeComments_KeepsDifferentFiles(t *testing.T) {
	llm := []model.LlmComment{{Path: "a.go", StartLine: 10, EndLine: 10, Provenance: model.ProvenanceLLM}}
	scanner := []model.LlmComment{{Path: "b.go", StartLine: 10, EndLine: 10, Provenance: model.ProvenanceScanner}}

	if got := MergeComments(llm, scanner, 3); len(got) != 2 {
		t.Fatalf("got %d comments, want 2", len(got))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/findings/... -run 'TestToComment|TestDedup|TestMergeComments' -v`
Expected: FAIL — `undefined: ToComment`, `undefined: Dedup`, `cm.Provenance undefined`.

- [ ] **Step 3: Extend the comment model**

In `internal/model/review.go`, add these constants above `LlmComment` and these fields at the end of the `LlmComment` struct:

```go
// Provenance values for LlmComment.Provenance.
const (
	// ProvenanceLLM is a comment the review agent produced on its own.
	ProvenanceLLM = "llm"
	// ProvenanceScanner is a finding reported verbatim from an external scanner.
	ProvenanceScanner = "scanner"
	// ProvenanceScannerConfirmed is a scanner finding the triage stage confirmed.
	ProvenanceScannerConfirmed = "scanner-confirmed"
)
```

Add to `LlmComment`:

```go
	// Provenance records who produced this comment: one of ProvenanceLLM,
	// ProvenanceScanner, or ProvenanceScannerConfirmed. Empty means ProvenanceLLM.
	Provenance string `json:"provenance,omitempty"`
	// Source names the scanner for a non-LLM comment (e.g. "Phoenix Security").
	Source string `json:"source,omitempty"`
	// RuleID is the scanner rule that fired.
	RuleID string `json:"rule_id,omitempty"`
	// CVE is set for dependency findings.
	CVE string `json:"cve,omitempty"`
	// Fingerprint joins the comment back to its ExternalFinding.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Verdict is the triage outcome: "confirmed", "dismissed", or "uncertain".
	Verdict string `json:"verdict,omitempty"`
```

- [ ] **Step 4: Write the conversion**

Create `internal/findings/convert.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

// ToComment renders a finding as a review comment. The rendered body states
// every three-state signal explicitly, including the ones that were not
// determined — an omitted signal reads as a negative to a human, which is
// exactly the misreading this design exists to prevent.
func ToComment(f ExternalFinding) model.LlmComment {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (`%s`)\n\n%s\n", strings.ToUpper(f.Severity), f.RuleID, f.Message)

	if f.CVE != "" {
		fmt.Fprintf(&b, "\n- CVE: `%s`", f.CVE)
	}
	if f.PURL != "" {
		fmt.Fprintf(&b, "\n- Component: `%s`", f.PURL)
	}
	if f.CWE != "" {
		fmt.Fprintf(&b, "\n- CWE: `%s`", f.CWE)
	}
	fmt.Fprintf(&b, "\n- Reachability: %s", describeReachability(f.Reachability))
	if f.Kind == KindSCA {
		fmt.Fprintf(&b, "\n- Known exploited: %s", describeTristate(f.KEV))
		fmt.Fprintf(&b, "\n- Exploit evidence: %s", describeTristate(f.ExploitEvidence))
	}
	fmt.Fprintf(&b, "\n\n_Reported by %s._", f.Source)

	return model.LlmComment{
		Path:        f.Path,
		Content:     b.String(),
		StartLine:   f.StartLine,
		EndLine:     f.EndLine,
		Category:    "security",
		Severity:    f.Severity,
		Provenance:  model.ProvenanceScanner,
		Source:      f.Source,
		RuleID:      f.RuleID,
		CVE:         f.CVE,
		Fingerprint: f.Fingerprint,
	}
}

func describeReachability(r Reachability) string {
	switch r {
	case Reachable:
		return "reachable from the changed code"
	case Unreachable:
		return "not reachable from the changed code"
	default:
		return "not determined"
	}
}

func describeTristate(t Tristate) string {
	switch t {
	case TriYes:
		return "yes"
	case TriNo:
		return "no"
	default:
		return "not determined"
	}
}
```

- [ ] **Step 5: Write the dedup and merge**

Create `internal/findings/dedup.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import "github.com/alibaba/open-code-review/internal/model"

// Dedup removes findings sharing a fingerprint, keeping the first occurrence
// and preserving input order.
func Dedup(fs []ExternalFinding) []ExternalFinding {
	seen := make(map[string]struct{}, len(fs))
	out := make([]ExternalFinding, 0, len(fs))
	for _, f := range fs {
		fp := f.Fingerprint
		if fp == "" {
			fp = f.ComputeFingerprint()
		}
		if _, dup := seen[fp]; dup {
			continue
		}
		seen[fp] = struct{}{}
		out = append(out, f)
	}
	return out
}

// MergeComments unions LLM comments with scanner comments, collapsing pairs
// that land within proximity lines of each other in the same file. The scanner
// comment wins a collision: it carries a rule id and a fingerprint that a
// reviewer can act on, which the LLM's prose restatement does not.
//
// Scanner comments are returned first so the merged output leads with the
// findings that have external corroboration.
func MergeComments(llmComments, scannerComments []model.LlmComment, proximity int) []model.LlmComment {
	out := make([]model.LlmComment, 0, len(llmComments)+len(scannerComments))
	out = append(out, scannerComments...)

	for _, lc := range llmComments {
		if collidesWithAny(lc, scannerComments, proximity) {
			continue
		}
		out = append(out, lc)
	}
	return out
}

func collidesWithAny(c model.LlmComment, others []model.LlmComment, proximity int) bool {
	for _, o := range others {
		if o.Path != c.Path {
			continue
		}
		if abs(o.StartLine-c.StartLine) <= proximity {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/findings/... ./internal/model/... -v`
Expected: PASS.

- [ ] **Step 7: Verify no existing test broke**

Run: `make test`
Expected: PASS. `LlmComment` gained only optional `omitempty` fields, so existing JSON golden comparisons are unaffected — if any fail, the golden file needs the new fields, not a rollback.

- [ ] **Step 8: Commit**

```bash
git add internal/model/review.go internal/findings/convert.go internal/findings/dedup.go internal/findings/convert_test.go internal/findings/dedup_test.go
git commit -m "feat(findings): add comment provenance, finding conversion, and merge"
```

---

## Task 7: The `finding_verdict` tool

**Files:**
- Modify: `internal/tool/definitions.go`
- Modify: `internal/config/toolsconfig/tools.json`
- Create: `internal/triage/verdict.go`
- Test: `internal/triage/verdict_test.go`

**Interfaces:**
- Consumes: `tool.Tool`, `tool.Provider` (existing).
- Produces: `tool.FindingVerdict` (a `tool.Tool`), `triage.Verdict` struct, `triage.VerdictCollector` with `Add(Verdict)` / `Verdicts() []Verdict`, `triage.NewVerdictProvider(*VerdictCollector) *VerdictProvider`.

Note: `tool.Tool` has an unexported `name` field, and `tool.IsReserved` / `tool.Dynamic` both consult `allTools()`. A new built-in tool **must** be added to both the `var` block and `allTools()`, or `mcp.RegisterAll` will happily let an MCP server shadow it.

- [ ] **Step 1: Write the failing test**

Create `internal/triage/verdict_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package triage

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/tool"
)

func TestVerdictProvider_Tool(t *testing.T) {
	p := NewVerdictProvider(NewVerdictCollector())
	if p.Tool() != tool.FindingVerdict {
		t.Errorf("Tool() = %v, want tool.FindingVerdict", p.Tool())
	}
}

func TestVerdictProvider_RecordsConfirmed(t *testing.T) {
	c := NewVerdictCollector()
	p := NewVerdictProvider(c)

	out, err := p.Execute(context.Background(), map[string]any{
		"fingerprint": "abc123",
		"verdict":     "confirmed",
		"rationale":   "The tainted value reaches db.Query with no escaping.",
		"fix":         "db.Query(\"...?\", userInput)",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "recorded") {
		t.Errorf("Execute() = %q, want an acknowledgement", out)
	}

	got := c.Verdicts()
	if len(got) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(got))
	}
	if got[0].Fingerprint != "abc123" || got[0].Verdict != VerdictConfirmed {
		t.Errorf("verdict = %+v", got[0])
	}
	if got[0].Fix == "" {
		t.Error("Fix was not recorded")
	}
}

func TestVerdictProvider_RejectsUnknownVerdict(t *testing.T) {
	c := NewVerdictCollector()
	p := NewVerdictProvider(c)

	out, err := p.Execute(context.Background(), map[string]any{
		"fingerprint": "abc",
		"verdict":     "probably-fine",
		"rationale":   "vibes",
	})
	if err != nil {
		t.Fatalf("Execute must report a bad argument to the model, not fail the run: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "error") {
		t.Errorf("Execute() = %q, want an error message the model can correct from", out)
	}
	if len(c.Verdicts()) != 0 {
		t.Error("an invalid verdict was recorded")
	}
}

func TestVerdictProvider_DismissalWithoutRationaleIsRejected(t *testing.T) {
	c := NewVerdictCollector()
	p := NewVerdictProvider(c)

	out, err := p.Execute(context.Background(), map[string]any{
		"fingerprint": "abc",
		"verdict":     "dismissed",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "rationale") {
		t.Errorf("Execute() = %q, want the model told a dismissal needs a rationale", out)
	}
	if len(c.Verdicts()) != 0 {
		t.Error("a dismissal with no rationale was recorded — every suppression must be justified")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/triage/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Register the built-in tool**

In `internal/tool/definitions.go`, add to the `var` block and to `allTools()`:

```go
	FindingVerdict = Tool{name: "finding_verdict"}
```

```go
func allTools() []Tool {
	return []Tool{Unknown, TaskDone, CodeComment, FileRead, FileFind, FileReadDiff, CodeSearch, FindingVerdict}
}
```

- [ ] **Step 4: Add the tool definition**

Append this object to the array in `internal/config/toolsconfig/tools.json`:

```json
  {
    "name": "finding_verdict",
    "plan_task": false,
    "main_task": false,
    "triage_task": true,
    "definition": {
      "name": "finding_verdict",
      "description": "Record your adjudication of one external scanner finding. Call this exactly once per finding you were asked to adjudicate. A dismissal requires a rationale that names the specific reason the finding does not apply to this change.",
      "parameters": {
        "type": "object",
        "properties": {
          "fingerprint": {
            "type": "string",
            "description": "The fingerprint of the finding being adjudicated, copied verbatim from the finding you were given."
          },
          "verdict": {
            "type": "string",
            "enum": ["confirmed", "dismissed", "uncertain"],
            "description": "confirmed: the finding is a real problem in this change. dismissed: the finding does not apply here. uncertain: you could not determine this from the available context — use this rather than guessing."
          },
          "rationale": {
            "type": "string",
            "description": "Why you reached this verdict, citing the specific code you examined. Required for 'dismissed'."
          },
          "fix": {
            "type": "string",
            "description": "Optional replacement code that resolves the finding."
          }
        },
        "required": ["fingerprint", "verdict"]
      }
    }
  }
```

Add the `TriageTask bool` field to the tool config struct in `internal/config/toolsconfig/toolsconfig.go`, mirroring the existing `PlanTask` / `MainTask` fields with the JSON tag `triage_task`.

- [ ] **Step 5: Write the implementation**

Create `internal/triage/verdict.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package triage adjudicates external scanner findings against the change
// under review, using an LLM tool-use loop with the same runner that drives
// review and scan.
package triage

import (
	"context"
	"strings"
	"sync"

	"github.com/alibaba/open-code-review/internal/tool"
)

// Verdict outcomes.
const (
	VerdictConfirmed = "confirmed"
	VerdictDismissed = "dismissed"
	VerdictUncertain = "uncertain"
)

// Verdict is the triage stage's adjudication of one finding.
type Verdict struct {
	Fingerprint string `json:"fingerprint"`
	Verdict     string `json:"verdict"`
	Rationale   string `json:"rationale,omitempty"`
	Fix         string `json:"fix,omitempty"`
}

// VerdictCollector is a thread-safe store of verdicts for one triage run.
type VerdictCollector struct {
	mu       sync.Mutex
	verdicts []Verdict
}

// NewVerdictCollector returns an empty collector.
func NewVerdictCollector() *VerdictCollector { return &VerdictCollector{} }

// Add records a verdict.
func (c *VerdictCollector) Add(v Verdict) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verdicts = append(c.verdicts, v)
}

// Verdicts returns a copy of every recorded verdict.
func (c *VerdictCollector) Verdicts() []Verdict {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Verdict, len(c.verdicts))
	copy(out, c.verdicts)
	return out
}

// VerdictProvider implements the finding_verdict tool.
type VerdictProvider struct {
	collector *VerdictCollector
}

// NewVerdictProvider returns a provider writing into collector.
func NewVerdictProvider(collector *VerdictCollector) *VerdictProvider {
	return &VerdictProvider{collector: collector}
}

// Tool implements tool.Provider.
func (p *VerdictProvider) Tool() tool.Tool { return tool.FindingVerdict }

// Execute implements tool.Provider. Argument problems are returned to the
// model as text rather than as errors, so it can correct itself in the next
// turn instead of failing the run.
func (p *VerdictProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	fingerprint, _ := args["fingerprint"].(string)
	if strings.TrimSpace(fingerprint) == "" {
		return "Error: 'fingerprint' is required and must be copied verbatim from the finding.", nil
	}

	verdict, _ := args["verdict"].(string)
	switch verdict {
	case VerdictConfirmed, VerdictDismissed, VerdictUncertain:
	default:
		return "Error: 'verdict' must be one of 'confirmed', 'dismissed', or 'uncertain'.", nil
	}

	rationale, _ := args["rationale"].(string)
	if verdict == VerdictDismissed && strings.TrimSpace(rationale) == "" {
		return "Error: a 'dismissed' verdict requires a 'rationale' naming why the finding does not apply to this change.", nil
	}

	fix, _ := args["fix"].(string)
	p.collector.Add(Verdict{
		Fingerprint: fingerprint,
		Verdict:     verdict,
		Rationale:   rationale,
		Fix:         fix,
	})
	return "Verdict recorded.", nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/triage/... ./internal/tool/... ./internal/config/toolsconfig/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tool/definitions.go internal/config/toolsconfig internal/triage
git commit -m "feat(triage): add finding_verdict tool for adjudicating scanner findings"
```

---

## Task 8: The triage prompts

**Files:**
- Create: `internal/config/template/prompts/triage_task_system.md`
- Create: `internal/config/template/prompts/triage_task_user.md`
- Modify: `internal/config/template/task_template.json`
- Modify: `internal/config/template/template.go`
- Test: `internal/config/template/template_test.go` (add cases)

**Interfaces:**
- Consumes: `template.LlmConversation`, `template.Template` (existing).
- Produces: `Template.TriageTask *LlmConversation` resolved from `TRIAGE_TASK` in `task_template.json`.

Placeholders used by the user prompt, which Task 9 substitutes: `{{finding_block}}`, `{{diff}}`, `{{current_file_path}}`, `{{requirement_background}}`, `{{current_system_date_time}}`.

- [ ] **Step 1: Write the system prompt**

Create `internal/config/template/prompts/triage_task_system.md`:

```markdown
You are a security engineer adjudicating findings reported by an automated
scanner against a specific code change. You are not re-running the scanner and
you are not reviewing the change for new issues. Your only job is to decide,
for each finding you are given, whether it is a real problem in this change.

## How to decide

For each finding:

1. Read the code at the reported location. Use `file_read` and
   `file_read_diff`; the reported line may have shifted.
2. Establish whether the vulnerable condition actually holds here. For an
   injection or taint finding, trace the value from its source to the reported
   sink. For a dependency finding, determine whether the vulnerable API is
   called at all.
3. Use `code_search` to find callers, and the graph tools when they are
   available, to establish reachability. Do not assume reachability from the
   fact that the code exists.
4. Call `finding_verdict` exactly once for the finding.

## Verdicts

- `confirmed` — the vulnerable condition holds in this change. Provide a `fix`
  when you can write one that is correct in this file's context.
- `dismissed` — the finding does not apply. The `rationale` must name the
  specific reason: the input is already validated at a named location, the
  vulnerable API is never called, the path is unreachable because of a named
  guard. "Looks fine" and "probably a false positive" are not rationales.
- `uncertain` — you could not establish either. Use this rather than guessing.
  An uncertain finding is reported to the reviewer with your notes attached.

## Rules

- A finding whose reachability was not determined is not thereby unreachable.
  Absence of evidence is `uncertain`, never `dismissed`.
- Never dismiss a finding because it is inconvenient, low-severity, or common.
  Severity is not your decision; applicability is.
- Adjudicate only the findings you were given. If you notice an unrelated
  problem, ignore it — a separate review pass covers that.
- When you have adjudicated every finding, call `task_done`.
```

- [ ] **Step 2: Write the user prompt**

Create `internal/config/template/prompts/triage_task_user.md`:

```markdown
Current time: {{current_system_date_time}}

## Change under review

File: `{{current_file_path}}`

```diff
{{diff}}
```

## Requirement background

{{requirement_background}}

## Findings to adjudicate

{{finding_block}}

Adjudicate each finding above. Call `finding_verdict` once per finding, then
call `task_done`.
```

- [ ] **Step 3: Register the task**

Add to `internal/config/template/task_template.json`, after `REVIEW_FILTER_TASK`:

```json
  "TRIAGE_TASK": {
    "messages": [
      { "role": "system", "prompt_file": "triage_task_system.md" },
      { "role": "user", "prompt_file": "triage_task_user.md" }
    ]
  },
```

In `internal/config/template/template.go`:
- add `TriageTask *LlmConversation \`json:"TRIAGE_TASK,omitempty"\`` to `Template`;
- add `TriageTask *manifestConversation \`json:"TRIAGE_TASK,omitempty"\`` to `templateManifest`;
- in `LoadDefault`, after the `ReviewFilterTask` resolution, add:

```go
	if tpl.TriageTask, err = resolveOptionalConversation(m.TriageTask, "TRIAGE_TASK"); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Write the failing test**

Add to `internal/config/template/template_test.go`:

```go
func TestLoadDefault_TriageTask(t *testing.T) {
	tpl, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if tpl.TriageTask == nil {
		t.Fatal("TriageTask is nil — TRIAGE_TASK was not resolved")
	}
	if len(tpl.TriageTask.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(tpl.TriageTask.Messages))
	}
	if tpl.TriageTask.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", tpl.TriageTask.Messages[0].Role)
	}

	user := tpl.TriageTask.Messages[1].Content
	for _, ph := range []string{"{{finding_block}}", "{{diff}}", "{{current_file_path}}", "{{requirement_background}}"} {
		if !strings.Contains(user, ph) {
			t.Errorf("triage user prompt is missing placeholder %s", ph)
		}
	}
}
```

Add `"strings"` to that file's imports if it is not already there.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/config/template/... -run TestLoadDefault -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/template
git commit -m "feat(triage): add triage task prompts and template wiring"
```

---

## Task 9: The triage stage

**Files:**
- Create: `internal/triage/cluster.go`
- Create: `internal/triage/triage.go`
- Test: `internal/triage/cluster_test.go`, `internal/triage/triage_test.go`

**Interfaces:**
- Consumes: `findings.ExternalFinding` (Task 1), `triage.VerdictCollector` (Task 7), `template.Template.TriageTask` (Task 8), `llmloop.Runner` and `llmloop.Deps` (existing).
- Produces: `type Unit struct { Key string; Path string; Findings []findings.ExternalFinding }`, `func Cluster(fs []findings.ExternalFinding) []Unit`, `func RenderFindingBlock(u Unit) string`, `type Stage struct`, `func NewStage(opts StageOptions) *Stage`, `func (s *Stage) Run(ctx context.Context, units []Unit) ([]Verdict, error)`.

Clustering rule: group by `Path`, then split any group larger than `maxPerUnit` (8) into chunks. One unit is one LLM conversation, so the unit key doubles as the `newPath` argument to `Runner.RunPerFile`.

- [ ] **Step 1: Write the failing clustering test**

Create `internal/triage/cluster_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package triage

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/findings"
)

func mkFinding(path, rule string, line int) findings.ExternalFinding {
	f := findings.ExternalFinding{
		Source: "phoenix", RuleID: rule, Path: path,
		StartLine: line, EndLine: line, Message: "m", Severity: "high",
		Snippet: rule + string(rune(line)),
	}
	findings.Normalize(&f)
	return f
}

func TestCluster_GroupsByPath(t *testing.T) {
	in := []findings.ExternalFinding{
		mkFinding("a.go", "r1", 1),
		mkFinding("b.go", "r2", 2),
		mkFinding("a.go", "r3", 3),
	}

	got := Cluster(in)

	if len(got) != 2 {
		t.Fatalf("got %d units, want 2", len(got))
	}
	byPath := map[string]int{}
	for _, u := range got {
		byPath[u.Path] = len(u.Findings)
	}
	if byPath["a.go"] != 2 {
		t.Errorf("a.go unit has %d findings, want 2", byPath["a.go"])
	}
	if byPath["b.go"] != 1 {
		t.Errorf("b.go unit has %d findings, want 1", byPath["b.go"])
	}
}

func TestCluster_SplitsOversizedGroups(t *testing.T) {
	var in []findings.ExternalFinding
	for i := 0; i < 20; i++ {
		in = append(in, mkFinding("big.go", "r", i+1))
	}

	got := Cluster(in)

	if len(got) != 3 {
		t.Fatalf("got %d units, want 3 (20 findings at 8 per unit)", len(got))
	}
	total := 0
	keys := map[string]struct{}{}
	for _, u := range got {
		total += len(u.Findings)
		if len(u.Findings) > 8 {
			t.Errorf("unit %q has %d findings, want at most 8", u.Key, len(u.Findings))
		}
		if _, dup := keys[u.Key]; dup {
			t.Errorf("duplicate unit key %q", u.Key)
		}
		keys[u.Key] = struct{}{}
	}
	if total != 20 {
		t.Errorf("units hold %d findings, want 20 — clustering must not drop findings", total)
	}
}

func TestCluster_EmptyInput(t *testing.T) {
	if got := Cluster(nil); len(got) != 0 {
		t.Errorf("got %d units, want 0", len(got))
	}
}

func TestRenderFindingBlock_IncludesFingerprintAndUnknowns(t *testing.T) {
	u := Unit{Key: "a.go#0", Path: "a.go", Findings: []findings.ExternalFinding{mkFinding("a.go", "r1", 5)}}

	got := RenderFindingBlock(u)

	if !strings.Contains(got, u.Findings[0].Fingerprint) {
		t.Error("rendered block omits the fingerprint the model must echo back")
	}
	if !strings.Contains(got, "not determined") {
		t.Error("rendered block omits the explicit 'not determined' reachability state")
	}
	if !strings.Contains(got, "a.go:5") {
		t.Errorf("rendered block omits the location; got:\n%s", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/triage/... -run 'TestCluster|TestRenderFindingBlock' -v`
Expected: FAIL — `undefined: Cluster`.

- [ ] **Step 3: Write the clustering implementation**

Create `internal/triage/cluster.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package triage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/findings"
)

// maxFindingsPerUnit caps how many findings share one conversation. Beyond
// this the model's attention degrades and verdicts start being skipped.
const maxFindingsPerUnit = 8

// Unit is one triage conversation: a set of findings in one file.
type Unit struct {
	// Key uniquely identifies the unit within a run. It is passed to
	// Runner.RunPerFile as the file key, so it must be unique and stable.
	Key string
	// Path is the repo-relative file the findings sit in.
	Path string
	// Findings are the findings to adjudicate in this conversation.
	Findings []findings.ExternalFinding
}

// Cluster groups findings into units of work, one file at a time, splitting
// any file with more than maxFindingsPerUnit findings across several units.
// Units are returned in a deterministic order so a resumed run rebuilds the
// same keys.
func Cluster(fs []findings.ExternalFinding) []Unit {
	byPath := make(map[string][]findings.ExternalFinding)
	for _, f := range fs {
		byPath[f.Path] = append(byPath[f.Path], f)
	}

	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var out []Unit
	for _, p := range paths {
		group := byPath[p]
		for i, chunk := 0, 0; i < len(group); i, chunk = i+maxFindingsPerUnit, chunk+1 {
			end := i + maxFindingsPerUnit
			if end > len(group) {
				end = len(group)
			}
			out = append(out, Unit{
				Key:      fmt.Sprintf("%s#%d", p, chunk),
				Path:     p,
				Findings: group[i:end],
			})
		}
	}
	return out
}

// RenderFindingBlock formats a unit's findings for the triage prompt. Every
// three-state signal is stated explicitly, including the undetermined ones:
// an omitted signal reads to the model as a negative.
func RenderFindingBlock(u Unit) string {
	var b strings.Builder
	for i, f := range u.Findings {
		fmt.Fprintf(&b, "### Finding %d\n\n", i+1)
		fmt.Fprintf(&b, "- fingerprint: `%s`\n", f.Fingerprint)
		fmt.Fprintf(&b, "- scanner: %s\n", f.Source)
		fmt.Fprintf(&b, "- rule: `%s`\n", f.RuleID)
		fmt.Fprintf(&b, "- location: `%s:%d`", f.Path, f.StartLine)
		if f.EndLine > f.StartLine {
			fmt.Fprintf(&b, "-%d", f.EndLine)
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "- severity: %s\n", f.Severity)
		fmt.Fprintf(&b, "- scanner confidence: %s\n", f.Confidence)
		fmt.Fprintf(&b, "- reachability: %s\n", describeReachability(f.Reachability))
		if f.CVE != "" {
			fmt.Fprintf(&b, "- CVE: `%s`\n", f.CVE)
		}
		if f.PURL != "" {
			fmt.Fprintf(&b, "- component: `%s`\n", f.PURL)
		}
		if f.CWE != "" {
			fmt.Fprintf(&b, "- CWE: `%s`\n", f.CWE)
		}
		fmt.Fprintf(&b, "\n%s\n", f.Message)
		if len(f.Evidence) > 0 {
			b.WriteString("\nReported flow:\n")
			for _, step := range f.Evidence {
				fmt.Fprintf(&b, "  - `%s:%d` %s\n", step.Path, step.Line, step.Description)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func describeReachability(r findings.Reachability) string {
	switch r {
	case findings.Reachable:
		return "reachable from the changed code"
	case findings.Unreachable:
		return "not reachable from the changed code"
	default:
		return "not determined — you must establish this yourself"
	}
}
```

- [ ] **Step 4: Run the clustering tests to verify they pass**

Run: `go test ./internal/triage/... -run 'TestCluster|TestRenderFindingBlock' -v`
Expected: PASS.

- [ ] **Step 5: Write the failing stage test**

Create `internal/triage/triage_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package triage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/findings"
	"github.com/alibaba/open-code-review/internal/llm"
)

// fakeRunner records the conversations it was asked to run and reports the
// verdicts a scripted model would have produced.
type fakeRunner struct {
	calls     []string
	prompts   []string
	onRun     func(unitKey string, collector *VerdictCollector)
	returnErr error
}

func (f *fakeRunner) RunUnit(_ context.Context, messages []llm.Message, unitKey string, collector *VerdictCollector) error {
	f.calls = append(f.calls, unitKey)
	for _, m := range messages {
		f.prompts = append(f.prompts, m.Content)
	}
	if f.returnErr != nil {
		return f.returnErr
	}
	if f.onRun != nil {
		f.onRun(unitKey, collector)
	}
	return nil
}

func newTestStage(r *fakeRunner) *Stage {
	return NewStage(StageOptions{
		SystemPrompt: "system",
		UserPrompt:   "file {{current_file_path}} findings {{finding_block}} diff {{diff}} bg {{requirement_background}}",
		DiffLookup:   func(path string) string { return "@@ -1 +1 @@\n+x\n" },
		Background:   "ship the thing",
		Concurrency:  1,
		runUnit:      r.RunUnit,
	})
}

func TestStage_Run_ProducesVerdictPerUnit(t *testing.T) {
	r := &fakeRunner{onRun: func(key string, c *VerdictCollector) {
		c.Add(Verdict{Fingerprint: "fp-" + key, Verdict: VerdictConfirmed, Rationale: "real"})
	}}
	s := newTestStage(r)

	units := Cluster([]findings.ExternalFinding{mkFinding("a.go", "r1", 1), mkFinding("b.go", "r2", 2)})
	got, err := s.Run(context.Background(), units)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d verdicts, want 2", len(got))
	}
	if len(r.calls) != 2 {
		t.Errorf("runner called %d times, want 2", len(r.calls))
	}
}

func TestStage_Run_SubstitutesPlaceholders(t *testing.T) {
	r := &fakeRunner{}
	s := newTestStage(r)

	units := Cluster([]findings.ExternalFinding{mkFinding("a.go", "r1", 5)})
	if _, err := s.Run(context.Background(), units); err != nil {
		t.Fatalf("Run: %v", err)
	}

	joined := strings.Join(r.prompts, "\n")
	for _, ph := range []string{"{{current_file_path}}", "{{finding_block}}", "{{diff}}", "{{requirement_background}}"} {
		if strings.Contains(joined, ph) {
			t.Errorf("placeholder %s was not substituted", ph)
		}
	}
	if !strings.Contains(joined, "a.go") {
		t.Error("prompt does not name the file under triage")
	}
	if !strings.Contains(joined, "ship the thing") {
		t.Error("prompt does not carry the requirement background")
	}
}

// TestStage_Run_UnadjudicatedFindingBecomesUncertain is the safety net: a model
// that returns without calling finding_verdict must not cause the finding to
// vanish. Silence is uncertainty, not dismissal.
func TestStage_Run_UnadjudicatedFindingBecomesUncertain(t *testing.T) {
	r := &fakeRunner{} // records nothing
	s := newTestStage(r)

	f := mkFinding("a.go", "r1", 1)
	got, err := s.Run(context.Background(), Cluster([]findings.ExternalFinding{f}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d verdicts, want 1 — an unadjudicated finding must still produce a verdict", len(got))
	}
	if got[0].Verdict != VerdictUncertain {
		t.Errorf("Verdict = %q, want %q", got[0].Verdict, VerdictUncertain)
	}
	if got[0].Fingerprint != f.Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", got[0].Fingerprint, f.Fingerprint)
	}
}

func TestStage_Run_UnitErrorYieldsUncertainNotLoss(t *testing.T) {
	r := &fakeRunner{returnErr: errors.New("llm exploded")}
	s := newTestStage(r)

	f := mkFinding("a.go", "r1", 1)
	got, err := s.Run(context.Background(), Cluster([]findings.ExternalFinding{f}))
	if err != nil {
		t.Fatalf("Run must not fail the whole stage for one failed unit: %v", err)
	}
	if len(got) != 1 || got[0].Verdict != VerdictUncertain {
		t.Fatalf("got %+v, want one uncertain verdict", got)
	}
	if !strings.Contains(got[0].Rationale, "llm exploded") {
		t.Errorf("Rationale = %q, want it to name the failure", got[0].Rationale)
	}
}
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test ./internal/triage/... -run TestStage -v`
Expected: FAIL — `undefined: NewStage`.

- [ ] **Step 7: Write the stage implementation**

Create `internal/triage/triage.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package triage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/tool"
)

// runUnitFn executes one triage conversation. It is a field rather than a
// direct llmloop call so the stage can be tested without an LLM.
type runUnitFn func(ctx context.Context, messages []llm.Message, unitKey string, collector *VerdictCollector) error

// StageOptions configures a triage Stage.
type StageOptions struct {
	// SystemPrompt and UserPrompt come from template.Template.TriageTask.
	SystemPrompt string
	UserPrompt   string
	// DiffLookup returns the raw diff text for a path, or "" when the file is
	// not part of the change.
	DiffLookup func(path string) string
	// Background is the run's requirement context.
	Background string
	// Concurrency caps units in flight. Values below 1 are treated as 1.
	Concurrency int

	// Runner and Tools are the production execution path. When runUnit is set
	// (tests), they are ignored.
	Runner *llmloop.Runner
	Tools  *tool.Registry

	runUnit runUnitFn
}

// Stage adjudicates findings, one conversation per unit.
type Stage struct {
	opts StageOptions
}

// NewStage returns a Stage bound to opts.
func NewStage(opts StageOptions) *Stage {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.runUnit == nil {
		opts.runUnit = defaultRunUnit(opts.Runner)
	}
	return &Stage{opts: opts}
}

// defaultRunUnit drives the shared llmloop.Runner. The runner's own comment
// collector picks up any code_comment calls; verdicts arrive through the
// finding_verdict tool provider registered against collector.
func defaultRunUnit(runner *llmloop.Runner) runUnitFn {
	return func(ctx context.Context, messages []llm.Message, unitKey string, _ *VerdictCollector) error {
		if runner == nil {
			return fmt.Errorf("triage: no runner configured")
		}
		_, stop, err := runner.RunPerFile(ctx, messages, unitKey)
		if err != nil {
			return fmt.Errorf("triage unit %q: %w", unitKey, err)
		}
		if stop.Reason() != "" {
			return fmt.Errorf("triage unit %q stopped: %s", unitKey, stop.Reason())
		}
		return nil
	}
}

// Run adjudicates every unit and returns one verdict per finding.
//
// A unit that fails, or that the model finishes without adjudicating, still
// yields a verdict — an "uncertain" one. A finding must never disappear
// because the machinery that was supposed to judge it did not.
func (s *Stage) Run(ctx context.Context, units []Unit) ([]Verdict, error) {
	collector := NewVerdictCollector()

	failures := make(map[string]string)
	var failMu sync.Mutex

	sem := make(chan struct{}, s.opts.Concurrency)
	var wg sync.WaitGroup

	for _, u := range units {
		u := u
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			messages := s.buildMessages(u)
			if err := s.opts.runUnit(ctx, messages, u.Key, collector); err != nil {
				failMu.Lock()
				failures[u.Key] = err.Error()
				failMu.Unlock()
			}
		}()
	}
	wg.Wait()

	return s.reconcile(units, collector.Verdicts(), failures), nil
}

// buildMessages substitutes the triage prompt placeholders for one unit.
func (s *Stage) buildMessages(u Unit) []llm.Message {
	diffText := ""
	if s.opts.DiffLookup != nil {
		diffText = s.opts.DiffLookup(u.Path)
	}

	user := s.opts.UserPrompt
	user = strings.ReplaceAll(user, "{{current_file_path}}", u.Path)
	user = strings.ReplaceAll(user, "{{finding_block}}", RenderFindingBlock(u))
	user = strings.ReplaceAll(user, "{{diff}}", diffText)
	user = strings.ReplaceAll(user, "{{requirement_background}}", s.opts.Background)
	user = strings.ReplaceAll(user, "{{current_system_date_time}}", time.Now().Format(time.RFC3339))

	return []llm.Message{
		{Role: "system", Content: s.opts.SystemPrompt},
		{Role: "user", Content: user},
	}
}

// reconcile guarantees exactly one verdict per finding. Recorded verdicts win;
// anything unaccounted for becomes uncertain, with the unit's failure reason
// attached when there was one.
func (s *Stage) reconcile(units []Unit, recorded []Verdict, failures map[string]string) []Verdict {
	byFingerprint := make(map[string]Verdict, len(recorded))
	for _, v := range recorded {
		byFingerprint[v.Fingerprint] = v
	}

	var out []Verdict
	for _, u := range units {
		for _, f := range u.Findings {
			if v, ok := byFingerprint[f.Fingerprint]; ok {
				out = append(out, v)
				continue
			}
			rationale := "The triage stage did not return a verdict for this finding."
			if reason, failed := failures[u.Key]; failed {
				rationale = fmt.Sprintf("Triage did not complete for this finding: %s", reason)
			}
			out = append(out, Verdict{
				Fingerprint: f.Fingerprint,
				Verdict:     VerdictUncertain,
				Rationale:   rationale,
			})
		}
	}
	return out
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/triage/... -v`
Expected: PASS.

- [ ] **Step 9: Run the full suite and coverage**

Run: `make test && make coverage`
Expected: PASS, coverage at or above 90%.

- [ ] **Step 10: Commit**

```bash
git add internal/triage
git commit -m "feat(triage): add the triage stage with per-finding verdict reconciliation"
```

---

## Task 10: Session records and the schema bump

**Files:**
- Modify: `internal/session/persist.go`
- Modify: `internal/session/manifest.go`
- Modify: `internal/agent/identity.go`
- Test: `internal/session/persist_test.go` (add cases), `internal/agent/identity_test.go` (add cases)

**Interfaces:**
- Consumes: `findings.ExternalFinding` (Task 1), `findings.Disposition` (Task 4), `triage.Verdict` (Task 7).
- Produces: `session.FindingRecord`, `session.VerdictRecord`, `(*SessionHistory).AppendFindingRecord`, `(*SessionHistory).AppendVerdictRecord`, and a bumped manifest `schema_version`.

Read `internal/session/persist.go` and `manifest.go` in full before starting. The existing record types define the JSONL envelope shape (a `type` discriminator plus a payload); the new records must follow it exactly rather than inventing a second convention.

**Why the schema bump:** `CLAUDE.md` §10 requires it, and three consumers key off it — `internal/viewer`, resume, and every CI posting script. Bump `ocr.run-manifest/v1` to `ocr.run-manifest/v2`.

- [ ] **Step 1: Write the failing test**

Add to `internal/session/persist_test.go` (adapt the helper names to whatever the existing tests use for creating a temporary session):

```go
func TestSessionHistory_FindingAndVerdictRoundTrip(t *testing.T) {
	sess := newTestSession(t) // existing helper in this file

	if err := sess.AppendFindingRecord(FindingRecord{
		Fingerprint: "fp1",
		Source:      "Phoenix Security",
		RuleID:      "go.sqli",
		Kind:        "sast",
		Path:        "a.go",
		StartLine:   10,
		EndLine:     10,
		Severity:    "high",
		Disposition: "triage",
	}); err != nil {
		t.Fatalf("AppendFindingRecord: %v", err)
	}

	if err := sess.AppendVerdictRecord(VerdictRecord{
		Fingerprint: "fp1",
		Verdict:     "dismissed",
		Rationale:   "Input is validated in middleware.Auth at line 44.",
	}); err != nil {
		t.Fatalf("AppendVerdictRecord: %v", err)
	}

	records := readSessionRecords(t, sess) // existing helper
	var sawFinding, sawVerdict bool
	for _, r := range records {
		switch r.Type {
		case "finding":
			sawFinding = true
		case "verdict":
			sawVerdict = true
		}
	}
	if !sawFinding {
		t.Error("no finding record was written")
	}
	if !sawVerdict {
		t.Error("no verdict record was written")
	}
}

func TestManifest_SchemaVersionBumped(t *testing.T) {
	if got := ManifestSchemaVersion; got != "ocr.run-manifest/v2" {
		t.Errorf("ManifestSchemaVersion = %q, want ocr.run-manifest/v2 — new record shapes require a bump", got)
	}
}
```

Add to `internal/agent/identity_test.go`:

```go
func TestIdentity_SecurityProfileChangesHash(t *testing.T) {
	base := newTestIdentityInput(t) // existing helper
	withSecurity := base
	withSecurity.SecurityProfile = "phoenix"

	if Hash(base) == Hash(withSecurity) {
		t.Error("the resume identity hash ignored the security profile — a --security rerun would reuse a non-security checkpoint and report no findings")
	}
}
```

Adapt `newTestIdentityInput` and `Hash` to the actual names in `internal/agent/identity.go`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/session/... ./internal/agent/... -run 'FindingAndVerdict|SchemaVersion|SecurityProfile' -v`
Expected: FAIL — undefined types and an unchanged hash.

- [ ] **Step 3: Add the record types**

In `internal/session/persist.go`, following the existing record convention:

```go
// FindingRecord is one external scanner finding considered by this run,
// including the ones that were dropped. A dropped finding is recorded so a
// reader can tell "we saw this and set it aside" from "we never saw it".
type FindingRecord struct {
	Fingerprint  string `json:"fingerprint"`
	Source       string `json:"source"`
	RuleID       string `json:"rule_id"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
	Severity     string `json:"severity"`
	Confidence   string `json:"confidence"`
	Reachability string `json:"reachability"`
	CVE          string `json:"cve,omitempty"`
	PURL         string `json:"purl,omitempty"`
	// Disposition is the policy decision: pass-through, triage, or drop.
	Disposition string `json:"disposition"`
}

// VerdictRecord is the triage stage's adjudication of one finding. Every
// suppression is recorded here with its rationale: a dismissed finding must
// remain auditable after the fact.
type VerdictRecord struct {
	Fingerprint string `json:"fingerprint"`
	Verdict     string `json:"verdict"`
	Rationale   string `json:"rationale,omitempty"`
	Fix         string `json:"fix,omitempty"`
}
```

Add `AppendFindingRecord` and `AppendVerdictRecord` methods mirroring the existing append methods exactly — same locking, same envelope construction, same error wrapping — with type discriminators `"finding"` and `"verdict"`.

- [ ] **Step 4: Bump the schema version**

In `internal/session/manifest.go`, change the manifest schema constant from `ocr.run-manifest/v1` to `ocr.run-manifest/v2`. Grep for every other occurrence and update it:

```bash
grep -rn "ocr.run-manifest/v1" --include=*.go --include=*.json --include=*.md --include=*.js --include=*.py .
```

Update the viewer, any CI posting script, and any documentation that names the version. Leave `ocr.resume-lineage/v1` alone — its shape did not change.

- [ ] **Step 5: Add the security profile to the resume identity**

In `internal/agent/identity.go`, add a `SecurityProfile string` field to the identity input struct and include it in the hash computation alongside the existing repo / diff-range / rule-config / provider-model components. Its value is the provider name (`""` when security is off, `"phoenix"` or `"sarif-file"` otherwise).

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/session/... ./internal/agent/... -v`
Expected: PASS.

- [ ] **Step 7: Run the full suite**

Run: `make test`
Expected: PASS. Any test asserting the literal string `ocr.run-manifest/v1` must be updated to `v2` — that is the point of the bump, not a regression.

- [ ] **Step 8: Commit**

```bash
git add internal/session internal/agent internal/viewer
git commit -m "feat(session): persist finding and verdict records, bump manifest schema to v2"
```

---

## Task 11: Wire prefetch, policy, triage, and merge into `ocr review`

**Files:**
- Modify: `cmd/opencodereview/shared_flags.go`
- Create: `cmd/opencodereview/security.go`
- Modify: `cmd/opencodereview/review_cmd.go`
- Test: `cmd/opencodereview/security_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–10.
- Produces: `func addFindingsFlags(cmd *cobra.Command, opts *securityOptions)`, `type securityOptions struct { findingsFile string; security bool; noTriage bool }`, `func runSecurityPipeline(ctx context.Context, in securityPipelineInput) (securityPipelineOutput, error)`.

The pipeline function is kept out of `review_cmd.go` deliberately: `review_cmd.go` is already large, and the security pipeline is independently testable only if it does not depend on the whole cobra command being constructed.

- [ ] **Step 1: Write the failing test**

Create `cmd/opencodereview/security_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/findings"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/triage"
)

type stubProvider struct {
	res findings.Result
	err error
}

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) Fetch(context.Context, findings.ScanRequest) (findings.Result, error) {
	return s.res, s.err
}

func mkExternal(path string, line int, sev string, kev findings.Tristate) findings.ExternalFinding {
	f := findings.ExternalFinding{
		Source: "stub", RuleID: "r", Path: path, StartLine: line, EndLine: line,
		Message: "m", Severity: sev, KEV: kev, Snippet: path,
	}
	findings.Normalize(&f)
	return f
}

func testInput(p findings.Provider, diffs []model.Diff) securityPipelineInput {
	return securityPipelineInput{
		Provider: p,
		Diffs:    diffs,
		Policy:   findings.DefaultPolicy(),
		RunTriage: func(_ context.Context, units []triage.Unit) ([]triage.Verdict, error) {
			var out []triage.Verdict
			for _, u := range units {
				for _, f := range u.Findings {
					out = append(out, triage.Verdict{
						Fingerprint: f.Fingerprint,
						Verdict:     triage.VerdictDismissed,
						Rationale:   "not applicable here",
					})
				}
			}
			return out, nil
		},
	}
}

const oneLineDiff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -5,0 +5,1 @@\n+db.Query(x)\n"

func TestRunSecurityPipeline_PassThroughBecomesComment(t *testing.T) {
	p := stubProvider{res: findings.Result{
		Findings: []findings.ExternalFinding{mkExternal("a.go", 5, "critical", findings.TriYes)},
	}}
	in := testInput(p, []model.Diff{{NewPath: "a.go", Diff: oneLineDiff}})

	got, err := runSecurityPipeline(context.Background(), in)
	if err != nil {
		t.Fatalf("runSecurityPipeline: %v", err)
	}
	if len(got.Comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(got.Comments))
	}
	if got.Comments[0].Provenance != model.ProvenanceScanner {
		t.Errorf("Provenance = %q, want %q", got.Comments[0].Provenance, model.ProvenanceScanner)
	}
}

func TestRunSecurityPipeline_DismissedFindingProducesNoComment(t *testing.T) {
	p := stubProvider{res: findings.Result{
		Findings: []findings.ExternalFinding{mkExternal("a.go", 5, "medium", findings.TriUnknown)},
	}}
	in := testInput(p, []model.Diff{{NewPath: "a.go", Diff: oneLineDiff}})

	got, err := runSecurityPipeline(context.Background(), in)
	if err != nil {
		t.Fatalf("runSecurityPipeline: %v", err)
	}
	if len(got.Comments) != 0 {
		t.Errorf("got %d comments, want 0 — a dismissed finding is suppressed", len(got.Comments))
	}
	if len(got.Verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1 — the dismissal must still be recorded", len(got.Verdicts))
	}
}

func TestRunSecurityPipeline_UncertainFindingIsReported(t *testing.T) {
	p := stubProvider{res: findings.Result{
		Findings: []findings.ExternalFinding{mkExternal("a.go", 5, "medium", findings.TriUnknown)},
	}}
	in := testInput(p, []model.Diff{{NewPath: "a.go", Diff: oneLineDiff}})
	in.RunTriage = func(_ context.Context, units []triage.Unit) ([]triage.Verdict, error) {
		return []triage.Verdict{{
			Fingerprint: units[0].Findings[0].Fingerprint,
			Verdict:     triage.VerdictUncertain,
			Rationale:   "could not trace the input",
		}}, nil
	}

	got, err := runSecurityPipeline(context.Background(), in)
	if err != nil {
		t.Fatalf("runSecurityPipeline: %v", err)
	}
	if len(got.Comments) != 1 {
		t.Fatalf("got %d comments, want 1 — an uncertain finding is surfaced, not suppressed", len(got.Comments))
	}
}

func TestRunSecurityPipeline_OutOfDiffFindingIsDropped(t *testing.T) {
	p := stubProvider{res: findings.Result{
		Findings: []findings.ExternalFinding{mkExternal("untouched.go", 5, "critical", findings.TriYes)},
	}}
	in := testInput(p, []model.Diff{{NewPath: "a.go", Diff: oneLineDiff}})

	got, err := runSecurityPipeline(context.Background(), in)
	if err != nil {
		t.Fatalf("runSecurityPipeline: %v", err)
	}
	if len(got.Comments) != 0 {
		t.Errorf("got %d comments, want 0", len(got.Comments))
	}
	if len(got.Dropped) != 1 {
		t.Errorf("got %d dropped, want 1 — a dropped finding must still be recorded", len(got.Dropped))
	}
}

func TestRunSecurityPipeline_ProviderFailureDegradesNotFails(t *testing.T) {
	p := stubProvider{err: errors.New("phoenix unreachable")}
	in := testInput(p, []model.Diff{{NewPath: "a.go", Diff: oneLineDiff}})

	got, err := runSecurityPipeline(context.Background(), in)
	if err != nil {
		t.Fatalf("a provider failure must degrade the run, not fail it: %v", err)
	}
	if !got.Degraded {
		t.Error("Degraded = false, want true — a failed scan must never render as a clean security pass")
	}
	if got.DegradedReason == "" {
		t.Error("DegradedReason is empty — the reason must reach the user")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/opencodereview/... -run TestRunSecurityPipeline -v`
Expected: FAIL — `undefined: runSecurityPipeline`.

- [ ] **Step 3: Write the pipeline**

Create `cmd/opencodereview/security.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/internal/findings"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/triage"
)

// securityOptions are the CLI flags that turn the security pipeline on.
type securityOptions struct {
	findingsFile string
	security     bool
	noTriage     bool
}

// securityPipelineInput is everything runSecurityPipeline needs. RunTriage is
// a function rather than a *triage.Stage so the pipeline is testable without
// an LLM.
type securityPipelineInput struct {
	Provider  findings.Provider
	Diffs     []model.Diff
	Policy    findings.Policy
	Request   findings.ScanRequest
	RunTriage func(ctx context.Context, units []triage.Unit) ([]triage.Verdict, error)
}

// securityPipelineOutput is the pipeline's contribution to the run.
type securityPipelineOutput struct {
	// Comments are the scanner comments to merge into the review output.
	Comments []model.LlmComment
	// Verdicts is every adjudication, including dismissals, for the session log.
	Verdicts []triage.Verdict
	// Considered is every finding the provider returned, for the session log.
	Considered []findings.ExternalFinding
	// Dropped is the subset that fell outside the diff.
	Dropped []findings.ExternalFinding
	// UpstreamVerdict is the provider's own gate decision, if any.
	UpstreamVerdict string
	// Degraded reports that the security pass did not complete.
	Degraded       bool
	DegradedReason string
}

// runSecurityPipeline fetches findings, splits them by policy, adjudicates the
// uncertain ones, and returns the comments to merge into the review.
//
// It never returns an error for a provider or triage failure: the review is
// still valid without the security pass. It marks the output Degraded instead,
// so the caller can say what did not happen rather than implying it passed.
func runSecurityPipeline(ctx context.Context, in securityPipelineInput) (securityPipelineOutput, error) {
	var out securityPipelineOutput

	res, err := in.Provider.Fetch(ctx, in.Request)
	if err != nil {
		out.Degraded = true
		out.DegradedReason = fmt.Sprintf("security scan with provider %q failed: %v", in.Provider.Name(), err)
		return out, nil
	}
	if res.Degraded {
		out.Degraded = true
		out.DegradedReason = res.DegradedReason
	}
	out.UpstreamVerdict = res.UpstreamVerdict
	out.Considered = findings.Dedup(res.Findings)

	changed := findings.BuildChangedLines(in.Diffs)
	parts := in.Policy.Partition(out.Considered, changed)
	out.Dropped = parts.Dropped

	for _, f := range parts.PassThrough {
		out.Comments = append(out.Comments, findings.ToComment(f))
	}

	if len(parts.Triage) == 0 {
		return out, nil
	}

	units := triage.Cluster(parts.Triage)
	verdicts, err := in.RunTriage(ctx, units)
	if err != nil {
		// Triage failed wholesale. Report every finding it was meant to judge
		// rather than dropping them: an unjudged finding is not a clean one.
		out.Degraded = true
		out.DegradedReason = fmt.Sprintf("triage stage failed: %v", err)
		for _, f := range parts.Triage {
			out.Comments = append(out.Comments, findings.ToComment(f))
		}
		return out, nil
	}
	out.Verdicts = verdicts

	byFingerprint := make(map[string]findings.ExternalFinding, len(parts.Triage))
	for _, f := range parts.Triage {
		byFingerprint[f.Fingerprint] = f
	}

	for _, v := range verdicts {
		f, ok := byFingerprint[v.Fingerprint]
		if !ok {
			continue
		}
		switch v.Verdict {
		case triage.VerdictDismissed:
			// Suppressed. The rationale is persisted by the caller.
			continue
		case triage.VerdictConfirmed:
			cm := findings.ToComment(f)
			cm.Provenance = model.ProvenanceScannerConfirmed
			cm.Verdict = v.Verdict
			cm.Content += "\n\n**Review:** " + v.Rationale
			cm.SuggestionCode = v.Fix
			out.Comments = append(out.Comments, cm)
		default:
			cm := findings.ToComment(f)
			cm.Verdict = v.Verdict
			cm.Content += "\n\n**Review:** could not confirm or rule this out. " + v.Rationale
			out.Comments = append(out.Comments, cm)
		}
	}

	return out, nil
}
```

- [ ] **Step 4: Add the flags**

In `cmd/opencodereview/shared_flags.go`, add:

```go
// addFindingsFlags registers the external-findings flags on cmd.
func addFindingsFlags(cmd *cobra.Command, opts *securityOptions) {
	cmd.Flags().StringVar(&opts.findingsFile, "findings", "", "path to a SARIF file of external scanner findings to review alongside the diff")
	cmd.Flags().BoolVar(&opts.security, "security", false, "run the configured security provider and review its findings (see 'ocr config set security.provider')")
	cmd.Flags().BoolVar(&opts.noTriage, "no-triage", false, "report external findings without LLM adjudication (never suppresses a finding)")
}
```

Call `addFindingsFlags(cmd, &opts.security)` from the review command's flag setup, adding a `security securityOptions` field to the review options struct.

- [ ] **Step 5: Wire it into the review command**

In `cmd/opencodereview/review_cmd.go`, after the diffs are loaded and before comments are rendered:

1. Build the provider: `--findings <path>` selects `sarifprov.New(path)`; `--security` selects the configured provider (Task 16 supplies the Phoenix one). Neither flag means no security pass at all — skip the whole block.
2. Build `findings.Policy` from `DefaultPolicy()`, setting `TriageEnabled = !opts.security.noTriage`.
3. Run the prefetch **concurrently with the per-file review**, joining before the merge. Start the pipeline in a goroutine before the review dispatch and receive its result afterwards.
4. Merge: `finalComments := findings.MergeComments(reviewComments, out.Comments, 3)`.
5. Persist: one `session.FindingRecord` per entry in `out.Considered` (with its disposition), one `session.VerdictRecord` per entry in `out.Verdicts`.
6. When `out.Degraded`, print the reason to stderr as an `[ocr] WARNING:` line and include it in the rendered summary. Never let a degraded run render as a clean security pass.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./cmd/opencodereview/... -v`
Expected: PASS.

- [ ] **Step 7: Verify end to end against a real SARIF file**

```bash
go build -o /tmp/ocr ./cmd/opencodereview
cd /tmp && git init sarif-demo && cd sarif-demo
printf 'package main\nfunc main() { db.Query(userInput) }\n' > a.go
git add a.go && git -c user.email=t@t -c user.name=t commit -qm init
printf 'package main\nfunc main() { db.Query(userInput + "x") }\n' > a.go
cat > f.sarif <<'EOF'
{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"demo"}},"results":[
{"ruleId":"sqli","level":"error","message":{"text":"SQL injection"},
"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},
"region":{"startLine":2}}}]}]}]}
EOF
/tmp/ocr review --findings f.sarif --no-triage -f json
```
Expected: the JSON output contains one comment with `"provenance": "scanner"` and `"rule_id": "sqli"`.

- [ ] **Step 8: Commit**

```bash
git add cmd/opencodereview
git commit -m "feat(review): wire external findings prefetch, policy, triage, and merge"
```

---

## Task 12: The verdict gate and SARIF provenance

**Files:**
- Create: `cmd/opencodereview/gate.go`
- Modify: `cmd/opencodereview/sarif.go`
- Modify: `cmd/opencodereview/review_cmd.go`
- Test: `cmd/opencodereview/gate_test.go`, `cmd/opencodereview/sarif_test.go` (add cases)

**Interfaces:**
- Consumes: `securityPipelineOutput` (Task 11), `model.LlmComment` (Task 6).
- Produces: `type GateVerdict string` (`GatePass`/`GateWarn`/`GateBlock`), `func ComputeGate(out securityPipelineOutput, comments []model.LlmComment, cfg GateConfig) (GateVerdict, string)`, `type GateConfig struct { BlockOnSeverities []string; Advisory bool }`.

Spec §2: `ocr` owns the verdict; Phoenix's is an input. A Phoenix `BLOCK` on a finding the agent dismissed is downgraded but always recorded.

- [ ] **Step 1: Write the failing test**

Create `cmd/opencodereview/gate_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/triage"
)

func defaultGate() GateConfig {
	return GateConfig{BlockOnSeverities: []string{"critical", "high"}}
}

func TestComputeGate_BlocksOnConfirmedHighSeverity(t *testing.T) {
	comments := []model.LlmComment{{
		Severity: "high", Provenance: model.ProvenanceScannerConfirmed, RuleID: "r",
	}}

	got, reason := ComputeGate(securityPipelineOutput{}, comments, defaultGate())

	if got != GateBlock {
		t.Errorf("gate = %q, want %q", got, GateBlock)
	}
	if reason == "" {
		t.Error("a blocking gate must explain itself")
	}
}

func TestComputeGate_DoesNotBlockOnLLMOnlyComments(t *testing.T) {
	comments := []model.LlmComment{{Severity: "critical", Provenance: model.ProvenanceLLM}}

	if got, _ := ComputeGate(securityPipelineOutput{}, comments, defaultGate()); got != GatePass {
		t.Errorf("gate = %q, want %q — an unreviewed LLM opinion does not gate a merge", got, GatePass)
	}
}

func TestComputeGate_UpstreamBlockDowngradedWhenAllDismissed(t *testing.T) {
	out := securityPipelineOutput{
		UpstreamVerdict: "BLOCK",
		Verdicts: []triage.Verdict{
			{Fingerprint: "a", Verdict: triage.VerdictDismissed, Rationale: "validated upstream"},
		},
	}

	got, reason := ComputeGate(out, nil, defaultGate())

	if got != GateWarn {
		t.Errorf("gate = %q, want %q — a dismissed upstream BLOCK is downgraded, not honoured", got, GateWarn)
	}
	if !strings.Contains(strings.ToUpper(reason), "BLOCK") {
		t.Errorf("reason = %q, want it to record that an upstream BLOCK was downgraded", reason)
	}
}

func TestComputeGate_UpstreamBlockHonouredWhenNothingWasAdjudicated(t *testing.T) {
	out := securityPipelineOutput{UpstreamVerdict: "BLOCK"}

	if got, _ := ComputeGate(out, nil, defaultGate()); got != GateBlock {
		t.Errorf("gate = %q, want %q — an un-adjudicated upstream BLOCK stands", got, GateBlock)
	}
}

func TestComputeGate_DegradedRunNeverPasses(t *testing.T) {
	out := securityPipelineOutput{Degraded: true, DegradedReason: "phoenix unreachable"}

	got, reason := ComputeGate(out, nil, defaultGate())

	if got == GatePass {
		t.Error("gate = PASS on a degraded run — a scan that did not happen must never read as clean")
	}
	if !strings.Contains(reason, "phoenix unreachable") {
		t.Errorf("reason = %q, want it to name the degradation", reason)
	}
}

func TestComputeGate_AdvisoryNeverBlocks(t *testing.T) {
	cfg := defaultGate()
	cfg.Advisory = true
	comments := []model.LlmComment{{Severity: "critical", Provenance: model.ProvenanceScannerConfirmed}}

	if got, _ := ComputeGate(securityPipelineOutput{}, comments, cfg); got == GateBlock {
		t.Error("advisory mode produced a BLOCK")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/opencodereview/... -run TestComputeGate -v`
Expected: FAIL — `undefined: ComputeGate`.

- [ ] **Step 3: Write the implementation**

Create `cmd/opencodereview/gate.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/triage"
)

// GateVerdict is ocr's merge decision for a run.
type GateVerdict string

const (
	GatePass  GateVerdict = "PASS"
	GateWarn  GateVerdict = "WARN"
	GateBlock GateVerdict = "BLOCK"
)

// GateConfig configures ComputeGate.
type GateConfig struct {
	// BlockOnSeverities are the severities that block when confirmed.
	BlockOnSeverities []string
	// Advisory caps the verdict at WARN. Nothing blocks a merge.
	Advisory bool
}

// ComputeGate decides the run's verdict.
//
// ocr owns this decision; an upstream provider's verdict is an input. An
// upstream BLOCK is honoured unless the triage stage actually adjudicated the
// findings behind it and dismissed every one — in which case it is downgraded
// to WARN and the downgrade is stated in the reason, never silently dropped.
//
// A degraded run cannot PASS. "We could not check" is not "we checked".
func ComputeGate(out securityPipelineOutput, comments []model.LlmComment, cfg GateConfig) (GateVerdict, string) {
	blocking := make(map[string]bool, len(cfg.BlockOnSeverities))
	for _, s := range cfg.BlockOnSeverities {
		blocking[s] = true
	}

	var blockers int
	for _, cm := range comments {
		if cm.Provenance != model.ProvenanceScannerConfirmed {
			continue
		}
		if blocking[cm.Severity] {
			blockers++
		}
	}

	verdict := GatePass
	reason := "No confirmed blocking security findings."

	switch {
	case blockers > 0:
		verdict, reason = GateBlock, fmt.Sprintf("%d confirmed security finding(s) at or above the blocking severity.", blockers)
	case out.UpstreamVerdict == "BLOCK":
		if allDismissed(out.Verdicts) && len(out.Verdicts) > 0 {
			verdict = GateWarn
			reason = "Upstream scanner returned BLOCK, downgraded to WARN: every finding behind it was reviewed and dismissed. See the recorded rationales."
		} else {
			verdict = GateBlock
			reason = "Upstream scanner returned BLOCK and its findings were not adjudicated."
		}
	case out.UpstreamVerdict == "WARN":
		verdict, reason = GateWarn, "Upstream scanner returned WARN."
	}

	if out.Degraded {
		if verdict == GatePass {
			verdict = GateWarn
		}
		reason = fmt.Sprintf("%s Security scan was incomplete: %s", reason, out.DegradedReason)
	}

	if cfg.Advisory && verdict == GateBlock {
		verdict = GateWarn
		reason = "Advisory mode: " + reason
	}

	return verdict, reason
}

func allDismissed(vs []triage.Verdict) bool {
	for _, v := range vs {
		if v.Verdict != triage.VerdictDismissed {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Emit provenance in SARIF**

In `cmd/opencodereview/sarif.go`, when building each result, set `ruleId` from `cm.RuleID` when it is non-empty (falling back to the current behaviour otherwise), and add a `properties` object carrying `provenance`, `source`, `verdict`, `fingerprint`, and `cve` for any comment whose `Provenance` is not `model.ProvenanceLLM`.

Add to `cmd/opencodereview/sarif_test.go`:

```go
func TestSARIF_CarriesScannerProvenance(t *testing.T) {
	doc := buildSARIF([]model.LlmComment{{ // adapt to the real builder name
		Path: "a.go", StartLine: 1, EndLine: 1, Content: "x", Severity: "high",
		Provenance: model.ProvenanceScannerConfirmed, Source: "Phoenix Security",
		RuleID: "go.sqli", Fingerprint: "fp1", Verdict: "confirmed",
	}})

	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal SARIF: %v", err)
	}
	for _, want := range []string{"go.sqli", "scanner-confirmed", "fp1", "Phoenix Security"} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("SARIF output is missing %q", want)
		}
	}
}
```

- [ ] **Step 5: Wire the gate into the exit code**

In `cmd/opencodereview/review_cmd.go`, after the merge, call `ComputeGate` and print the verdict and reason in the rendered summary for every format. Map the verdict to the process exit code: `GatePass` and `GateWarn` exit `0`; `GateBlock` exits `1`.

This matches `phoenix pr-scan`'s convention (`0` = PASS/WARN, `1` = BLOCK) so a CI pipeline swapping one for the other needs no change. Do not change the exit code for runs with no security pass at all — the existing behaviour stands.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./cmd/opencodereview/... -v && make test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/opencodereview
git commit -m "feat(review): add security gate verdict and SARIF provenance output"
```

---

## Task 13: The `ocr triage` command

**Files:**
- Create: `cmd/opencodereview/triage_cmd.go`
- Modify: `cmd/opencodereview/root.go`
- Test: `cmd/opencodereview/triage_cmd_test.go`

**Interfaces:**
- Consumes: `runSecurityPipeline` (Task 11), `ComputeGate` (Task 12), `sarifprov.New` (Task 5).
- Produces: the `ocr triage` cobra command.

Per the spec, `ocr triage` runs the security pipeline and rendering but not the per-file review loop. Same output formats, same exit codes.

- [ ] **Step 1: Write the failing test**

Create `cmd/opencodereview/triage_cmd_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"
)

func TestTriageCmd_RequiresFindings(t *testing.T) {
	cmd := newTriageCmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error when --findings is omitted")
	}
}

func TestTriageCmd_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd() // adapt to the real constructor name in root.go
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "triage" {
			found = true
		}
	}
	if !found {
		t.Error("the triage command is not registered on the root command")
	}
}

func TestTriageCmd_RejectsUnsupportedFormat(t *testing.T) {
	cmd := newTriageCmd()
	cmd.SetArgs([]string{"--findings", "x.sarif", "-f", "yaml"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("err = %v, want a format validation error", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/opencodereview/... -run TestTriageCmd -v`
Expected: FAIL — `undefined: newTriageCmd`.

- [ ] **Step 3: Write the command**

Create `cmd/opencodereview/triage_cmd.go` following the structure of `scan_cmd.go` (read it first). The command:

- registers `--findings` (required), `--repo`, `--from`, `--to`, `--commit`, `-f/--format` (`text`/`json`/`sarif`), `--background`, `--background-file`, `--no-triage`, `--concurrency`, `--timeout`;
- returns an error naming the flag when `--findings` is empty or the format is not one of the three;
- loads diffs exactly as `review` does, so findings are still scoped to the change;
- builds `sarifprov.New(findingsFile)` as the provider;
- calls `runSecurityPipeline`, then `ComputeGate`, then the same renderers `review` uses;
- writes the same session records as `review` (Task 10).

Register it in `root.go` alongside the existing subcommands.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/opencodereview/... -v`
Expected: PASS.

- [ ] **Step 5: Verify by hand**

Reuse the `/tmp/sarif-demo` repository from Task 11:

```bash
cd /tmp/sarif-demo && /tmp/ocr triage --findings f.sarif --no-triage -f text
```
Expected: the finding is printed with its rule id and a `PASS`/`WARN`/`BLOCK` line; exit code `0`.

- [ ] **Step 6: Commit**

```bash
git add cmd/opencodereview/triage_cmd.go cmd/opencodereview/root.go cmd/opencodereview/triage_cmd_test.go
git commit -m "feat(cli): add ocr triage for adjudicating an existing SARIF report"
```

---

## Task 14: The Phoenix provider — HTTP client and SAST

**Files:**
- Create: `internal/findings/providers/phoenix/client.go`
- Create: `internal/findings/providers/phoenix/sast.go`
- Test: `internal/findings/providers/phoenix/client_test.go`, `internal/findings/providers/phoenix/sast_test.go`
- Create: `internal/findings/providers/phoenix/testdata/pr_scan.sarif`

**Interfaces:**
- Consumes: `findings.IngestSARIF` (Task 2), `findings.Result`, `findings.ScanRequest` (Task 5).
- Produces: `phoenix.Config`, `phoenix.NewClient(Config) *Client`, `(*Client).PRScan(ctx, PRScanInput) (PRScanOutput, error)`, `phoenix.PRScanInput`, `phoenix.PRScanOutput`.

**Contract, taken from `agent-code-analyzer-r2/cli/.../PhoenixApiClient.kt`:**

| Step | Method and path | Body / response |
|---|---|---|
| Resolve | `POST /api/v1/external/pr-scan/resolve` | `{"repoName":…,"workspaceId":…}` → `{"repoName":…,"graphAvailable":bool,"usedFallbackBundle":bool,"warning":str|null,"bundles":[…]}` |
| Execute | `POST /api/v1/external/pr-scan/execute` | `{"repoName":…,"workspaceId":…,"baseBranch":…,"headBranch":…,"prNumber":…,"changedFiles":[…],"timeoutSeconds":…,"resolvedBundles":[…]}` → `{"jobId":…,"status":…}` |
| Poll | `GET /api/v1/external/pr-scan/{jobId}` | `{"status":…,"finalVerdict":…,"degradedReason":…,"errorMessage":…,"hybridWarning":…}` |
| SARIF | `GET /api/v1/external/pr-scan/{jobId}/sarif` | SARIF 2.1.0 document |

Non-terminal statuses: `QUEUED`, `RUNNING`, `PENDING`, `ACCEPTED`, `IN_PROGRESS`. Terminal success: `COMPLETED`, `SUCCEEDED`, `SUCCESS`. Terminal failure: `FAILED`, `TIMEOUT`, `CANCELLED`, `ERROR`.

**Auth:** `Authorization: Bearer <key>`. A key beginning `phx_live_` or `phx_dev_` must first be exchanged at `POST /api/v1/external/auth/token` (sending the raw key as the bearer) for a short-lived access token; any other key is used directly.

- [ ] **Step 1: Write the SARIF fixture**

Create `internal/findings/providers/phoenix/testdata/pr_scan.sarif`:

```json
{
  "version": "2.1.0",
  "runs": [{
    "tool": { "driver": { "name": "Phoenix Security", "rules": [
      { "id": "go.sqli", "properties": { "security-severity": "8.8", "cwe": "CWE-89" } }
    ] } },
    "results": [{
      "ruleId": "go.sqli",
      "level": "error",
      "message": { "text": "Unsanitised input reaches a SQL query." },
      "locations": [{ "physicalLocation": {
        "artifactLocation": { "uri": "internal/store/query.go" },
        "region": { "startLine": 42, "snippet": { "text": "db.Query(userInput)" } }
      } }]
    }]
  }]
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/findings/providers/phoenix/sast_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package phoenix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// prScanServer replays a full resolve → execute → poll → sarif exchange.
// pollStatuses are returned in order, one per poll.
func prScanServer(t *testing.T, pollStatuses []string, finalVerdict, degradedReason string) *httptest.Server {
	t.Helper()
	sarif, err := os.ReadFile(filepath.Join("testdata", "pr_scan.sarif"))
	if err != nil {
		t.Fatalf("read sarif fixture: %v", err)
	}
	poll := 0

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/pr-scan/resolve"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repoName":"demo","graphAvailable":true,"usedFallbackBundle":false,
				"bundles":[{"bindingId":"b1","bundleId":"u1","bundleName":"default","ordinal":0,
				"executionMode":"SEQUENTIAL","scanTier":"RULES_AI_SAST","useGraphDeclared":true,
				"useGraphEffective":true,"graphDegraded":false,"ruleSelectorJson":"{}"}]}`))
		case strings.HasSuffix(r.URL.Path, "/pr-scan/execute"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobId":"job-1","status":"ACCEPTED"}`))
		case strings.HasSuffix(r.URL.Path, "/pr-scan/job-1/sarif"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(sarif)
		case strings.HasSuffix(r.URL.Path, "/pr-scan/job-1"):
			status := pollStatuses[len(pollStatuses)-1]
			if poll < len(pollStatuses) {
				status = pollStatuses[poll]
			}
			poll++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobId":"job-1","status":"` + status +
				`","finalVerdict":"` + finalVerdict + `","degradedReason":"` + degradedReason + `"}`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testClient(url string) *Client {
	return NewClient(Config{
		BaseURL:      url,
		APIKey:       "test-key",
		PollInterval: time.Millisecond,
		PollTimeout:  2 * time.Second,
	})
}

func TestClient_PRScan_Success(t *testing.T) {
	srv := prScanServer(t, []string{"RUNNING", "RUNNING", "COMPLETED"}, "PASS", "")
	defer srv.Close()

	out, err := testClient(srv.URL).PRScan(context.Background(), PRScanInput{
		RepoName: "demo", BaseBranch: "main", HeadBranch: "feature",
	})
	if err != nil {
		t.Fatalf("PRScan: %v", err)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(out.Findings))
	}
	if out.Findings[0].RuleID != "go.sqli" {
		t.Errorf("RuleID = %q", out.Findings[0].RuleID)
	}
	if out.Findings[0].Severity != "high" {
		t.Errorf("Severity = %q, want high (security-severity 8.8)", out.Findings[0].Severity)
	}
	if out.Verdict != "PASS" {
		t.Errorf("Verdict = %q, want PASS", out.Verdict)
	}
	if out.Degraded {
		t.Error("a clean completed scan must not be degraded")
	}
}

func TestClient_PRScan_JobFailureDegrades(t *testing.T) {
	srv := prScanServer(t, []string{"FAILED"}, "", "worker crashed")
	defer srv.Close()

	out, err := testClient(srv.URL).PRScan(context.Background(), PRScanInput{RepoName: "demo", BaseBranch: "main", HeadBranch: "f"})
	if err != nil {
		t.Fatalf("a failed job must degrade, not error: %v", err)
	}
	if !out.Degraded {
		t.Error("Degraded = false, want true")
	}
	if !strings.Contains(out.DegradedReason, "worker crashed") {
		t.Errorf("DegradedReason = %q, want it to carry the server's reason", out.DegradedReason)
	}
	if len(out.Findings) != 0 {
		t.Error("a failed job must not report findings")
	}
	if out.Verdict == "PASS" {
		t.Error("a failed job must never report a PASS verdict")
	}
}

func TestClient_PRScan_DegradedReasonOnSuccessIsCarried(t *testing.T) {
	srv := prScanServer(t, []string{"COMPLETED"}, "PASS", "graph was stale; reachability not computed")
	defer srv.Close()

	out, err := testClient(srv.URL).PRScan(context.Background(), PRScanInput{RepoName: "demo", BaseBranch: "main", HeadBranch: "f"})
	if err != nil {
		t.Fatalf("PRScan: %v", err)
	}
	if !out.Degraded {
		t.Error("a completed job that reported a degradedReason must be marked degraded")
	}
}

func TestClient_PRScan_PollTimeoutDegrades(t *testing.T) {
	srv := prScanServer(t, []string{"RUNNING"}, "", "")
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, APIKey: "test-key", PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond})
	out, err := c.PRScan(context.Background(), PRScanInput{RepoName: "demo", BaseBranch: "main", HeadBranch: "f"})
	if err != nil {
		t.Fatalf("a poll timeout must degrade, not error: %v", err)
	}
	if !out.Degraded {
		t.Error("Degraded = false after a poll timeout")
	}
}

func TestClient_PRScan_HTTPErrorIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).PRScan(context.Background(), PRScanInput{RepoName: "demo", BaseBranch: "main", HeadBranch: "f"})
	if err == nil {
		t.Fatal("expected an error for a 401 — a rejected credential is a configuration problem, not a degraded scan")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/findings/providers/phoenix/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 4: Write the client**

Create `internal/findings/providers/phoenix/client.go` with:

- `type Config struct { BaseURL, APIKey, WorkspaceID string; PollInterval, PollTimeout time.Duration; HTTPClient *http.Client }`, with `NewClient` defaulting `PollInterval` to 2s, `PollTimeout` to 300s, and `HTTPClient` to `&http.Client{Timeout: 120 * time.Second}` when unset (matching `PrScanCommand`'s defaults).
- `type Client struct { cfg Config; token string; tokenMu sync.Mutex }`.
- `func (c *Client) do(ctx context.Context, method, path string, body any, accept string) ([]byte, error)` — marshals `body` when non-nil, sets `Content-Type: application/json` and the `Accept` header, attaches `Authorization: Bearer <resolved>`, and returns an error naming the status and response body for any status outside 2xx.
- `func (c *Client) bearer(ctx context.Context) (string, error)` — returns `c.cfg.APIKey` unless it begins `phx_live_` or `phx_dev_`, in which case it `POST`s to `/api/v1/external/auth/token` with the raw key as the bearer, caches the returned `access_token` under `tokenMu`, and returns it.

Add `client_test.go` covering `bearer`: a plain key is used verbatim; a `phx_live_` key triggers exactly one exchange and the result is cached across two calls.

- [ ] **Step 5: Write the SAST flow**

Create `internal/findings/providers/phoenix/sast.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package phoenix

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/findings"
)

// PRScanInput is one Phoenix pr-scan request.
type PRScanInput struct {
	RepoName     string
	BaseBranch   string
	HeadBranch   string
	PRNumber     int
	ChangedFiles []string
	TierOverride string
}

// PRScanOutput is the result of a completed (or degraded) pr-scan.
type PRScanOutput struct {
	JobID          string
	Findings       []findings.ExternalFinding
	Verdict        string
	Degraded       bool
	DegradedReason string
}

type resolvedBundle struct {
	BindingID        string `json:"bindingId"`
	BundleID         string `json:"bundleId"`
	BundleName       string `json:"bundleName"`
	Ordinal          int    `json:"ordinal"`
	ExecutionMode    string `json:"executionMode"`
	ScanTier         string `json:"scanTier"`
	UseGraphDeclared bool   `json:"useGraphDeclared"`
	UseGraphEffective bool  `json:"useGraphEffective"`
	GraphDegraded    bool   `json:"graphDegraded"`
	RuleSelectorJSON string `json:"ruleSelectorJson"`
}

type resolveResponse struct {
	RepoName           string           `json:"repoName"`
	GraphAvailable     bool             `json:"graphAvailable"`
	GraphStale         bool             `json:"graphStale"`
	UsedFallbackBundle bool             `json:"usedFallbackBundle"`
	Warning            string           `json:"warning"`
	Bundles            []resolvedBundle `json:"bundles"`
}

type executeResponse struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

type jobStatusResponse struct {
	JobID          string `json:"jobId"`
	Status         string `json:"status"`
	FinalVerdict   string `json:"finalVerdict"`
	DegradedReason string `json:"degradedReason"`
	ErrorMessage   string `json:"errorMessage"`
	HybridWarning  string `json:"hybridWarning"`
}

var nonTerminalStatuses = map[string]bool{
	"QUEUED": true, "RUNNING": true, "PENDING": true, "ACCEPTED": true, "IN_PROGRESS": true,
}

var terminalSuccessStatuses = map[string]bool{
	"COMPLETED": true, "SUCCEEDED": true, "SUCCESS": true,
}

// PRScan runs the full resolve → execute → poll → SARIF exchange.
//
// A transport or authentication failure is an error: the caller misconfigured
// something and needs to know. A scan that ran but did not finish cleanly is a
// degraded result with no findings and no PASS verdict — never an empty
// success, which would read as "we scanned and found nothing".
func (c *Client) PRScan(ctx context.Context, in PRScanInput) (PRScanOutput, error) {
	var out PRScanOutput

	var resolved resolveResponse
	body, err := c.do(ctx, "POST", "/api/v1/external/pr-scan/resolve", map[string]any{
		"repoName":    in.RepoName,
		"workspaceId": nilIfEmpty(c.cfg.WorkspaceID),
	}, "application/json")
	if err != nil {
		return out, fmt.Errorf("pr-scan resolve: %w", err)
	}
	if err := json.Unmarshal(body, &resolved); err != nil {
		return out, fmt.Errorf("pr-scan resolve response: %w", err)
	}

	bundles := resolved.Bundles
	if in.TierOverride != "" {
		bundles = make([]resolvedBundle, len(resolved.Bundles))
		copy(bundles, resolved.Bundles)
		for i := range bundles {
			bundles[i].ScanTier = in.TierOverride
		}
	}

	var accepted executeResponse
	body, err = c.do(ctx, "POST", "/api/v1/external/pr-scan/execute", map[string]any{
		"repoName":        in.RepoName,
		"workspaceId":     nilIfEmpty(c.cfg.WorkspaceID),
		"baseBranch":      in.BaseBranch,
		"headBranch":      in.HeadBranch,
		"prNumber":        nilIfZero(in.PRNumber),
		"changedFiles":    nilIfEmptySlice(in.ChangedFiles),
		"timeoutSeconds":  int64(c.cfg.PollTimeout / time.Second),
		"resolvedBundles": bundles,
	}, "application/json")
	if err != nil {
		return out, fmt.Errorf("pr-scan execute: %w", err)
	}
	if err := json.Unmarshal(body, &accepted); err != nil {
		return out, fmt.Errorf("pr-scan execute response: %w", err)
	}
	out.JobID = accepted.JobID

	status, err := c.pollJob(ctx, accepted.JobID)
	if err != nil {
		return out, err
	}

	switch {
	case nonTerminalStatuses[strings.ToUpper(status.Status)]:
		out.Degraded = true
		out.DegradedReason = fmt.Sprintf("Phoenix pr-scan job %s did not finish within %s", accepted.JobID, c.cfg.PollTimeout)
		return out, nil
	case !terminalSuccessStatuses[strings.ToUpper(status.Status)]:
		out.Degraded = true
		out.DegradedReason = strings.TrimSpace(fmt.Sprintf("Phoenix pr-scan job %s ended with status %s: %s %s",
			accepted.JobID, status.Status, status.ErrorMessage, status.DegradedReason))
		return out, nil
	}

	sarif, err := c.do(ctx, "GET", "/api/v1/external/pr-scan/"+accepted.JobID+"/sarif", nil, "application/json")
	if err != nil {
		return out, fmt.Errorf("pr-scan sarif: %w", err)
	}
	fs, err := findings.IngestSARIF(sarif, "Phoenix Security")
	if err != nil {
		return out, fmt.Errorf("pr-scan sarif parse: %w", err)
	}

	out.Findings = fs
	out.Verdict = status.FinalVerdict
	if status.DegradedReason != "" {
		out.Degraded = true
		out.DegradedReason = status.DegradedReason
	}
	if status.HybridWarning != "" {
		out.Degraded = true
		out.DegradedReason = strings.TrimSpace(out.DegradedReason + " " + status.HybridWarning)
	}
	return out, nil
}

// pollJob polls until the job leaves a non-terminal status or the poll budget
// is exhausted. The last observed status is returned in either case.
func (c *Client) pollJob(ctx context.Context, jobID string) (jobStatusResponse, error) {
	deadline := time.Now().Add(c.cfg.PollTimeout)
	var status jobStatusResponse

	for {
		body, err := c.do(ctx, "GET", "/api/v1/external/pr-scan/"+jobID, nil, "application/json")
		if err != nil {
			return status, fmt.Errorf("pr-scan poll: %w", err)
		}
		if err := json.Unmarshal(body, &status); err != nil {
			return status, fmt.Errorf("pr-scan poll response: %w", err)
		}
		if !nonTerminalStatuses[strings.ToUpper(status.Status)] {
			return status, nil
		}
		if time.Now().After(deadline) {
			return status, nil
		}

		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(c.cfg.PollInterval):
		}
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func nilIfEmptySlice(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/findings/providers/phoenix/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/findings/providers/phoenix
git commit -m "feat(phoenix): add PR scan client for Phoenix SAST findings"
```

---

## Task 15: The Phoenix provider — SCA and manifest delta

**Files:**
- Create: `internal/findings/manifest.go`
- Create: `internal/findings/providers/phoenix/sca.go`
- Create: `internal/findings/providers/phoenix/provider.go`
- Test: `internal/findings/manifest_test.go`, `internal/findings/providers/phoenix/sca_test.go`, `internal/findings/providers/phoenix/provider_test.go`

**Interfaces:**
- Consumes: Task 14's `Client`, `findings.ScanRequest`/`Result` (Task 5).
- Produces: `func IsManifestPath(path string) bool`, `func ManifestPaths(changed []string) []string`, `(*Client).SCADelta(ctx, SCADeltaInput) (SCADeltaOutput, error)`, `phoenix.New(Config) *Provider` implementing `findings.Provider`.

**Contract for the endpoint this depends on** (built by the companion Phoenix plan; this task codes against it and its fixtures):

```
POST /api/v1/external/sca/pr-delta
{"repoName":…,"workspaceId":…,"baseRef":…,"headRef":…,"manifestPaths":[…]}
→ {"components":[{"purl":…,"changeType":"added"|"bumped","manifestPath":…,"vulnerabilities":[
     {"cve":…,"severity":…,"summary":…,"reachability":"reachable"|"unreachable"|"unknown",
      "kev":"yes"|"no"|"unknown","malware":…,"exploitEvidence":…,"fixedVersion":…}]}],
   "degradedReason": ""}
```

Every three-state field may be absent. Absent means `unknown` — the decoder must not default it to the negative.

- [ ] **Step 1: Write the failing manifest test**

Create `internal/findings/manifest_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import "testing"

func TestIsManifestPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"go.mod", true},
		{"go.sum", true},
		{"package.json", true},
		{"package-lock.json", true},
		{"yarn.lock", true},
		{"pnpm-lock.yaml", true},
		{"requirements.txt", true},
		{"poetry.lock", true},
		{"Pipfile.lock", true},
		{"pom.xml", true},
		{"build.gradle", true},
		{"build.gradle.kts", true},
		{"Gemfile.lock", true},
		{"Cargo.toml", true},
		{"Cargo.lock", true},
		{"composer.lock", true},
		{"services/api/go.mod", true},
		{"internal/store/query.go", false},
		{"README.md", false},
		{"vendor/foo/go.mod", false},
		{"node_modules/x/package.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := IsManifestPath(tc.path); got != tc.want {
				t.Errorf("IsManifestPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestManifestPaths_FiltersAndDedupes(t *testing.T) {
	got := ManifestPaths([]string{"a.go", "go.mod", "go.mod", "web/package.json", "README.md"})

	if len(got) != 2 {
		t.Fatalf("got %v, want 2 entries", got)
	}
	if got[0] != "go.mod" || got[1] != "web/package.json" {
		t.Errorf("got %v, want sorted [go.mod web/package.json]", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/findings/... -run 'TestIsManifestPath|TestManifestPaths' -v`
Expected: FAIL — `undefined: IsManifestPath`.

- [ ] **Step 3: Write the manifest detection**

Create `internal/findings/manifest.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package findings

import (
	"path"
	"sort"
	"strings"
)

// manifestNames are the dependency manifests and lockfiles whose change can
// introduce a new component into the build.
var manifestNames = map[string]bool{
	"go.mod": true, "go.sum": true,
	"package.json": true, "package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"requirements.txt": true, "poetry.lock": true, "Pipfile.lock": true, "pyproject.toml": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true, "gradle.lockfile": true,
	"Gemfile": true, "Gemfile.lock": true,
	"Cargo.toml": true, "Cargo.lock": true,
	"composer.json": true, "composer.lock": true,
}

// vendoredPrefixes are directories whose manifests describe already-vendored
// dependencies rather than this project's own declared ones.
var vendoredPrefixes = []string{"vendor/", "node_modules/", "third_party/"}

// IsManifestPath reports whether path is a dependency manifest whose change
// can introduce or bump a component.
func IsManifestPath(p string) bool {
	clean := strings.TrimPrefix(path.Clean(p), "./")
	for _, prefix := range vendoredPrefixes {
		if strings.HasPrefix(clean, prefix) || strings.Contains(clean, "/"+prefix) {
			return false
		}
	}
	return manifestNames[path.Base(clean)]
}

// ManifestPaths returns the sorted, de-duplicated manifests among changed.
func ManifestPaths(changed []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, p := range changed {
		if !IsManifestPath(p) {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Write the failing SCA test**

Create `internal/findings/providers/phoenix/sca_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package phoenix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/findings"
)

func scaServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/external/sca/pr-delta" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func scaClient(url string) *Client {
	return NewClient(Config{BaseURL: url, APIKey: "k", PollInterval: time.Millisecond, PollTimeout: time.Second})
}

func TestClient_SCADelta_MapsFields(t *testing.T) {
	srv := scaServer(t, `{"components":[{"purl":"pkg:npm/lodash@4.17.20","changeType":"bumped",
		"manifestPath":"package.json","vulnerabilities":[
		{"cve":"CVE-2021-23337","severity":"high","summary":"Command injection",
		 "reachability":"reachable","kev":"yes","exploitEvidence":"yes","fixedVersion":"4.17.21"}]}]}`)
	defer srv.Close()

	out, err := scaClient(srv.URL).SCADelta(context.Background(), SCADeltaInput{
		RepoName: "demo", BaseRef: "main", HeadRef: "f", ManifestPaths: []string{"package.json"},
	})
	if err != nil {
		t.Fatalf("SCADelta: %v", err)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(out.Findings))
	}
	f := out.Findings[0]
	if f.Kind != findings.KindSCA {
		t.Errorf("Kind = %q, want sca", f.Kind)
	}
	if f.CVE != "CVE-2021-23337" || f.PURL != "pkg:npm/lodash@4.17.20" {
		t.Errorf("identity fields wrong: %+v", f)
	}
	if f.Path != "package.json" {
		t.Errorf("Path = %q, want the manifest that introduced the component", f.Path)
	}
	if f.Reachability != findings.Reachable || f.KEV != findings.TriYes {
		t.Errorf("three-state fields wrong: reachability=%q kev=%q", f.Reachability, f.KEV)
	}
}

// TestClient_SCADelta_AbsentFieldsBecomeUnknown is the fail-closed invariant.
func TestClient_SCADelta_AbsentFieldsBecomeUnknown(t *testing.T) {
	srv := scaServer(t, `{"components":[{"purl":"pkg:npm/x@1.0.0","changeType":"added",
		"manifestPath":"package.json","vulnerabilities":[{"cve":"CVE-1","severity":"critical","summary":"s"}]}]}`)
	defer srv.Close()

	out, err := scaClient(srv.URL).SCADelta(context.Background(), SCADeltaInput{RepoName: "d", ManifestPaths: []string{"package.json"}})
	if err != nil {
		t.Fatalf("SCADelta: %v", err)
	}
	f := out.Findings[0]
	if f.Reachability != findings.ReachUnknown {
		t.Errorf("Reachability = %q, want unknown — an absent field is not 'unreachable'", f.Reachability)
	}
	if f.KEV != findings.TriUnknown {
		t.Errorf("KEV = %q, want unknown — an absent field is not 'not exploited'", f.KEV)
	}
	if f.ExploitEvidence != findings.TriUnknown {
		t.Errorf("ExploitEvidence = %q, want unknown", f.ExploitEvidence)
	}
}

func TestClient_SCADelta_NoManifestsSkipsTheCall(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	out, err := scaClient(srv.URL).SCADelta(context.Background(), SCADeltaInput{RepoName: "d"})
	if err != nil {
		t.Fatalf("SCADelta: %v", err)
	}
	if called {
		t.Error("the endpoint was called with no changed manifests")
	}
	if len(out.Findings) != 0 || out.Degraded {
		t.Error("no changed manifests is a clean empty result, not a degraded one")
	}
}

func TestClient_SCADelta_DegradedReasonIsCarried(t *testing.T) {
	srv := scaServer(t, `{"components":[],"degradedReason":"reachability engine unavailable"}`)
	defer srv.Close()

	out, err := scaClient(srv.URL).SCADelta(context.Background(), SCADeltaInput{RepoName: "d", ManifestPaths: []string{"go.mod"}})
	if err != nil {
		t.Fatalf("SCADelta: %v", err)
	}
	if !out.Degraded || out.DegradedReason == "" {
		t.Error("the server's degradedReason was dropped")
	}
}
```

- [ ] **Step 5: Run the test to verify it fails**

Run: `go test ./internal/findings/providers/phoenix/... -run TestClient_SCADelta -v`
Expected: FAIL — `undefined: SCADeltaInput`.

- [ ] **Step 6: Write the SCA flow**

Create `internal/findings/providers/phoenix/sca.go`. It defines `SCADeltaInput{RepoName, BaseRef, HeadRef string; ManifestPaths []string}` and `SCADeltaOutput{Findings []findings.ExternalFinding; Degraded bool; DegradedReason string}`, returns an empty non-degraded output when `ManifestPaths` is empty, `POST`s the documented body, and maps each vulnerability to an `ExternalFinding` with `Kind: findings.KindSCA`, `Path` set to the component's `manifestPath`, `StartLine: 1`, `Message` built from the summary plus the change type and fixed version, and `Source: "Phoenix Security"`.

The three-state decode is the point of this task:

```go
// tristate maps an optional API string onto a Tristate. An absent or
// unrecognised value is "unknown" — never TriNo. A missing answer means the
// question was not asked, which is not the same as the answer being no.
func tristate(s string) findings.Tristate {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "true":
		return findings.TriYes
	case "no", "false":
		return findings.TriNo
	default:
		return findings.TriUnknown
	}
}

// reachability maps an optional API string onto a Reachability. As above, an
// absent value is "unknown", never "unreachable".
func reachability(s string) findings.Reachability {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "reachable":
		return findings.Reachable
	case "unreachable":
		return findings.Unreachable
	default:
		return findings.ReachUnknown
	}
}
```

Call `findings.Normalize` on every finding before returning.

- [ ] **Step 7: Write the provider that joins SAST and SCA**

Create `internal/findings/providers/phoenix/provider.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package phoenix implements findings.Provider against the Phoenix Security
// external API: pr-scan for SAST, sca/pr-delta for dependency findings.
package phoenix

import (
	"context"
	"strings"

	"github.com/alibaba/open-code-review/internal/findings"
)

// Provider fetches Phoenix SAST and SCA findings for one change.
type Provider struct {
	client *Client
	cfg    Config
}

// New returns a Provider using cfg.
func New(cfg Config) *Provider {
	return &Provider{client: NewClient(cfg), cfg: cfg}
}

// Name implements findings.Provider.
func (p *Provider) Name() string { return "phoenix" }

// Fetch implements findings.Provider. SAST and SCA are independent: a failure
// in one degrades the result and is reported, but does not discard the other.
func (p *Provider) Fetch(ctx context.Context, req findings.ScanRequest) (findings.Result, error) {
	var res findings.Result
	var reasons []string

	repoName := p.cfg.RepoName
	if repoName == "" {
		repoName = baseName(req.RepoDir)
	}

	sast, err := p.client.PRScan(ctx, PRScanInput{
		RepoName:     repoName,
		BaseBranch:   req.BaseRef,
		HeadBranch:   req.HeadRef,
		PRNumber:     req.PRNumber,
		ChangedFiles: req.ChangedFiles,
		TierOverride: p.cfg.TierOverride,
	})
	if err != nil {
		reasons = append(reasons, "SAST scan failed: "+err.Error())
	} else {
		res.Findings = append(res.Findings, sast.Findings...)
		res.UpstreamVerdict = sast.Verdict
		if sast.Degraded {
			reasons = append(reasons, sast.DegradedReason)
		}
	}

	manifests := req.ChangedManifests
	if len(manifests) == 0 {
		manifests = findings.ManifestPaths(req.ChangedFiles)
	}

	sca, err := p.client.SCADelta(ctx, SCADeltaInput{
		RepoName:      repoName,
		BaseRef:       req.BaseRef,
		HeadRef:       req.HeadRef,
		ManifestPaths: manifests,
	})
	if err != nil {
		reasons = append(reasons, "SCA delta failed: "+err.Error())
	} else {
		res.Findings = append(res.Findings, sca.Findings...)
		if sca.Degraded {
			reasons = append(reasons, sca.DegradedReason)
		}
	}

	if len(reasons) > 0 {
		res.Degraded = true
		res.DegradedReason = strings.Join(reasons, "; ")
		// A degraded scan cannot carry a PASS verdict: the caller would render
		// an incomplete scan as a clean one.
		if strings.EqualFold(res.UpstreamVerdict, "PASS") {
			res.UpstreamVerdict = ""
		}
	}
	return res, nil
}
```

Add `RepoName` and `TierOverride` fields to `Config`, and a small `baseName` helper using `path/filepath.Base`.

- [ ] **Step 8: Write the provider test**

Create `internal/findings/providers/phoenix/provider_test.go` with a server routing both `/pr-scan/*` and `/sca/pr-delta`, asserting: both finding sets appear in one `Result`; a SAST failure still yields the SCA findings with `Degraded` true; and a degraded result never carries `UpstreamVerdict == "PASS"`.

- [ ] **Step 9: Run the tests and coverage**

Run: `go test ./internal/findings/... -v && make coverage`
Expected: PASS, coverage at or above 90%.

- [ ] **Step 10: Commit**

```bash
git add internal/findings
git commit -m "feat(phoenix): add SCA manifest-delta client and the combined findings provider"
```

---

## Task 16: The `--security` profile — config, credentials, MCP

**Files:**
- Modify: `cmd/opencodereview/config_cmd.go`
- Modify: `cmd/opencodereview/review_cmd.go`
- Create: `cmd/opencodereview/security_config.go`
- Test: `cmd/opencodereview/security_config_test.go`

**Interfaces:**
- Consumes: `phoenix.New`, `phoenix.Config` (Tasks 14–15), `sarifprov.New` (Task 5).
- Produces: `type SecurityConfig struct`, `func resolveSecurityProvider(cfg *Config, opts securityOptions) (findings.Provider, findings.Policy, error)`.

- [ ] **Step 1: Write the failing test**

Create `cmd/opencodereview/security_config_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"
)

func TestResolveSecurityProvider_NoFlagsMeansNoProvider(t *testing.T) {
	p, _, err := resolveSecurityProvider(&Config{}, securityOptions{})
	if err != nil {
		t.Fatalf("resolveSecurityProvider: %v", err)
	}
	if p != nil {
		t.Errorf("provider = %v, want nil when neither --findings nor --security is set", p)
	}
}

func TestResolveSecurityProvider_FindingsFileWins(t *testing.T) {
	p, _, err := resolveSecurityProvider(&Config{}, securityOptions{findingsFile: "x.sarif"})
	if err != nil {
		t.Fatalf("resolveSecurityProvider: %v", err)
	}
	if p == nil || p.Name() != "sarif-file" {
		t.Fatalf("provider = %v, want the sarif-file provider", p)
	}
}

func TestResolveSecurityProvider_SecurityRequiresConfiguredProvider(t *testing.T) {
	_, _, err := resolveSecurityProvider(&Config{}, securityOptions{security: true})
	if err == nil {
		t.Fatal("expected an error when --security is set but no provider is configured")
	}
	if !strings.Contains(err.Error(), "security.provider") {
		t.Errorf("err = %v, want it to name the config key the user must set", err)
	}
}

func TestResolveSecurityProvider_PhoenixRequiresToken(t *testing.T) {
	t.Setenv("PHOENIX_API_TOKEN", "")
	cfg := &Config{Security: &SecurityConfig{Provider: "phoenix", Phoenix: &PhoenixConfig{APIURL: "https://x"}}}

	_, _, err := resolveSecurityProvider(cfg, securityOptions{security: true})
	if err == nil {
		t.Fatal("expected an error when the Phoenix token env var is unset")
	}
	if !strings.Contains(err.Error(), "PHOENIX_API_TOKEN") {
		t.Errorf("err = %v, want it to name the env var", err)
	}
}

func TestResolveSecurityProvider_Phoenix(t *testing.T) {
	t.Setenv("PHOENIX_API_TOKEN", "phx_live_abc")
	cfg := &Config{Security: &SecurityConfig{
		Provider: "phoenix",
		Phoenix:  &PhoenixConfig{APIURL: "https://phoenix.example", WorkspaceID: "ws-1"},
	}}

	p, _, err := resolveSecurityProvider(cfg, securityOptions{security: true})
	if err != nil {
		t.Fatalf("resolveSecurityProvider: %v", err)
	}
	if p == nil || p.Name() != "phoenix" {
		t.Fatalf("provider = %v, want the phoenix provider", p)
	}
}

func TestResolveSecurityProvider_NoTriageDisablesTriageInPolicy(t *testing.T) {
	_, policy, err := resolveSecurityProvider(&Config{}, securityOptions{findingsFile: "x.sarif", noTriage: true})
	if err != nil {
		t.Fatalf("resolveSecurityProvider: %v", err)
	}
	if policy.TriageEnabled {
		t.Error("TriageEnabled = true under --no-triage")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/opencodereview/... -run TestResolveSecurityProvider -v`
Expected: FAIL — `undefined: resolveSecurityProvider`.

- [ ] **Step 3: Add the config types**

In `cmd/opencodereview/config_cmd.go`, add to the `Config` struct:

```go
	Security *SecurityConfig `json:"security,omitempty"`
```

and define:

```go
// SecurityConfig configures the external security findings pass.
type SecurityConfig struct {
	// Provider selects the findings provider: "phoenix" today.
	Provider string `json:"provider,omitempty"`
	// BlockOnSeverities are the severities that block a merge when confirmed.
	// Empty means the built-in default (critical, high).
	BlockOnSeverities []string `json:"block_on_severities,omitempty"`
	// Advisory caps the gate verdict at WARN.
	Advisory bool           `json:"advisory,omitempty"`
	Phoenix  *PhoenixConfig `json:"phoenix,omitempty"`
}

// PhoenixConfig configures the Phoenix provider. The API token is deliberately
// absent: it is read from PHOENIX_API_TOKEN so it is never written to the
// config file or to a session record.
type PhoenixConfig struct {
	APIURL       string `json:"api_url,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	RepoName     string `json:"repo_name,omitempty"`
	TierOverride string `json:"tier_override,omitempty"`
}
```

Extend the `config set` / `config unset` key handling to accept `security.provider`, `security.advisory`, `security.block_on_severities`, and `security.phoenix.<field>`, following the existing `custom_providers.<name>.<field>` and `mcp_servers.<name>.<field>` handling exactly.

- [ ] **Step 4: Write the resolver**

Create `cmd/opencodereview/security_config.go`:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"

	"github.com/alibaba/open-code-review/internal/findings"
	"github.com/alibaba/open-code-review/internal/findings/providers/phoenix"
	sarifprov "github.com/alibaba/open-code-review/internal/findings/providers/sarif"
)

// phoenixTokenEnv is the only place the Phoenix credential is read from. It is
// never persisted to the config file or to a session record.
const phoenixTokenEnv = "PHOENIX_API_TOKEN"

// resolveSecurityProvider returns the provider and policy for this run, or a
// nil provider when no security pass was requested.
//
// --findings takes precedence over --security: an explicit file is an explicit
// instruction, and silently preferring a network scan over it would be a
// surprise.
func resolveSecurityProvider(cfg *Config, opts securityOptions) (findings.Provider, findings.Policy, error) {
	policy := findings.DefaultPolicy()
	policy.TriageEnabled = !opts.noTriage

	switch {
	case opts.findingsFile != "":
		return sarifprov.New(opts.findingsFile), policy, nil

	case opts.security:
		sec := cfg.Security
		if sec == nil || sec.Provider == "" {
			return nil, policy, fmt.Errorf("--security requires a configured provider: run 'ocr config set security.provider phoenix'")
		}
		if sec.Provider != "phoenix" {
			return nil, policy, fmt.Errorf("unknown security.provider %q (supported: phoenix)", sec.Provider)
		}
		if sec.Phoenix == nil || sec.Phoenix.APIURL == "" {
			return nil, policy, fmt.Errorf("security.phoenix.api_url is not set: run 'ocr config set security.phoenix.api_url <url>'")
		}
		token := os.Getenv(phoenixTokenEnv)
		if token == "" {
			return nil, policy, fmt.Errorf("%s is not set; the Phoenix API token is read from the environment, never from the config file", phoenixTokenEnv)
		}
		return phoenix.New(phoenix.Config{
			BaseURL:      sec.Phoenix.APIURL,
			APIKey:       token,
			WorkspaceID:  sec.Phoenix.WorkspaceID,
			RepoName:     sec.Phoenix.RepoName,
			TierOverride: sec.Phoenix.TierOverride,
		}), policy, nil

	default:
		return nil, policy, nil
	}
}
```

- [ ] **Step 5: Wire the gate config**

In `cmd/opencodereview/review_cmd.go`, build `GateConfig` from `cfg.Security`: `BlockOnSeverities` falling back to `[]string{"critical", "high"}` when empty, and `Advisory` from the config. Pass it to `ComputeGate` (Task 12).

- [ ] **Step 6: Document the MCP pairing**

`--security` does not auto-register MCP tools — MCP servers are user-configured and auto-registering a network server behind a flag would be surprising. Instead, when `--security` is set and no MCP server is configured, print one line to stderr:

```
[ocr] NOTE: triage has no graph tools available. For reachability analysis, configure Phoenix's MCP server:
[ocr]   ocr config set mcp_servers.phoenix.url <api-url>/api/v1/external/mcp
[ocr]   ocr config set mcp_servers.phoenix.headers.Authorization "Bearer $PHOENIX_API_TOKEN"
```

Add a test asserting this note appears when `--security` is set with no `mcp_servers` entry, and does not appear when one is configured.

- [ ] **Step 7: Run the tests**

Run: `go test ./cmd/opencodereview/... -v && make test && make coverage`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/opencodereview
git commit -m "feat(cli): add --security profile with Phoenix provider configuration"
```

---

## Task 17: Mechanical skill-tree mirroring

**Files:**
- Create: `scripts/sync-skills.sh`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Test: `scripts/sync-skills_test.sh` is not the convention here; verification is the CI check itself plus the manual run in Step 4.

**Interfaces:**
- Consumes: nothing.
- Produces: `make sync-skills` and a CI job step that fails when the generated plugin copies differ from what the script produces.

**The convention this automates:** each file under `plugins/open-code-review/skills/<name>/SKILL.md` is byte-identical to `skills/<name>/SKILL.md` except for a four-line note inserted immediately after the `# Title` heading and the blank line that follows it:

```
This Codex plugin skill intentionally mirrors the canonical skill at
`skills/<name>/SKILL.md`. Keep both files synchronized when updating
OCR agent instructions; a symlink is avoided because plugin installs may only
materialize the plugin subtree.
```

Verify this before writing the script — both existing pairs match it exactly:

```bash
diff -u skills/open-code-review/SKILL.md plugins/open-code-review/skills/open-code-review/SKILL.md
```

- [ ] **Step 1: Write the sync script**

Create `scripts/sync-skills.sh`:

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors
#
# Regenerates plugins/open-code-review/skills/ from the canonical skills/ tree.
#
# The plugin copies are not symlinks because a plugin install may materialize
# only the plugin subtree. They are the canonical file with one mirror note
# inserted after the title heading. This script is the single definition of
# that transformation.
#
# Usage:
#   scripts/sync-skills.sh          regenerate the plugin copies in place
#   scripts/sync-skills.sh --check  exit 1 if any copy is out of date
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${REPO_ROOT}/skills"
DST_DIR="${REPO_ROOT}/plugins/open-code-review/skills"

CHECK_ONLY=0
if [[ "${1:-}" == "--check" ]]; then
  CHECK_ONLY=1
fi

status=0

for src in "${SRC_DIR}"/*/SKILL.md; do
  name="$(basename "$(dirname "${src}")")"
  dst="${DST_DIR}/${name}/SKILL.md"

  generated="$(mktemp)"
  trap 'rm -f "${generated}"' EXIT

  awk -v name="${name}" '
    BEGIN { inserted = 0 }
    { print }
    # Insert the mirror note after the first level-1 heading and its blank line.
    !inserted && /^# / {
      getline blank
      print blank
      print "This Codex plugin skill intentionally mirrors the canonical skill at"
      print "`skills/" name "/SKILL.md`. Keep both files synchronized when updating"
      print "OCR agent instructions; a symlink is avoided because plugin installs may only"
      print "materialize the plugin subtree."
      print ""
      inserted = 1
    }
  ' "${src}" > "${generated}"

  if [[ "${CHECK_ONLY}" == "1" ]]; then
    if [[ ! -f "${dst}" ]] || ! diff -q "${generated}" "${dst}" >/dev/null; then
      echo "OUT OF DATE: ${dst}" >&2
      diff -u "${dst:-/dev/null}" "${generated}" >&2 || true
      status=1
    fi
  else
    mkdir -p "$(dirname "${dst}")"
    cp "${generated}" "${dst}"
    echo "synced: ${dst}"
  fi

  rm -f "${generated}"
  trap - EXIT
done

if [[ "${CHECK_ONLY}" == "1" && "${status}" == "0" ]]; then
  echo "skills are in sync"
fi
exit "${status}"
```

```bash
chmod +x scripts/sync-skills.sh
```

- [ ] **Step 2: Verify the script reproduces the existing files exactly**

Run: `scripts/sync-skills.sh --check`
Expected: `skills are in sync`, exit 0.

If it reports a difference, the awk transformation does not match the real convention. Fix the script to match the committed files — do **not** rewrite the committed files to match a wrong script. Existing behaviour is the specification here.

- [ ] **Step 3: Add the make target**

Add to `Makefile`, near the other development targets:

```makefile
sync-skills:
	@bash scripts/sync-skills.sh

skills-check:
	@bash scripts/sync-skills.sh --check
```

Add `skills-check` to the `check` target's prerequisites:

```makefile
check: license-check english-check skills-check
```

- [ ] **Step 4: Verify the round trip**

```bash
printf '\n<!-- sync probe -->\n' >> skills/open-code-review/SKILL.md
make skills-check   # expect: failure naming the plugin copy
make sync-skills
make skills-check   # expect: skills are in sync
git checkout -- skills plugins
```

- [ ] **Step 5: Add the CI step**

In `.github/workflows/ci.yml`, add to the existing lint or check job:

```yaml
      - name: Verify skill mirrors are in sync
        run: bash scripts/sync-skills.sh --check
```

- [ ] **Step 6: Commit**

```bash
git add scripts/sync-skills.sh Makefile .github/workflows/ci.yml
git commit -m "build: generate plugin skill mirrors from the canonical tree"
```

---

## Task 18: The security review skill

**Files:**
- Create: `skills/open-code-review-security-phx/SKILL.md`
- Generated: `plugins/open-code-review/skills/open-code-review-security-phx/SKILL.md` (by Task 17's script)

**Interfaces:**
- Consumes: the CLI surface from Tasks 11–16.
- Produces: a skill named `open-code-review-security-phx`.

Read `skills/open-code-review/SKILL.md` in full first and follow its structure, frontmatter shape, and tone.

- [ ] **Step 1: Write the skill**

Create `skills/open-code-review-security-phx/SKILL.md`:

```markdown
---
name: open-code-review-security-phx
description: >
  Security-focused code review that combines Phoenix Security SAST and SCA
  scanning with LLM adjudication, using the `ocr` CLI. Use when the user asks
  for a security review, a vulnerability check on a PR, a SAST or SCA scan of
  changes, or wants scanner findings triaged against the actual diff. Reports
  high-confidence findings with fixes and adjudicates uncertain ones, recording
  a rationale for every finding it sets aside.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI. `--security` additionally requires a configured
  Phoenix provider (`ocr config set security.provider phoenix`) and the
  PHOENIX_API_TOKEN environment variable. `--findings <file.sarif>` works with
  any SARIF-emitting scanner and needs no Phoenix access.
metadata:
  author: Phoenix Security
  homepage: https://github.com/alibaba/open-code-review
  version: "1.0.0"
---

# Open Code Review — Security

Runs a security review over Git changes: an external scanner supplies SAST and
SCA findings, and the review agent adjudicates them against the actual diff
rather than reporting them raw.

## What this does that a plain scan does not

A scanner reports what matched a pattern. This skill decides what matters here:

- **High-confidence findings pass through.** A known-exploited CVE or a
  confirmed taint path is reported with a suggested fix. The agent never
  overrules it.
- **Uncertain findings are adjudicated.** The agent reads the code, traces the
  value, and returns `confirmed`, `dismissed`, or `uncertain`. Every dismissal
  carries a recorded rationale.
- **Nothing is silently dropped.** A finding the agent could not resolve is
  reported as uncertain. A scan that failed is reported as incomplete, never
  as clean.

## Prerequisites

```bash
which ocr || npm install -g @alibaba-group/open-code-review
```

For Phoenix-backed scanning, once per machine:

```bash
ocr config set security.provider phoenix
ocr config set security.phoenix.api_url https://your-phoenix-instance
ocr config set security.phoenix.workspace_id <workspace-id>
export PHOENIX_API_TOKEN=<token>
```

The token is read from the environment only. Never write it into the config
file and never paste it into a commit.

For reachability analysis during adjudication, also register Phoenix's MCP
server so the agent can query the call graph:

```bash
ocr config set mcp_servers.phoenix.url https://your-phoenix-instance/api/v1/external/mcp
ocr config set mcp_servers.phoenix.headers.Authorization "Bearer ${PHOENIX_API_TOKEN}"
```

## Workflow

### Step 1: Gather business context

Analyze the change to extract concise context about what it is meant to do.
Pass it via `--background`. Adjudication depends on it: whether an input is
trusted is a question about the system, not about the line.

### Step 2: Run the security review

```bash
ocr review --security --audience agent --background "<context>" [user-args]
```

- Default with no arguments: reviews staged, unstaged, and untracked changes.
- Pass `--commit`, or `--from` and `--to`, straight through when the user
  supplies them.
- Add `--no-triage` when the user wants every finding reported without
  adjudication. This never suppresses anything — it only skips the second pass.
- Set a 10-minute timeout: the scan and the review run concurrently, but a
  cold scan can take several minutes.

When the user already has a scanner report, or has no Phoenix access:

```bash
ocr review --findings <file.sarif> --audience agent
ocr triage --findings <file.sarif> --audience agent   # adjudicate only, no review pass
```

### Step 3: Read the verdict, not just the findings

The output ends with `PASS`, `WARN`, or `BLOCK` and a reason. Report it to the
user verbatim. In particular:

- A `WARN` whose reason mentions an **incomplete scan** means the security pass
  did not finish. Say so explicitly. Do not summarize it as "no issues found".
- A `WARN` whose reason mentions a **downgraded upstream BLOCK** means the
  scanner wanted to block and the agent dismissed every finding behind it.
  Surface the rationales so the user can disagree.

### Step 4: Present and fix

Group findings by provenance:

- **Confirmed** (`provenance: scanner-confirmed`) — real, corroborated by both
  the scanner and the agent's reading. Fix these.
- **Reported** (`provenance: scanner`) — high-confidence findings that skipped
  adjudication. Fix these; do not argue them away.
- **Uncertain** — the agent could not resolve them. Present them with the
  agent's notes and let the user decide. Do not fix speculatively.

Apply fixes that carry a `suggestion_code` block and that you can verify. For
anything else, explain the finding and let the user choose.

## Rules

- Never present a dismissed finding as though it never existed. If the user
  asks what was suppressed, read the session: `ocr session comments <id>`.
- Never report a degraded run as a clean one.
- Do not re-run the scan to "get a better result". A finding that was
  dismissed with a rationale stays dismissed; a scan that failed needs the
  failure fixed, not retried blindly.
```

- [ ] **Step 2: Generate the plugin mirror**

Run: `make sync-skills`
Expected: `synced: .../plugins/open-code-review/skills/open-code-review-security-phx/SKILL.md`.

- [ ] **Step 3: Verify the check passes**

Run: `make skills-check`
Expected: `skills are in sync`.

- [ ] **Step 4: Commit**

```bash
git add skills/open-code-review-security-phx plugins/open-code-review/skills/open-code-review-security-phx
git commit -m "feat(skills): add the open-code-review-security-phx skill"
```

---

## Task 19: The `/ocr-review-security-phx` command and plugin manifests

**Files:**
- Create: `plugins/open-code-review/claude-code/commands/ocr-review-security-phx.md`
- Modify: `plugins/open-code-review/claude-code/.claude-plugin/plugin.json`
- Modify: `.claude-plugin/marketplace.json`
- Modify: `plugins/open-code-review/.codex-plugin/plugin.json`
- Modify: `plugins/open-code-review/.cursor-plugin/plugin.json`

**Interfaces:**
- Consumes: the CLI surface from Tasks 11–16.
- Produces: the `/ocr-review-security-phx` slash command.

Read `plugins/open-code-review/claude-code/commands/review.md` first; this file follows its structure exactly. `plugin.json` declares `"commands": "./commands"`, so the directory is auto-discovered and no manifest entry names individual commands.

- [ ] **Step 1: Write the command**

Create `plugins/open-code-review/claude-code/commands/ocr-review-security-phx.md`:

```markdown
---
description: Run a Phoenix-backed security review (SAST + SCA) with LLM adjudication of scanner findings.
---

Invoke OpenCodeReview (OCR) in security mode: an external scanner supplies SAST
and SCA findings for the change, and the review agent adjudicates them against
the actual diff before reporting.

## Workflow

### Step 1: Check the prerequisites

```bash
ocr config get security.provider
```

- If this is empty and the user has not passed `--findings`, tell them what to
  configure and stop. Do not guess an API URL:
  ```
  ocr config set security.provider phoenix
  ocr config set security.phoenix.api_url <url>
  ocr config set security.phoenix.workspace_id <id>
  export PHOENIX_API_TOKEN=<token>
  ```
- If `PHOENIX_API_TOKEN` is unset, say so. Never read a token from a file and
  never echo one you do find.

### Step 2: Run the security review

```bash
ocr review --security --audience agent [user-args]
```

- Default (no user arguments): reviews staged, unstaged, and untracked changes.
- Pass `--commit`/`-c`, or `--from` and `--to`, through as-is.
- If the user supplies a SARIF file, use `--findings <path>` instead of
  `--security`; this needs no Phoenix access and works with Semgrep, Trivy,
  Snyk, or any SARIF-emitting scanner.
- If the user wants every finding without adjudication, add `--no-triage`.
- Capture full stdout. Set a 10-minute timeout.
- If `ocr` is not found, install it: `npm i -g @alibaba-group/open-code-review`.

### Step 3: Report the verdict first

The run ends with `PASS`, `WARN`, or `BLOCK` and a reason. Lead with it.

- If the reason says the scan was **incomplete**, say that plainly. An
  incomplete security scan is not a passing one, and summarizing it as "no
  issues" is the failure this whole mode exists to prevent.
- If the reason says an upstream `BLOCK` was **downgraded**, show the
  dismissal rationales so the user can push back.

### Step 4: Triage the output for the user

Unlike a plain review, do not silently discard low-confidence items — they have
already been adjudicated once, and discarding them a second time hides work
that was done. Instead group them:

- **Confirmed** (`provenance: scanner-confirmed`) — corroborated by the scanner
  and the agent. Present all of them.
- **Reported** (`provenance: scanner`) — high-confidence, adjudication skipped
  by policy. Present all of them.
- **Uncertain** — present with the agent's notes and say explicitly that they
  are unresolved.

### Step 5: Fix

Apply fixes for confirmed and reported findings that carry a `suggestion_code`
block and that you can verify against the surrounding code. Leave uncertain
findings alone unless the user asks — implementing a speculative fix for a
finding nobody could confirm adds risk without removing any.
```

- [ ] **Step 2: Bump the plugin manifests**

In `plugins/open-code-review/claude-code/.claude-plugin/plugin.json`, bump `version` to `"1.1.0"` and extend `description` to mention security review.

In `.claude-plugin/marketplace.json`, bump the plugin entry's `version` to `"1.1.0"` and extend its `description` the same way.

In `plugins/open-code-review/.codex-plugin/plugin.json`, bump `version` to `"1.1.0"` and add to `interface.defaultPrompt`:

```json
      "Use Open Code Review to run a security review of my current changes.",
      "Use Open Code Review to triage this SARIF report against my diff."
```

In `plugins/open-code-review/.cursor-plugin/plugin.json`, bump `version` to `"1.1.0"` and add `"security"`, `"sast"`, and `"sca"` to `keywords`.

- [ ] **Step 3: Validate every manifest parses**

```bash
for f in plugins/open-code-review/claude-code/.claude-plugin/plugin.json \
         plugins/open-code-review/.codex-plugin/plugin.json \
         plugins/open-code-review/.cursor-plugin/plugin.json \
         .claude-plugin/marketplace.json; do
  python3 -c "import json,sys; json.load(open(sys.argv[1])); print('ok', sys.argv[1])" "$f"
done
```
Expected: four `ok` lines.

- [ ] **Step 4: Verify the command frontmatter matches the existing convention**

```bash
head -3 plugins/open-code-review/claude-code/commands/review.md
head -3 plugins/open-code-review/claude-code/commands/ocr-review-security-phx.md
```
Expected: both open with `---`, a single `description:` line, and `---`.

- [ ] **Step 5: Commit**

```bash
git add plugins/open-code-review .claude-plugin/marketplace.json
git commit -m "feat(plugins): add the ocr-review-security-phx command and bump manifests"
```

---

## Task 20: The opencode plugin and documentation

**Files:**
- Modify: `plugins/open-code-review/opencode/open-code-review.ts`
- Modify: `plugins/open-code-review/opencode/test/open-code-review.test.mjs`
- Modify: `CLAUDE.md`
- Modify: `DOC_INDEX.md`
- Create: `docs/security/EXTERNAL_FINDINGS.md`
- Modify: `docs/architecture/RUNTIME_FLOWS.md`, `docs/architecture/DATA_CONTRACTS.md`

**Interfaces:**
- Consumes: everything above.
- Produces: `security` and `findings` inputs on the opencode plugin, plus documentation.

- [ ] **Step 1: Write the failing opencode test**

Read `plugins/open-code-review/opencode/test/open-code-review.test.mjs` first and follow its existing assertion style. Add:

```javascript
test("security flag is passed through", () => {
  const args = buildArgs({ security: true })   // adapt to the real exported helper
  assert.ok(args.includes("--security"), `expected --security in ${args.join(" ")}`)
})

test("findings file is passed through", () => {
  const args = buildArgs({ findings: "scan.sarif" })
  const i = args.indexOf("--findings")
  assert.ok(i >= 0, `expected --findings in ${args.join(" ")}`)
  assert.equal(args[i + 1], "scan.sarif")
})

test("noTriage is passed through", () => {
  const args = buildArgs({ findings: "scan.sarif", noTriage: true })
  assert.ok(args.includes("--no-triage"), `expected --no-triage in ${args.join(" ")}`)
})

test("security and findings are not both emitted", () => {
  assert.throws(() => buildArgs({ security: true, findings: "scan.sarif" }),
    /security.*findings|findings.*security/i)
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd plugins/open-code-review/opencode && npm test`
Expected: FAIL — the new flags are not emitted.

- [ ] **Step 3: Extend the plugin**

In `plugins/open-code-review/opencode/open-code-review.ts`:

- add `security?: boolean`, `findings?: string`, and `noTriage?: boolean` to the input type, with doc comments matching the surrounding style;
- alongside the existing `preview`/`resume` mutual-exclusion check, reject `security` together with `findings` — they select different providers and silently preferring one would surprise the caller;
- in the argument builder, after the existing `pushValue` calls:

```typescript
  pushValue(args, "--findings", input.findings)
  if (input.security) {
    args.push("--security")
  }
  if (input.noTriage) {
    args.push("--no-triage")
  }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd plugins/open-code-review/opencode && npm test`
Expected: PASS.

- [ ] **Step 5: Write the engineering doc**

Create `docs/security/EXTERNAL_FINDINGS.md` covering, with links into the code:

- the `ExternalFinding` model and why every risk signal is three-state, naming the failure mode (absence read as a negative);
- the pipeline order from spec §4.4, and that prefetch runs concurrently with the review;
- the confidence policy table from spec §4.3;
- the gate semantics: ocr owns the verdict, an upstream `BLOCK` is downgraded only when every finding behind it was adjudicated and dismissed, and a degraded run can never `PASS`;
- the session record shapes and the `ocr.run-manifest/v2` bump;
- the Phoenix provider's endpoints and the `PHOENIX_API_TOKEN` credential path;
- a "Known gaps" section carrying spec §10 forward.

- [ ] **Step 6: Update the routers**

In `DOC_INDEX.md`, add `docs/security/EXTERNAL_FINDINGS.md` to the security domain and update that domain's document count.

In `CLAUDE.md`:
- §4 Entry Points: add a row for `ocr triage`;
- §5 Core Runtime Flow Summaries: add a **Security** bullet summarizing prefetch → policy → triage → merge → gate;
- §6 High-Risk Areas: add `internal/findings/policy.go` (the fail-closed split) and note that `internal/session` is now at manifest schema v2;
- §8 Documentation Loading Guide: add a "Security findings change" row pointing at `EXTERNAL_FINDINGS.md` → `SECURITY_MODEL.md` → `DATA_CONTRACTS.md`.

In `docs/architecture/RUNTIME_FLOWS.md`, add the security flow with a mermaid diagram matching the existing diagrams' style.

In `docs/architecture/DATA_CONTRACTS.md`, document `FindingRecord` and `VerdictRecord` and the schema-version bump.

- [ ] **Step 7: Verify every documentation link resolves**

```bash
grep -ohE '\]\(([^)#]+\.md)' CLAUDE.md DOC_INDEX.md docs/security/EXTERNAL_FINDINGS.md \
  | sed 's/](//' | sort -u | while read -r p; do
      [ -f "$p" ] || [ -f "docs/$p" ] || echo "BROKEN LINK: $p"
    done
```
Expected: no output.

- [ ] **Step 8: Run the full gate**

Run: `make check && make test && make coverage`
Expected: all pass, coverage at or above 90%.

- [ ] **Step 9: Commit**

```bash
git add plugins/open-code-review/opencode CLAUDE.md DOC_INDEX.md docs
git commit -m "docs: document the external findings pipeline and extend the opencode plugin"
```

---

## Companion plan: Phoenix (`agent-code-analyzer-r2`)

Tasks 14 and 15 code against a contract that does not exist yet. Until it ships,
`--security` returns SAST findings and a degraded SCA result naming the missing
endpoint — which is correct behaviour, not a bug, but it is not the feature.

The companion plan in `agent-code-analyzer-r2` must deliver:

1. `POST /api/v1/external/sca/pr-delta` matching the contract in Task 15.
2. Manifest-delta resolution: `sbom-worker`'s build-file resolution run against
   two trees, classifying each component as added, bumped, or unchanged.
3. Per-CVE reachability returning a three-state verdict keyed to the changed code.
4. MCP tools `sca_pr_delta` and `sca_finding_context`.
5. A recorded response fixture, committed to *this* repo under
   `internal/findings/providers/phoenix/testdata/`, so the Go tests verify
   against a real payload rather than a hand-written guess.

Item 5 is the handoff point between the two plans. Take it first if the two are
being built in parallel.

## Verification checklist

Before calling this plan done, all of the following must hold:

- [ ] `make check` passes, including the new `skills-check`.
- [ ] `make test` passes with `-race`.
- [ ] `make coverage` reports at or above 90%.
- [ ] `ocr review --findings <sarif>` reports scanner findings with provenance.
- [ ] `ocr triage --findings <sarif>` reports the same findings with no review pass.
- [ ] A provider failure produces a `WARN` naming the failure, never a `PASS`.
- [ ] A finding with no reachability data is adjudicated, never passed through and never dropped.
- [ ] Every dismissed finding has a rationale in the session JSONL.
- [ ] The manifest `schema_version` is `ocr.run-manifest/v2` everywhere it appears.
- [ ] `/ocr-review-security-phx` is discoverable and its prerequisites check runs first.
- [ ] The two new prompts have been verified manually against one real session
      (`CLAUDE.md` §10: prompt wording has no automated regression).
