// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ReceiptSchema identifies the receipt document version.
const ReceiptSchema = "ocr.session-receipt/v1"

// Receipt status values. `running` is the live state; a process killed hard
// (SIGKILL, supervisor timeout) leaves the last written receipt in `running`
// with pending files, which is how callers detect an unclean end. The other
// values are written by Finalize (or MarkReceiptCancelled).
const (
	ReceiptStatusRunning   = "running"
	ReceiptStatusComplete  = "complete"
	ReceiptStatusPartial   = "partial"
	ReceiptStatusFailed    = "failed"
	ReceiptStatusCancelled = "cancelled"
	ReceiptStatusSkipped   = "skipped"
)

// Receipt file states, mirroring the checkpoint record kinds.
const (
	ReceiptItemPending = "pending"
	ReceiptItemDone    = "done"
	ReceiptItemReused  = "reused"
	ReceiptItemFailed  = "failed"
)

// ReceiptGroup is the per-group rollup the issue asks callers to be able to
// read: group identity, its files, a terminal-state-shaped summary, and which
// files are still missing.
type ReceiptGroup struct {
	GroupID        string   `json:"group_id"`
	Label          string   `json:"label,omitempty"`
	Files          []string `json:"files"`
	State          string   `json:"state"`
	CompletedFiles int      `json:"completed_files"`
	MissingFiles   []string `json:"missing_files"`
	FailureCode    string   `json:"failure_code,omitempty"`
}

// ReceiptFile is one dispatched item's state.
type ReceiptFile struct {
	Path        string `json:"path"`
	State       string `json:"state"`
	GroupID     string `json:"group_id,omitempty"`
	FailureCode string `json:"failure_code,omitempty"`
}

// Receipt is the parseable, continuously maintained snapshot of one review or
// scan run, written beside the journal as <session-id>.receipt.json. It is
// deliberately not a .jsonl file so `ocr session list` enumeration is
// unaffected, and it is a documented interface (unlike the journal), so a
// caller whose run was killed can discover the session id and the per-group /
// per-file coverage without scraping anything.
type Receipt struct {
	Schema     string         `json:"schema"`
	SessionID  string         `json:"session_id"`
	RepoDir    string         `json:"repo_dir"`
	ReviewMode string         `json:"review_mode"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DiffFrom   string         `json:"diff_from,omitempty"`
	DiffTo     string         `json:"diff_to,omitempty"`
	DiffCommit string         `json:"diff_commit,omitempty"`
	Model      string         `json:"model,omitempty"`
	Groups     []ReceiptGroup `json:"groups,omitempty"`
	Files      []ReceiptFile  `json:"files"`
}

// receiptItem is the in-memory state behind the receipt.
type receiptItem struct {
	state       string
	failureCode string
}

// SetReceiptGroups installs the dispatched file set partitioned into groups
// and writes the initial receipt (every file pending). Review calls it once
// after grouping; scan appends one group per batch via AppendReceiptGroup.
// Group identity keys must match the paths later passed to
// RecordReviewItemDone/Reused/Failed.
func (sh *SessionHistory) SetReceiptGroups(groups []ReceiptGroup) {
	if sh == nil {
		return
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.receiptGroups = groups
	if sh.receiptItems == nil {
		sh.receiptItems = make(map[string]*receiptItem)
	}
	// Preserve states already recorded before installation (resume reuse
	// records land before grouping computes the groups).
	for _, g := range groups {
		for _, p := range g.Files {
			if _, ok := sh.receiptItems[p]; !ok {
				sh.receiptItems[p] = &receiptItem{state: ReceiptItemPending}
			}
		}
	}
	sh.writeReceiptLocked()
}

// AppendReceiptGroup adds one group (scan batch dispatch calls this per batch)
// and writes the receipt. Later RecordReviewItem* calls for its files flip
// their states; the group rollup recomputes on every write.
func (sh *SessionHistory) AppendReceiptGroup(group ReceiptGroup) {
	if sh == nil {
		return
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.receiptGroups = append(sh.receiptGroups, group)
	if sh.receiptItems == nil {
		sh.receiptItems = make(map[string]*receiptItem)
	}
	for _, p := range group.Files {
		if _, ok := sh.receiptItems[p]; !ok {
			sh.receiptItems[p] = &receiptItem{state: ReceiptItemPending}
		}
	}
	sh.writeReceiptLocked()
}

// MarkReceiptCancelled pins the receipt's terminal status to `cancelled`.
// Called from the agent's cancellation path; the pinned status survives the
// Finalize rewrite so a gracefully cancelled run never reads as partial.
func (sh *SessionHistory) MarkReceiptCancelled() {
	if sh == nil {
		return
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.receiptStatusOverride = ReceiptStatusCancelled
	sh.writeReceiptLocked()
}

// ReceiptSnapshot returns a copy of the current receipt state for callers
// that want the same rollup the file carries (the CLI's groups[] enrichment).
func (sh *SessionHistory) ReceiptSnapshot() Receipt {
	if sh == nil {
		return Receipt{}
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.buildReceiptLocked()
}

// recordReceiptItem flips one file's state and rewrites the receipt. Called
// from RecordReviewItemDone/Reused/Failed; receipt write failures never
// propagate — the checkpoint record is the source of truth, the receipt is a
// convenience snapshot.
func (sh *SessionHistory) recordReceiptItem(path, state, failureCode string) {
	if sh == nil || path == "" {
		return
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.receiptItems == nil {
		return // no expected set installed; nothing to reconcile
	}
	item, ok := sh.receiptItems[path]
	if !ok {
		// Resume flows may record paths outside the (re)installed group set;
		// track them so the receipt stays complete.
		item = &receiptItem{state: ReceiptItemPending}
		sh.receiptItems[path] = item
	}
	item.state = state
	item.failureCode = failureCode
	sh.writeReceiptLocked()
}

// buildReceiptLocked assembles the document from in-memory state.
func (sh *SessionHistory) buildReceiptLocked() Receipt {
	files := make([]ReceiptFile, 0, len(sh.receiptItems))
	byPath := make(map[string]*ReceiptFile, len(sh.receiptItems))
	for path, item := range sh.receiptItems {
		f := ReceiptFile{Path: path, State: item.state, FailureCode: item.failureCode}
		files = append(files, f)
		byPath[path] = &files[len(files)-1]
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	groups := make([]ReceiptGroup, 0, len(sh.receiptGroups))
	for gi := range sh.receiptGroups {
		g := sh.receiptGroups[gi]
		rg := ReceiptGroup{
			GroupID:      g.GroupID,
			Label:        g.Label,
			MissingFiles: []string{},
		}
		if rg.Label == "" {
			rg.Label = g.GroupID
		}
		rg.Files = append([]string(nil), g.Files...)
		for _, p := range rg.Files {
			if f := byPath[p]; f != nil {
				f.GroupID = g.GroupID
			}
			item := sh.receiptItems[p]
			switch {
			case item == nil || item.state == ReceiptItemPending || item.state == ReceiptItemFailed:
				rg.MissingFiles = append(rg.MissingFiles, p)
				if item != nil && item.state == ReceiptItemFailed && rg.FailureCode == "" {
					rg.FailureCode = item.failureCode
				}
			default:
				rg.CompletedFiles++
			}
		}
		switch {
		case len(rg.MissingFiles) == 0:
			rg.State = "done"
		case rg.CompletedFiles == 0:
			rg.State = "pending"
			rg.FailureCode = ""
		default:
			rg.State = "partial"
		}
		groups = append(groups, rg)
	}

	return Receipt{
		Schema:     ReceiptSchema,
		SessionID:  sh.SessionID,
		RepoDir:    sh.RepoDir,
		ReviewMode: sh.ReviewMode,
		Status:     sh.receiptStatusLocked(),
		StartedAt:  sh.StartTime,
		UpdatedAt:  time.Now(),
		DiffFrom:   sh.DiffFrom,
		DiffTo:     sh.DiffTo,
		DiffCommit: sh.DiffCommit,
		Model:      sh.Model,
		Groups:     groups,
		Files:      files,
	}
}

// receiptStatusLocked derives the current status: the explicit override
// (cancelled) wins, then Finalize-derived states, else running.
func (sh *SessionHistory) receiptStatusLocked() string {
	if sh.receiptStatusOverride != "" {
		return sh.receiptStatusOverride
	}
	if sh.EndTime.IsZero() {
		return ReceiptStatusRunning
	}
	// Terminal: classify by outcome counts.
	var done, failed, total int
	for _, item := range sh.receiptItems {
		total++
		switch item.state {
		case ReceiptItemDone, ReceiptItemReused:
			done++
		case ReceiptItemFailed:
			failed++
		}
	}
	if total == 0 {
		return ReceiptStatusSkipped
	}
	switch {
	case failed == 0 && done == total:
		return ReceiptStatusComplete
	case done == 0 && failed == total:
		return ReceiptStatusFailed
	default:
		return ReceiptStatusPartial
	}
}

// writeReceiptLocked atomically replaces <session-id>.receipt.json beside the
// journal. Every failure is swallowed: the receipt is a convenience snapshot,
// and a write problem must never fail the run that owns it.
func (sh *SessionHistory) writeReceiptLocked() {
	if sh.persist == nil || sh.SessionID == "" {
		return
	}
	if sh.receiptItems == nil {
		// No expected set was ever installed — the run never reached dispatch,
		// so there is nothing this receipt could honestly report.
		return
	}
	dir, err := SessionsDir(sh.RepoDir)
	if err != nil {
		return
	}
	receipt := sh.buildReceiptLocked()
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return
	}
	final := filepath.Join(dir, sh.SessionID+".receipt.json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, final)
}
