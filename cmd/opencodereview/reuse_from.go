// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// reuseSource is the exported shape of a previous run's `--format json` output
// file. It carries exactly what the reuse join needs: the run manifest (coverage
// fingerprints plus repository identity) and the flat comment list, both of
// which every result file already publishes.
type reuseSource struct {
	Manifest  *session.RunManifest `json:"manifest"`
	Comments  []model.LlmComment   `json:"comments"`
	SessionID string               `json:"session_id"`
}

// loadReuseState reads a previous run's JSON output file and synthesizes a
// ResumeState for the fingerprint reuse engine (applyResume). Unlike a real
// resume, nothing here is admitted or validated as a checkpoint lineage: no
// ValidateOptions, no ValidateResume — the cross-push case those would reject
// (different refs, rule config, provider) is the normal case for reuse, and the
// trust boundary is the repository identity below plus the diff fingerprint
// itself, which embeds mode and both path spellings.
//
// Every failure is a warning plus a nil state, never an error: a missing,
// unreadable, malformed or foreign-repository source degrades to today's full
// review. The caller prints the warnings; both are nil when the state loads.
func loadReuseState(ctx context.Context, repoDir, path string) (*session.ResumeState, []string) {
	var warnings []string
	warn := func(format string, args ...any) (*session.ResumeState, []string) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
		return nil, warnings
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return warn("cannot read reuse source: %v", err)
	}
	var src reuseSource
	if err := json.Unmarshal(data, &src); err != nil {
		return warn("not a valid previous-run JSON output: %v", err)
	}
	if src.Manifest == nil {
		return warn("previous-run output has no manifest; nothing to reuse")
	}
	if src.Manifest.SchemaVersion != session.ManifestSchemaVersion {
		return warn("unsupported manifest schema version %q (want %q)",
			src.Manifest.SchemaVersion, session.ManifestSchemaVersion)
	}
	if !sameRepositoryIdentity(ctx, repoDir, src.Manifest.Repository.IdentitySHA256) {
		return warn("previous run reviewed a different repository; nothing to reuse")
	}

	// Join: comments are grouped by their exact path, and every coverage item
	// the parent settled as completed or reused contributes one checkpoint
	// keyed by its fingerprint. A fingerprint match implies the identical
	// old/new path strings, so the group for CoverageItem.Path is exactly the
	// comment set the parent settled for that item; a completed item with zero
	// comments is still reusable (there was simply nothing to say).
	commentsByPath := make(map[string][]model.LlmComment, len(src.Comments))
	for _, c := range src.Comments {
		commentsByPath[c.Path] = append(commentsByPath[c.Path], c)
	}
	items := make(map[string]session.ResumeItem)
	for _, bucket := range [][]session.CoverageItem{src.Manifest.Coverage.Completed, src.Manifest.Coverage.Reused} {
		for _, cov := range bucket {
			if cov.Fingerprint == "" {
				continue
			}
			if _, exists := items[cov.Fingerprint]; exists {
				continue
			}
			items[cov.Fingerprint] = session.ResumeItem{
				FilePath:    cov.Path,
				OldPath:     cov.OldPath,
				NewPath:     cov.Path,
				Fingerprint: cov.Fingerprint,
				Comments:    commentsByPath[cov.Path],
			}
		}
	}

	// "reuse:<run_id>" self-describes the source in session events and
	// resume.resumed_from. Finalize rejects an empty run_id, but a hand-edited
	// file may lack one; the file name keeps the id unique and meaningful.
	runID := src.Manifest.RunID
	if runID == "" {
		runID = filepath.Base(path)
	}
	return &session.ResumeState{
		SessionID: "reuse:" + runID,
		RepoDir:   repoDir,
		Items:     items,
		Manifest:  src.Manifest,
		Closed:    true,
		Model:     src.Manifest.Execution.Model,
	}, warnings
}

// sameRepositoryIdentity reports whether the identity the source run recorded
// matches the current repository. Both sides use the digest the run itself
// records (agent applyInputIdentity): sha256-hex of the canonicalized origin
// remote, with no origin remote yielding the empty string on both sides — two
// remote-less repositories therefore match, which mirrors the resume identity
// contract; the diff fingerprint still requires byte-identical content and
// paths before anything is actually reused.
func sameRepositoryIdentity(ctx context.Context, repoDir, recordedSHA string) bool {
	current := ""
	if id := diff.NewWorkspaceProvider(repoDir, nil).RemoteIdentity(ctx); id != "" {
		sum := sha256.Sum256([]byte(id))
		current = hex.EncodeToString(sum[:])
	}
	return current == recordedSHA
}
