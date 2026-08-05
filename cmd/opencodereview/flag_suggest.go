// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagErrorWithSuggestion is a SetFlagErrorFunc handler that appends a
// "Did you mean?" suggestion when the user misspells a flag.
func flagErrorWithSuggestion(cmd *cobra.Command, err error) error {
	msg := err.Error()

	var unknown string
	if strings.HasPrefix(msg, "unknown flag: ") {
		unknown = strings.TrimPrefix(msg, "unknown flag: ")
		unknown = strings.TrimLeft(unknown, "-")
	}
	if unknown == "" {
		return err
	}

	if suggestion := suggestFlag(cmd, unknown); suggestion != "" {
		return fmt.Errorf("%w%s", err, suggestion)
	}
	return err
}

func suggestFlag(cmd *cobra.Command, unknown string) string {
	unknown = strings.TrimLeft(unknown, "-")
	if unknown == "" {
		return ""
	}

	var best string
	bestDist := 3 // max edit distance to consider
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		d := levenshtein(unknown, f.Name)
		if d < bestDist {
			bestDist = d
			best = f.Name
		}
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		d := levenshtein(unknown, f.Name)
		if d < bestDist {
			bestDist = d
			best = f.Name
		}
	})

	if best != "" {
		return fmt.Sprintf("\n\nDid you mean this?\n\t--%s", best)
	}
	return ""
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
