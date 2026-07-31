package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ManifestSchemaVersion identifies the versioned, machine-readable coverage
// contract emitted for a review (or scan) run. Consumers should gate on this
// value; unknown future versions must be ignored rather than misread.
const ManifestSchemaVersion = "ocr.run-manifest/v1"

// OperationReview is the manifest operation for a diff review run. It is the
// only operation wired in v1 (scan stays legacy with no manifest).
const OperationReview = "review"

// Input modes describe how the run's input was selected. They are stored in
// ManifestInput.Mode (mandatory) and decide how the remaining input fields are
// interpreted. They also feed ItemID derivation so that the same logical file
// keeps a stable item_id across a resume chain.
const (
	InputModeRange     = "range"
	InputModeCommit    = "commit"
	InputModeWorkspace = "workspace"
)

// validInputMode reports whether m is one of the three mandatory input modes.
func validInputMode(m string) bool {
	switch m {
	case InputModeRange, InputModeCommit, InputModeWorkspace:
		return true
	default:
		return false
	}
}

// FailureClass is the typed classification attached to every failed coverage
// item. The set is fixed so downstream consumers can switch on it reliably.
// FailureUnknown is the mandatory catch-all: it only applies to an item that is
// known to have failed but cannot be reliably mapped to a more specific class.
type FailureClass string

const (
	FailureProvider      FailureClass = "provider"
	FailureTimeout       FailureClass = "timeout"
	FailureCancelled     FailureClass = "cancelled"
	FailureConfiguration FailureClass = "configuration"
	FailureInput         FailureClass = "input"
	FailureBudget        FailureClass = "budget"
	FailurePanic         FailureClass = "panic"
	FailureUnknown       FailureClass = "unknown"
)

// valid reports whether c is one of the fixed item failure classes.
func (c FailureClass) valid() bool {
	switch c {
	case FailureProvider, FailureTimeout, FailureCancelled, FailureConfiguration,
		FailureInput, FailureBudget, FailurePanic, FailureUnknown:
		return true
	default:
		return false
	}
}

// RunFailureClass classifies why the whole run stopped. It is a distinct
// enumeration from the per-item FailureClass: a run never fails with "provider"
// or "panic" (those are always attributed to a single item), and it adds
// "internal" for scheduler/invariant failures. Its values are:
//
//	input         — diff resolution / input freezing failed
//	configuration — an in-run configuration failure after the builder existed
//	timeout       — a global deadline elapsed
//	cancelled     — the user actively cancelled the run
//	budget        — an aggregate token/round budget pool was exhausted
//	internal      — scheduler failure or an internal invariant was violated
//	unknown       — confirmed run-level failure that cannot be classified
type RunFailureClass string

const (
	RunFailureInput         RunFailureClass = "input"
	RunFailureConfiguration RunFailureClass = "configuration"
	RunFailureTimeout       RunFailureClass = "timeout"
	RunFailureCancelled     RunFailureClass = "cancelled"
	RunFailureBudget        RunFailureClass = "budget"
	RunFailureInternal      RunFailureClass = "internal"
	RunFailureUnknown       RunFailureClass = "unknown"
)

// valid reports whether c is one of the fixed run failure classes.
func (c RunFailureClass) valid() bool {
	switch c {
	case RunFailureInput, RunFailureConfiguration, RunFailureTimeout,
		RunFailureCancelled, RunFailureBudget, RunFailureInternal, RunFailureUnknown:
		return true
	default:
		return false
	}
}

// itemFailureForRunClass maps a run-level stop cause onto the item-level
// classification used when Finalize sweeps the still-undecided selected items.
// A global timeout/cancel/budget stop colors its pending items with the exact
// matching class; input/configuration keep their own name; internal and unknown
// have no item-level equivalent (the item enum has no "internal"), so their
// swept items fall back to FailureUnknown while the run_failure retains the
// precise cause.
func itemFailureForRunClass(c RunFailureClass) FailureClass {
	switch c {
	case RunFailureInput:
		return FailureInput
	case RunFailureConfiguration:
		return FailureConfiguration
	case RunFailureTimeout:
		return FailureTimeout
	case RunFailureCancelled:
		return FailureCancelled
	case RunFailureBudget:
		return FailureBudget
	default: // internal, unknown
		return FailureUnknown
	}
}

// RunFailure records why the entire run stopped, independently of any single
// item. Its presence forces the terminal state to failed while leaving the
// per-item outcomes that did complete intact.
type RunFailure struct {
	Classification RunFailureClass `json:"classification"`
	Reason         string          `json:"reason,omitempty"`
}

// pendingFailureCause is the builder-internal classification Finalize applies to
// items that are still undecided when the run closes, used when the run stopped
// covering them deliberately rather than because the run itself failed.
//
// It is the contract for a *controlled coverage truncation*: the remaining items
// genuinely did not get reviewed, so they are failed(<class>) and count against
// coverage, but the run did not fail and its terminal state stays coverage-derived
// (partial while anything completed, failed only when nothing did). It is
// deliberately NOT part of RunManifest: schema v1 is frozen, and the fact is
// already observable through coverage.failed[].classification.
type pendingFailureCause struct {
	Classification FailureClass
	Reason         string
}

// TerminalState is the single, coverage-derived outcome of a run. It is the
// authoritative replacement for the warning-derived "completed_with_errors"
// status: it is computed only from the coverage sets plus run_failure, never
// from comment count or warnings.
type TerminalState string

const (
	StateComplete TerminalState = "complete"
	StatePartial  TerminalState = "partial"
	StateFailed   TerminalState = "failed"
	StateSkipped  TerminalState = "skipped"
)

// CoverageItem is one file's entry in a coverage set. ItemID is the
// content-independent logical identity used to sort and cross-reference
// entries; it is minted via ItemID() from operation, input mode and the
// normalized old/new path, so a diff-content change alone does not change it.
// Fingerprint retains the raw diff fingerprint (which does include diff content)
// so the item can be cross-referenced against the resume checkpoint index (which
// is keyed by the raw fingerprint). Classification and Reason are only populated
// for failed/waived items; Reason is passed through sanitizeReason() by the
// builder as a redaction floor (callers should still redact context-aware).
type CoverageItem struct {
	ItemID         string       `json:"item_id"`
	Path           string       `json:"path"`
	OldPath        string       `json:"old_path,omitempty"`
	Fingerprint    string       `json:"fingerprint,omitempty"`
	Classification FailureClass `json:"classification,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

// ItemID is the single, canonical derivation of a manifest item_id. It is
// content-independent: it is derived from the operation, the input mode and the
// normalized old/new path, so the same logical file keeps a stable item_id
// across a resume chain even when its diff content (and therefore its
// fingerprint) changes. The raw diff content lives only in
// CoverageItem.Fingerprint, which is used for checkpoint matching. Every call
// site — RegisterSelected and each Mark* — MUST key on the same
// ItemID(operation, mode, oldPath, newPath) so a mismatched key never silently
// no-ops a transition.
func ItemID(operation, mode, oldPath, newPath string) string {
	// NUL-join the identity fields. Paths never contain NUL and operation/mode
	// are controlled enums, so this join is unambiguous.
	key := strings.Join([]string{
		operation,
		mode,
		normalizePath(oldPath),
		normalizePath(newPath),
	}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// normalizePath canonicalizes a path for identity derivation: it unifies
// separators to forward slashes and cleans redundant "." / ".." / duplicate
// separators, so cosmetically different spellings of the same path yield the
// same item_id. An empty path stays empty.
func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

// Coverage holds the five disjoint file sets. selected is the denominator and
// equals the disjoint union of completed, reused, failed and waived. Arrays are
// sorted by ItemID and are always non-nil so JSON renders "[]" rather than null.
type Coverage struct {
	Selected  []CoverageItem `json:"selected"`
	Completed []CoverageItem `json:"completed"`
	Reused    []CoverageItem `json:"reused"`
	Failed    []CoverageItem `json:"failed"`
	Waived    []CoverageItem `json:"waived"`
}

// ManifestRepository is the redacted repository identity for a run.
type ManifestRepository struct {
	IdentitySHA256 string `json:"identity_sha256,omitempty"`
}

// ManifestInput is the frozen, resolved input identity of a run. Mode is
// mandatory and decides how the remaining fields are read. Resolved values are
// actual commit SHAs captured before execution, not the mutable refs the user
// typed. A child (resume) run always recomputes its own input rather than
// copying the parent's.
type ManifestInput struct {
	Mode                 string `json:"mode"`
	RequestedFrom        string `json:"requested_from,omitempty"`
	RequestedHead        string `json:"requested_head,omitempty"`
	ResolvedBase         string `json:"resolved_base,omitempty"`
	ResolvedHead         string `json:"resolved_head,omitempty"`
	ExactRange           string `json:"exact_range,omitempty"`
	SourceArtifactSHA256 string `json:"source_artifact_sha256,omitempty"`
}

// ManifestExecution records how the run was executed. Only non-secret values
// and hashes are stored here — never tokens, endpoints or raw config.
type ManifestExecution struct {
	OCRVersion            string `json:"ocr_version,omitempty"`
	Provider              string `json:"provider,omitempty"`
	Model                 string `json:"model,omitempty"`
	ConfiguredConcurrency int    `json:"configured_concurrency,omitempty"`
	RuleConfigSHA256      string `json:"rule_config_sha256,omitempty"`
	RuntimeConfigSHA256   string `json:"runtime_config_sha256,omitempty"`
}

// RunManifest is the immutable, versioned coverage snapshot of a single run.
// It is produced once, at Finalize, and is the same object serialized to both
// the CLI JSON and the persisted session, so the two outlets can never compute
// coverage differently.
type RunManifest struct {
	SchemaVersion string             `json:"schema_version"`
	RunID         string             `json:"run_id"`
	ParentRunID   string             `json:"parent_run_id,omitempty"`
	Operation     string             `json:"operation"`
	TerminalState TerminalState      `json:"terminal_state"`
	Repository    ManifestRepository `json:"repository"`
	Input         ManifestInput      `json:"input"`
	Execution     ManifestExecution  `json:"execution"`
	Coverage      Coverage           `json:"coverage"`
	RunFailure    *RunFailure        `json:"run_failure,omitempty"`
	ElapsedMS     int64              `json:"elapsed_ms"`
}

// itemState is the internal per-item lifecycle. Every registered item starts as
// selected and moves to exactly one terminal state.
type itemState string

const (
	stateSelected  itemState = "selected"
	stateCompleted itemState = "completed"
	stateReused    itemState = "reused"
	stateFailed    itemState = "failed"
	stateWaived    itemState = "waived"
)

type builderItem struct {
	item  CoverageItem
	state itemState
}

// Errors returned by the builder's mutating entry points. Callers must handle
// them rather than assume success: a silent no-op would let a mis-keyed or
// late transition drop an outcome and be discovered only at Finalize (or never).
var (
	errNilBuilder = errors.New("manifest: operation on nil builder")
	errFrozen     = errors.New("manifest: builder already finalized")
	errSealed     = errors.New("manifest: selected set already sealed")
	errEmptyID    = errors.New("manifest: empty item_id")
)

// ManifestBuilder accumulates coverage for one run and freezes it into a
// RunManifest. It is safe for concurrent use: every registration, transition
// and freeze is serialized by a single mutex. The lifecycle has two explicit
// boundaries beyond the mutex:
//
//   - sealed: after SealSelected the selected set is closed; RegisterSelected
//     then returns an error. The pre-dispatch pass registers every planned item
//     and seals immediately, before resume reuse or concurrent dispatch, so the
//     coverage denominator can never be widened mid-run.
//   - frozen: after Finalize the whole builder is immutable; every mutating call
//     returns an error and Finalize returns the same frozen manifest.
//
// A written terminal state is never overwritten by a conflicting later
// transition (that returns an error); the same outcome applied twice is
// idempotent.
type ManifestBuilder struct {
	mu    sync.Mutex
	items map[string]*builderItem

	runID       string
	parentRunID string
	operation   string
	repository  ManifestRepository
	input       ManifestInput
	execution   ManifestExecution

	runFailure     *RunFailure
	pendingFailure *pendingFailureCause

	sealed bool
	frozen bool
	result *RunManifest
}

// NewManifestBuilder creates a builder for a run identified by runID (the
// canonical session ID) and operation ("review" or "scan").
func NewManifestBuilder(runID, operation string) *ManifestBuilder {
	return &ManifestBuilder{
		runID:     runID,
		operation: operation,
		items:     make(map[string]*builderItem),
	}
}

// SetParentRunID links this run to the session it resumed from. It only records
// the direct parent; the parent's input is never copied into this run.
func (b *ManifestBuilder) SetParentRunID(parent string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.frozen {
		b.parentRunID = parent
	}
}

// SetRepository sets the redacted repository identity.
func (b *ManifestBuilder) SetRepository(repo ManifestRepository) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.frozen {
		b.repository = repo
	}
}

// SetInput sets the frozen, resolved input identity (including the mandatory
// Mode). Finalize rejects an invalid or missing Mode.
func (b *ManifestBuilder) SetInput(in ManifestInput) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.frozen {
		b.input = in
	}
}

// SetExecution sets the non-secret execution provenance.
func (b *ManifestBuilder) SetExecution(ex ManifestExecution) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.frozen {
		b.execution = ex
	}
}

// SetRunFailure records why the whole run stopped. It rejects an invalid class
// (so an unclassifiable stop is never silently downgraded) and a change of an
// already-set class (the first recorded stop cause wins); re-recording the same
// class is idempotent. reason is passed through sanitizeReason as a redaction
// floor. Callers must record the cause at the trigger site — Finalize never
// infers it from context state.
func (b *ManifestBuilder) SetRunFailure(class RunFailureClass, reason string) error {
	if b == nil {
		return errNilBuilder
	}
	if !class.valid() {
		return fmt.Errorf("manifest: invalid run_failure class %q", class)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen {
		return errFrozen
	}
	if b.runFailure != nil {
		if b.runFailure.Classification == class {
			return nil
		}
		return fmt.Errorf("manifest: run_failure already set to %q, cannot change to %q",
			b.runFailure.Classification, class)
	}
	b.runFailure = &RunFailure{Classification: class, Reason: sanitizeReason(reason)}
	return nil
}

// SetPendingFailureCause records the classification Finalize must use when it
// sweeps items that never received a Mark*. It records no run_failure and
// transitions no item now, so it does not escalate the run's terminal state.
//
// This is the entry point for a controlled coverage truncation — the caller
// stopped covering the remaining selected items on purpose (the aggregate token
// budget is the only wired producer today). Those items must be attributed to
// that specific cause instead of degrading to "unknown", while the terminal state
// stays coverage-derived: partial while anything completed or was reused, failed
// only when the truncation left nothing covered.
//
// Setting a cause instead of walking the undispatched slice avoids three hazards
// a hand-written loop has: it cannot race a subtask still in flight (Finalize runs
// after the wait group drains), it cannot overwrite an already-recorded
// completed/reused/failed outcome (the sweep only touches still-selected items),
// and it cannot mis-mark an item that was never registered in the first place
// (deleted/filtered files are not in the selected set).
//
// A run_failure, if also set, wins in the sweep and forces the run to failed;
// this cause is then an unused fallback. It rejects an invalid class and a change
// of an already-set class (first cause wins); re-recording the same class is
// idempotent. reason is passed through sanitizeReason as a redaction floor.
func (b *ManifestBuilder) SetPendingFailureCause(class FailureClass, reason string) error {
	if b == nil {
		return errNilBuilder
	}
	if !class.valid() {
		return fmt.Errorf("manifest: invalid pending failure class %q", class)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen {
		return errFrozen
	}
	if b.pendingFailure != nil {
		if b.pendingFailure.Classification == class {
			return nil
		}
		return fmt.Errorf("manifest: pending failure cause already set to %q, cannot change to %q",
			b.pendingFailure.Classification, class)
	}
	b.pendingFailure = &pendingFailureCause{Classification: class, Reason: sanitizeReason(reason)}
	return nil
}

// RegisterSelected records an item in the selected set (the coverage
// denominator). It must be called once per item during the pre-dispatch pass,
// after filtering and before SealSelected. Re-registering an already-known
// item_id keeps the first registration (idempotent). Registration after seal or
// after freeze returns an error, so the denominator can never be widened once
// dispatch has begun.
//
// The caller MUST register only the post-deletion, post-filter dispatchable set:
// files excluded before planning (deleted files, path/extension-filtered files)
// must NOT be registered, because every registered item that never receives a
// Mark* is swept to failed at Finalize — registering a non-dispatchable file
// would fabricate a bogus failure and misreport the run as partial.
func (b *ManifestBuilder) RegisterSelected(item CoverageItem) error {
	if b == nil {
		return errNilBuilder
	}
	if item.ItemID == "" {
		return errEmptyID
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen {
		return errFrozen
	}
	if b.sealed {
		return errSealed
	}
	if b.items == nil {
		b.items = make(map[string]*builderItem)
	}
	if _, exists := b.items[item.ItemID]; exists {
		return nil // first registration wins; idempotent
	}
	sel := CoverageItem{
		ItemID:      item.ItemID,
		Path:        item.Path,
		OldPath:     item.OldPath,
		Fingerprint: item.Fingerprint,
	}
	b.items[item.ItemID] = &builderItem{item: sel, state: stateSelected}
	return nil
}

// SealSelected closes the selected set. After it returns, RegisterSelected fails
// and the coverage denominator is fixed; per-item Mark* calls continue to work
// against the sealed set. It is idempotent and separate from freeze: sealing
// happens once the pre-dispatch pass has registered every planned item, before
// resume reuse and concurrent dispatch.
func (b *ManifestBuilder) SealSelected() error {
	if b == nil {
		return errNilBuilder
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen {
		return errFrozen
	}
	b.sealed = true
	return nil
}

// Sealed reports whether the selected set has been sealed.
func (b *ManifestBuilder) Sealed() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sealed
}

// transition moves a selected item to a terminal state through the single
// coverage entry point. Unknown item_id, an invalid failed classification, an
// empty waive reason, a conflicting transition (a different terminal state than
// already recorded) and any call after freeze all return an error rather than
// silently no-op'ing. Re-applying the same terminal state is idempotent.
func (b *ManifestBuilder) transition(itemID string, to itemState, class FailureClass, reason string) error {
	if b == nil {
		return errNilBuilder
	}
	if itemID == "" {
		return errEmptyID
	}
	// Validate the payload before taking the lock so an invalid class/reason is
	// rejected regardless of the item's current state.
	if to == stateFailed && !class.valid() {
		return fmt.Errorf("manifest: invalid failure class %q for item %s", class, itemID)
	}
	sanitized := ""
	if to == stateFailed || to == stateWaived {
		sanitized = sanitizeReason(reason)
		if to == stateWaived && strings.TrimSpace(sanitized) == "" {
			return fmt.Errorf("manifest: waived item %s requires a non-empty reason", itemID)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen {
		return errFrozen
	}
	bi, ok := b.items[itemID]
	if !ok {
		return fmt.Errorf("manifest: transition on unknown item %s", itemID)
	}
	if bi.state != stateSelected {
		if bi.state == to {
			// Re-applying the same terminal state is idempotent, except that a
			// failed item re-marked with a different classification is a
			// conflicting transition: reason is free text and does not
			// participate, but the machine-readable classification must match so
			// a mis-keyed double-mark surfaces instead of silently keeping the
			// first class.
			if to == stateFailed && bi.item.Classification != class {
				return fmt.Errorf("manifest: item %s already failed as %s, cannot re-mark as %s",
					itemID, bi.item.Classification, class)
			}
			return nil // idempotent: same outcome re-applied
		}
		return fmt.Errorf("manifest: item %s already %s, cannot transition to %s",
			itemID, bi.state, to)
	}
	bi.state = to
	if to == stateFailed {
		bi.item.Classification = class
		bi.item.Reason = sanitized
	}
	if to == stateWaived {
		bi.item.Reason = sanitized
	}
	return nil
}

// maxReasonLen bounds the redacted failure/waive summary stored in the manifest,
// counted in runes so multibyte text is never cut mid-character.
const maxReasonLen = 500

var (
	// urlUserinfoRe matches credentials embedded in a URL (scheme://user:pass@host).
	urlUserinfoRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/\s:@]+(?::[^/\s@]+)?@`)
	// bearerRe matches "Bearer <token>" / "Basic <token>" auth values.
	bearerRe = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[A-Za-z0-9._~+/=\-]+`)
	// secretAssignmentRe matches `key: value` / `key=value` where the key names a
	// credential-like field, so the value can be redacted while the key is kept.
	// The value alternation handles quoted values (with spaces) and bare tokens,
	// so a quoted secret is not partially leaked.
	secretAssignmentRe = regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|secret|password|passwd|token)\b(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s"']+)`)
)

// stripUnsafeChars removes control characters and Unicode line separators that
// would let escape sequences (ANSI/ANSI-CSI) or line breaks survive into a
// terminal renderer, and collapses horizontal/vertical whitespace to a single
// space so a reason stays one line. C0 controls (except tab/newlines, mapped to
// space), DEL, and C1 controls are dropped.
func stripUnsafeChars(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t', r == '\n', r == '\r', r == '\v', r == '\f',
			r == 0x85, r == 0x2028, r == 0x2029:
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1 // drop remaining C0 / DEL / C1 controls
		default:
			return r
		}
	}, s)
}

// sanitizeReason is the single, best-effort redaction+truncation floor applied
// to every reason the builder stores, so no caller path can write an unredacted
// summary into the manifest (which is serialized to both CLI JSON and the
// persisted session). It is a floor, not a substitute for caller-side
// redaction: callers should still pass an already-summarized reason and omit
// anything they cannot confirm is safe. It strips URL credentials, Bearer/Basic
// tokens and credential-like key=value pairs, removes control/escape characters,
// collapses to a single line, coerces to valid UTF-8, and caps length.
//
// Absolute local paths, cookies and raw request/response bodies are not stripped
// here. Callers must replace those values with a safe summary before storing a
// reason.
func sanitizeReason(s string) string {
	if s == "" {
		return ""
	}
	// Order matters. Coerce to valid UTF-8 and strip control characters BEFORE
	// the redaction regexes: an embedded control byte inside a token would
	// otherwise truncate a regex match (e.g. "Bearer AAA\x00BBB" matches only
	// "Bearer AAA"), and the later strip would then drop the byte and splice the
	// surviving "BBB" back in, leaking part of the secret. Removing the byte
	// first lets the regex see and redact the whole token.
	//
	// An invalid UTF-8 byte is replaced with the replacement rune "�" rather than
	// removed, so it can still truncate a token match. This sanitizer is therefore
	// only a defensive floor; callers still own context-aware redaction.
	s = strings.ToValidUTF8(s, "�")
	s = stripUnsafeChars(s)
	// Within the redaction pass, strip "Bearer <tok>" before the assignment rule,
	// so a token following "Authorization:" is removed rather than left behind.
	s = urlUserinfoRe.ReplaceAllString(s, "${1}[REDACTED]@")
	s = bearerRe.ReplaceAllString(s, "[REDACTED]")
	s = secretAssignmentRe.ReplaceAllString(s, "${1}${2}[REDACTED]")
	if utf8.RuneCountInString(s) > maxReasonLen {
		s = string([]rune(s)[:maxReasonLen]) + "…"
	}
	return s
}

// MarkCompleted records that the item's subtask completed. Whether it produced
// comments does not affect completion.
func (b *ManifestBuilder) MarkCompleted(itemID string) error {
	return b.transition(itemID, stateCompleted, "", "")
}

// MarkReused records that the item was reused from a parent session checkpoint.
func (b *ManifestBuilder) MarkReused(itemID string) error {
	return b.transition(itemID, stateReused, "", "")
}

// MarkFailed records that the item failed. class must be a valid, specific
// failure class — an empty or unknown-string class is rejected rather than
// downgraded, so an unclassified failure is never fabricated. reason must
// already be a redacted summary; the builder redacts again as a floor.
func (b *ManifestBuilder) MarkFailed(itemID string, class FailureClass, reason string) error {
	return b.transition(itemID, stateFailed, class, reason)
}

// MarkWaived records that the user explicitly waived this selected item, which
// must still be in the pre-terminal (selected) state. reason is required and
// must be non-empty. In the resume flow an item that failed in the parent
// session is re-registered as selected in this child run and waived here; the
// parent manifest is never modified.
func (b *ManifestBuilder) MarkWaived(itemID, reason string) error {
	return b.transition(itemID, stateWaived, "", reason)
}

// Frozen reports whether Finalize has already run.
func (b *ManifestBuilder) Frozen() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.frozen
}

// Finalize performs the hard-validated close of the run and returns the
// immutable manifest. In order it: sweeps any item still selected into failed
// (colored by the run_failure class, else the pending failure cause, else
// unknown — see the sweep comment for the precedence); builds the five
// coverage sets; validates the contract invariants (non-empty run_id/operation,
// a valid input.mode, the set partition, a valid class on every failed item, a
// non-empty reason on every waived item, and a valid run_failure class if set);
// attaches run_failure; computes the terminal state (run_failure forces failed
// while keeping recorded outcomes) and freezes the builder.
//
// A validation failure returns a non-nil error and does NOT freeze the builder,
// so the caller sees a construction failure rather than an invalid v1 manifest.
// On success Finalize is idempotent: later calls return the same frozen manifest
// (as an independent copy) and ignore elapsed. Finalize never infers a stop
// cause from context state — callers record run_failure at the trigger site.
func (b *ManifestBuilder) Finalize(elapsed time.Duration) (RunManifest, error) {
	if b == nil {
		return emptyManifest(), errNilBuilder
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen && b.result != nil {
		return b.result.cloned(), nil
	}

	// Backstop: no selected item may be left without an outcome. This covers
	// goroutines that exited early, cancellation before dispatch, a controlled
	// coverage truncation, or any path the caller forgot. Highest priority first:
	//
	//  1. run_failure set — undecided items take its matching item class, and
	//     computeTerminal forces the whole run to failed.
	//  2. no run_failure but a pending failure cause set — undecided items take
	//     that class and the terminal state stays coverage-derived, so a
	//     deliberate truncation reports partial rather than a run failure.
	//  3. neither — fall back to unknown.
	//
	// It only runs when the process can still execute Finalize; a hard kill falls
	// back to the per-item checkpoints.
	sweepClass := FailureUnknown
	sweepReason := "no terminal outcome recorded"
	switch {
	case b.runFailure != nil:
		sweepClass = itemFailureForRunClass(b.runFailure.Classification)
		if r := b.runFailure.Reason; r != "" {
			sweepReason = r
		}
	case b.pendingFailure != nil:
		sweepClass = b.pendingFailure.Classification
		if r := b.pendingFailure.Reason; r != "" {
			sweepReason = r
		}
	}
	type sweptItem struct {
		item   *builderItem
		before builderItem
	}
	swept := make([]sweptItem, 0)
	for _, bi := range b.items {
		if bi.state == stateSelected {
			swept = append(swept, sweptItem{item: bi, before: *bi})
			bi.state = stateFailed
			bi.item.Classification = sweepClass
			if bi.item.Reason == "" {
				bi.item.Reason = sweepReason
			}
		}
	}

	cov := b.buildCoverageLocked()
	if err := b.validateLocked(cov); err != nil {
		// A failed Finalize must leave the unfrozen builder exactly as it was
		// before the backstop sweep. Callers may repair a validation problem and
		// still record the real outcome for an item before retrying Finalize.
		for _, entry := range swept {
			*entry.item = entry.before
		}
		return RunManifest{}, err
	}
	m := RunManifest{
		SchemaVersion: ManifestSchemaVersion,
		RunID:         b.runID,
		ParentRunID:   b.parentRunID,
		Operation:     b.operation,
		TerminalState: computeTerminal(cov, b.runFailure),
		Repository:    b.repository,
		Input:         b.input,
		Execution:     b.execution,
		Coverage:      cov,
		RunFailure:    b.runFailure,
		ElapsedMS:     elapsed.Milliseconds(),
	}
	b.frozen = true
	b.result = &m
	return b.result.cloned(), nil
}

// emptyManifest is the well-formed, empty skipped manifest returned alongside an
// error from a nil-builder Finalize, so a mistaken nil receiver never panics and
// its coverage arrays still render as [] rather than null.
func emptyManifest() RunManifest {
	return RunManifest{
		SchemaVersion: ManifestSchemaVersion,
		TerminalState: StateSkipped,
		Coverage: Coverage{
			Selected:  []CoverageItem{},
			Completed: []CoverageItem{},
			Reused:    []CoverageItem{},
			Failed:    []CoverageItem{},
			Waived:    []CoverageItem{},
		},
	}
}

// validateLocked enforces the contract invariants at Finalize. Caller holds b.mu.
func (b *ManifestBuilder) validateLocked(cov Coverage) error {
	if b.runID == "" {
		return errors.New("manifest: empty run_id")
	}
	if b.operation == "" {
		return errors.New("manifest: empty operation")
	}
	if !validInputMode(b.input.Mode) {
		return fmt.Errorf("manifest: invalid input.mode %q", b.input.Mode)
	}
	// selected must be the disjoint union of the four terminal sets. The internal
	// map already guarantees a single state per item_id, so this is a size check
	// plus per-item field checks.
	partition := len(cov.Completed) + len(cov.Reused) + len(cov.Failed) + len(cov.Waived)
	if partition != len(cov.Selected) {
		return fmt.Errorf("manifest: coverage partition mismatch: selected=%d, terminal sum=%d",
			len(cov.Selected), partition)
	}
	for _, it := range cov.Failed {
		if !it.Classification.valid() {
			return fmt.Errorf("manifest: failed item %s has invalid classification %q",
				it.ItemID, it.Classification)
		}
	}
	for _, it := range cov.Waived {
		if it.Reason == "" {
			return fmt.Errorf("manifest: waived item %s missing reason", it.ItemID)
		}
	}
	if b.runFailure != nil && !b.runFailure.Classification.valid() {
		return fmt.Errorf("manifest: invalid run_failure class %q", b.runFailure.Classification)
	}
	return nil
}

// cloned returns a copy of the manifest whose coverage slices are owned copies,
// so every Finalize caller gets an independent, non-aliased snapshot. The
// "immutable" contract then holds even against a consumer that mutates in place.
// Slices are always non-nil so JSON renders "[]" rather than null. RunFailure is
// deep-copied so a caller cannot mutate the builder's stored value.
func (m RunManifest) cloned() RunManifest {
	m.Coverage.Selected = cloneItems(m.Coverage.Selected)
	m.Coverage.Completed = cloneItems(m.Coverage.Completed)
	m.Coverage.Reused = cloneItems(m.Coverage.Reused)
	m.Coverage.Failed = cloneItems(m.Coverage.Failed)
	m.Coverage.Waived = cloneItems(m.Coverage.Waived)
	if m.RunFailure != nil {
		rf := *m.RunFailure
		m.RunFailure = &rf
	}
	return m
}

func cloneItems(src []CoverageItem) []CoverageItem {
	out := make([]CoverageItem, len(src))
	copy(out, src)
	return out
}

// buildCoverageLocked assembles the five sorted, non-nil coverage sets from the
// current item map. Caller must hold b.mu.
func (b *ManifestBuilder) buildCoverageLocked() Coverage {
	cov := Coverage{
		Selected:  []CoverageItem{},
		Completed: []CoverageItem{},
		Reused:    []CoverageItem{},
		Failed:    []CoverageItem{},
		Waived:    []CoverageItem{},
	}
	for _, bi := range b.items {
		sel := CoverageItem{
			ItemID:      bi.item.ItemID,
			Path:        bi.item.Path,
			OldPath:     bi.item.OldPath,
			Fingerprint: bi.item.Fingerprint,
		}
		cov.Selected = append(cov.Selected, sel)
		switch bi.state {
		case stateCompleted:
			cov.Completed = append(cov.Completed, sel)
		case stateReused:
			cov.Reused = append(cov.Reused, sel)
		case stateFailed:
			cov.Failed = append(cov.Failed, bi.item)
		case stateWaived:
			cov.Waived = append(cov.Waived, bi.item)
		}
	}
	sortItems(cov.Selected)
	sortItems(cov.Completed)
	sortItems(cov.Reused)
	sortItems(cov.Failed)
	sortItems(cov.Waived)
	return cov
}

// computeTerminal derives the terminal state from coverage plus run_failure.
// A run_failure forces failed while leaving the recorded outcomes intact. After
// the Finalize sweep every item is terminal, so "no failed" means all
// completed/reused/waived.
func computeTerminal(cov Coverage, rf *RunFailure) TerminalState {
	if rf != nil {
		return StateFailed
	}
	selected := len(cov.Selected)
	if selected == 0 {
		return StateSkipped
	}
	failed := len(cov.Failed)
	switch {
	case failed == 0:
		return StateComplete
	case failed == selected:
		return StateFailed
	default:
		return StatePartial
	}
}

func sortItems(items []CoverageItem) {
	// SliceStable to match the design's "sorted stably by item_id": item_ids are
	// unique per set today so the result is identical to sort.Slice, but stable
	// sort keeps the wording and behavior aligned if a set ever carries dupes.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ItemID < items[j].ItemID
	})
}
