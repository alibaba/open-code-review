package main

// go build ./cmd/testdiff/ -o /tmp/testdiff && /tmp/testdiff ...
// Or just: go run ./cmd/testdiff/ ...
import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/model"
)

func main() {
	args := parseArgs(os.Args[1:])
	if args.showHelp || len(args.raw) == 0 {
		printUsage()
		os.Exit(0)
	}

	repoDir, err := resolveRepo(args.repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	provider := buildProvider(repoDir, args)
	diffs, err := provider.GetDiff()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(diffs) == 0 {
		fmt.Println("(no changes)")
		return
	}

	if args.summary {
		printSummary(diffs)
		return
	}

	if args.format == "json" {
		out, _ := json.MarshalIndent(diffs, "", "  ")
		fmt.Println(string(out))
		return
	}

	printText(diffs)
}

// ---- argument parsing ----

type cliArgs struct {
	repo    string
	from    string
	to      string
	commit  string
	format  string // "text" or "json"
	summary bool   // just print file list and stats
	showHelp bool
	raw     []string
}

func parseArgs(args []string) cliArgs {
	result := cliArgs{raw: args, format: "text"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			result.showHelp = true
			return result
		case "-repo":
			i++; result.repo = args[i]
		case "-from":
			i++; result.from = args[i]
		case "-to":
			i++; result.to = args[i]
		case "-commit":
			i++; result.commit = args[i]
		case "-format":
			i++; result.format = args[i]
		case "-summary":
			result.summary = true
		}
	}
	return result
}

func printUsage() {
	fmt.Println(`testdiff — quick diff parsing test helper.

Usage:
  go run ./cmd/testdiff [flags]

Examples:
  # Workspace mode (default if no refs given, runs from CWD)
  go run ./cmd/testdiff

  # Range mode
  go run ./cmd/testdiff -from master -to dev-ref

  # Single commit vs its parent
  go run ./cmd/testdiff -commit abc1234

  # Multiple commits combined
  go run ./cmd/testdiff -commit abc1234,def5678

  # Summary only (file paths and line counts)
  go run ./cmd/testdiff -from master -to dev-ref -summary

Flags:
  -repo DIR       git repository root (default: auto-detect via git rev-parse)
  -from REF       source ref (e.g. 'main')
  -to REF         target ref (e.g. 'feature-branch')
  -commit SHA     commit hash(es) to review -- single or comma-separated for multiple
  -format FMT     output format: text or json (default: text)
  -summary        show file list and insertions/deletions only`)
}

func resolveRepo(input string) (string, error) {
	if input == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("not in a git repo (%s)", strings.TrimSpace(string(out)))
		}
		input = strings.TrimSpace(string(out))
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func buildProvider(repoDir string, args cliArgs) *diff.Provider {
	switch {
	case args.commit != "":
		if strings.Contains(args.commit, ",") {
			commits := parseCommitList(args.commit)
			return diff.NewMultiCommitProvider(repoDir, commits)
		}
		return diff.NewCommitProvider(repoDir, args.commit)
	case args.from != "" && args.to != "":
		return diff.NewProvider(repoDir, args.from, args.to)
	default:
		return diff.NewWorkspaceProvider(repoDir)
	}
}

func parseCommitList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ---- output helpers ----

func printSummary(diffs []model.Diff) {
	var totalAdd, totalDel int64
	for _, d := range diffs {
		status := "M"
		if d.IsNew {
			status = "A"
		} else if d.IsDeleted {
			status = "D"
		}
		path := d.NewPath
		if path == "/dev/null" {
			path = d.OldPath
		}
		fmt.Printf("  %s  %-4s  +%d/-%d  %s\n", status, "", d.Insertions, d.Deletions, path)
		totalAdd += d.Insertions
		totalDel += d.Deletions
	}
	fmt.Printf("\n%d file(s), +%d/-%d lines\n", len(diffs), totalAdd, totalDel)
}

func printText(diffs []model.Diff) {
	for i, d := range diffs {
		path := d.NewPath
		if path == "/dev/null" {
			path = d.OldPath
		}
		status := "MODIFIED"
		if d.IsNew {
			status = "ADDED"
		} else if d.IsDeleted {
			status = "DELETED"
		}
		fmt.Printf("--- %s (%s, +%d/-%d) ---\n", path, status, d.Insertions, d.Deletions)
		fmt.Print(d.Diff)
		if i < len(diffs)-1 {
			fmt.Println()
		}
	}
}
