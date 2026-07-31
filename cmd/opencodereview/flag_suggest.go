package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// suggestFlagFromError parses cobra's "unknown flag" error and suggests the
// closest matching flag. Returns empty string if no suggestion is applicable.
func suggestFlagFromError(errMsg string) string {
	// Cobra formats: "unknown flag: --xxx" or "unknown shorthand flag: 'x' in -x"
	var unknown string
	if strings.HasPrefix(errMsg, "unknown flag: ") {
		unknown = strings.TrimPrefix(errMsg, "unknown flag: ")
		unknown = strings.TrimLeft(unknown, "-")
	} else if strings.Contains(errMsg, "unknown shorthand flag") {
		return ""
	} else {
		return ""
	}
	if unknown == "" {
		return ""
	}

	// Find the command that was being executed to access its flags.
	cmd, _, _ := rootCmd.Find(os.Args[1:])
	if cmd == nil {
		cmd = rootCmd
	}
	return suggestFlag(cmd, unknown)
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
