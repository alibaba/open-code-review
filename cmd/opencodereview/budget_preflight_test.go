// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeBudgetPreflight(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"warn", budgetPreflightWarn},
		{" Confirm ", budgetPreflightConfirm},
		{"ABORT", budgetPreflightAbort},
	} {
		got, err := normalizeBudgetPreflight(tc.in)
		if err != nil {
			t.Fatalf("normalizeBudgetPreflight(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("normalizeBudgetPreflight(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if _, err := normalizeBudgetPreflight("ask"); err == nil {
		t.Fatal("normalizeBudgetPreflight(ask) unexpectedly succeeded")
	}
}

func TestBudgetPreflightFlagsRegistered(t *testing.T) {
	if f := reviewCmd.Flags().Lookup("budget-preflight"); f == nil {
		t.Fatal("review --budget-preflight flag is not registered")
	} else if f.DefValue != budgetPreflightWarn {
		t.Errorf("review --budget-preflight default = %q, want %q", f.DefValue, budgetPreflightWarn)
	}
	if f := scanCmd.Flags().Lookup("budget-preflight"); f == nil {
		t.Fatal("scan --budget-preflight flag is not registered")
	} else if f.DefValue != budgetPreflightWarn {
		t.Errorf("scan --budget-preflight default = %q, want %q", f.DefValue, budgetPreflightWarn)
	}
}

func TestBudgetPreflightFreshParsersAcceptFlag(t *testing.T) {
	oldReview, oldScan := reviewBudgetPreflight, scanBudgetPreflight
	defer func() {
		reviewBudgetPreflight = oldReview
		scanBudgetPreflight = oldScan
	}()

	reviewBudgetPreflight = budgetPreflightWarn
	if _, err := parseReviewFlags([]string{"--budget-preflight", budgetPreflightAbort}); err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}
	if reviewBudgetPreflight != budgetPreflightAbort {
		t.Fatalf("review policy = %q, want %q", reviewBudgetPreflight, budgetPreflightAbort)
	}

	scanBudgetPreflight = budgetPreflightWarn
	if _, err := parseScanFlags([]string{"--budget-preflight", budgetPreflightConfirm}); err != nil {
		t.Fatalf("parseScanFlags: %v", err)
	}
	if scanBudgetPreflight != budgetPreflightConfirm {
		t.Fatalf("scan policy = %q, want %q", scanBudgetPreflight, budgetPreflightConfirm)
	}
}

func TestEnforceBudgetPreflight(t *testing.T) {
	t.Run("under budget does nothing", func(t *testing.T) {
		var out bytes.Buffer
		if err := enforceBudgetPreflight(budgetPreflightConfirm, "scan", 100, 100,
			strings.NewReader(""), &out, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Len() != 0 {
			t.Fatalf("unexpected prompt: %q", out.String())
		}
	})

	t.Run("warn preserves existing behavior", func(t *testing.T) {
		if err := enforceBudgetPreflight(budgetPreflightWarn, "review", 200, 100,
			strings.NewReader(""), &bytes.Buffer{}, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("abort rejects before run", func(t *testing.T) {
		err := enforceBudgetPreflight(budgetPreflightAbort, "scan", 200_000, 100_000,
			strings.NewReader(""), &bytes.Buffer{}, false)
		if err == nil || !strings.Contains(err.Error(), "aborted") {
			t.Fatalf("error = %v, want aborted", err)
		}
	})

	t.Run("confirm yes starts run", func(t *testing.T) {
		var out bytes.Buffer
		err := enforceBudgetPreflight(budgetPreflightConfirm, "review", 200_000, 100_000,
			strings.NewReader("yes\n"), &out, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out.String(), "Start anyway? [y/N]") {
			t.Fatalf("prompt = %q", out.String())
		}
		if !strings.Contains(out.String(), "stop new dispatch") {
			t.Fatalf("prompt does not explain dispatch semantics: %q", out.String())
		}
		if !strings.Contains(out.String(), "in-flight work may finish") {
			t.Fatalf("prompt does not explain possible overrun: %q", out.String())
		}
	})

	t.Run("confirm defaults to no", func(t *testing.T) {
		var out bytes.Buffer
		err := enforceBudgetPreflight(budgetPreflightConfirm, "scan", 200, 100,
			strings.NewReader("\n"), &out, true)
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("error = %v, want cancelled", err)
		}
	})

	t.Run("confirm fails closed without tty", func(t *testing.T) {
		var out bytes.Buffer
		err := enforceBudgetPreflight(budgetPreflightConfirm, "scan", 200, 100,
			strings.NewReader("yes\n"), &out, false)
		if err == nil || !strings.Contains(err.Error(), "interactive stdin") {
			t.Fatalf("error = %v, want interactive-stdin failure", err)
		}
		if out.Len() != 0 {
			t.Fatalf("non-interactive path unexpectedly prompted: %q", out.String())
		}
	})
}
