// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"format", "forma", 1},
		{"model", "modle", 2},
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSuggestFlag(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	parent.PersistentFlags().String("repo", "", "")
	child := &cobra.Command{Use: "child"}
	child.Flags().String("format", "", "")
	child.Flags().String("model", "", "")
	parent.AddCommand(child)

	t.Run("close match on local flag", func(t *testing.T) {
		if got := suggestFlag(child, "forma"); got == "" || !strings.Contains(got, "--format") {
			t.Errorf("suggestFlag(forma) = %q, want suggestion for --format", got)
		}
	})
	t.Run("close match on inherited flag", func(t *testing.T) {
		if got := suggestFlag(child, "rep"); got == "" || !strings.Contains(got, "--repo") {
			t.Errorf("suggestFlag(rep) = %q, want suggestion for --repo", got)
		}
	})
	t.Run("no close match", func(t *testing.T) {
		if got := suggestFlag(child, "zzzzzzzz"); got != "" {
			t.Errorf("suggestFlag(zzzzzzzz) = %q, want empty", got)
		}
	})
	t.Run("empty after trimming dashes", func(t *testing.T) {
		if got := suggestFlag(child, "--"); got != "" {
			t.Errorf("suggestFlag(--) = %q, want empty", got)
		}
	})
}

func TestFlagErrorWithSuggestion(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("format", "", "")

	t.Run("unknown flag yields suggestion", func(t *testing.T) {
		in := errors.New("unknown flag: --forma")
		out := flagErrorWithSuggestion(cmd, in)
		if !strings.Contains(out.Error(), "Did you mean") {
			t.Errorf("expected suggestion, got %q", out.Error())
		}
	})
	t.Run("unknown flag with no close match returns original", func(t *testing.T) {
		in := errors.New("unknown flag: --zzzzzzzz")
		out := flagErrorWithSuggestion(cmd, in)
		if out != in {
			t.Errorf("expected original error, got %q", out.Error())
		}
	})
	t.Run("non-flag error returned unchanged", func(t *testing.T) {
		in := errors.New("some other error")
		out := flagErrorWithSuggestion(cmd, in)
		if out != in {
			t.Errorf("expected original error, got %q", out.Error())
		}
	})
	t.Run("dashes-only unknown returned unchanged", func(t *testing.T) {
		in := errors.New("unknown flag: ---")
		out := flagErrorWithSuggestion(cmd, in)
		if out != in {
			t.Errorf("expected original error, got %q", out.Error())
		}
	})
}
