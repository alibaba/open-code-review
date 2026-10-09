// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package intent turns one user prompt into a structured OCR invocation.
//
// It reports three outcomes: a runnable review/scan intent, a clarification
// question, or a rejection. Slash commands are parsed deterministically; every
// other input goes to an LLM through a single constrained tool call. The LLM
// only proposes intent fields. A deterministic layer then builds the final argv
// with the phase 3 contract package, so no prompt text can reach adapter-owned
// flags such as --format or the OCR binary path.
package intent

import "github.com/alibaba/open-code-review/acp/internal/contract"

// Kind is the outcome kind of a parse.
type Kind int

const (
	// KindIntent means the prompt produced a runnable intent.
	KindIntent Kind = iota
	// KindClarify means information is missing; no process may start.
	KindClarify
	// KindReject means the prompt is invalid or out of scope.
	KindReject
)

// Clarify is a question plus the slots that are still missing.
type Clarify struct {
	Question string
	Missing  []string
}

// Reject explains why a prompt was refused and how to retry.
type Reject struct {
	Reason string
	Hint   string
}

// Result is the three-state parse outcome. Exactly one payload is set, matching Kind.
type Result struct {
	Kind    Kind
	Review  *contract.ReviewIntent
	Scan    *contract.ScanIntent
	Clarify *Clarify
	Reject  *Reject
}

// IntentResult wraps a runnable intent.
func IntentResult(review *contract.ReviewIntent, scan *contract.ScanIntent) Result {
	return Result{Kind: KindIntent, Review: review, Scan: scan}
}

// ClarifyResult asks the user for the missing slots.
func ClarifyResult(question string, missing ...string) Result {
	return Result{Kind: KindClarify, Clarify: &Clarify{Question: question, Missing: missing}}
}

// RejectResult refuses the prompt with an actionable hint.
func RejectResult(reason, hint string) Result {
	return Result{Kind: KindReject, Reject: &Reject{Reason: reason, Hint: hint}}
}
