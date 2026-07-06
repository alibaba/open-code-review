package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/open-code-review/open-code-review/internal/session"
)

func runSession(args []string) error {
	if len(args) == 0 {
		printSessionUsage()
		return nil
	}
	switch args[0] {
	case "list", "ls":
		return runSessionList(args[1:])
	case "show":
		return runSessionShow(args[1:])
	case "-h", "--help":
		printSessionUsage()
		return nil
	default:
		return fmt.Errorf("unknown session sub-command: %s\nRun 'ocr session -h' for usage", args[0])
	}
}

func runSessionList(args []string) error {
	a := newOcrFlagSet("ocr session list")
	var repoDir string
	var asJSON bool
	var limit int
	a.StringVar(&repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.BoolVar(&asJSON, "json", false, "emit JSON instead of a table")
	a.IntVar(&limit, "limit", 20, "cap the number of listed sessions (0 = unlimited)")
	if err := a.Parse(args); err != nil {
		return err
	}
	if a.showHelp {
		printSessionListUsage()
		return nil
	}

	resolvedRepo, err := resolveWorkingDirForSession(repoDir)
	if err != nil {
		return err
	}
	summaries, err := session.ListSessions(resolvedRepo)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summaries)
	}

	if len(summaries) == 0 {
		fmt.Printf("No sessions found for %s\n", resolvedRepo)
		return nil
	}
	printSessionTable(os.Stdout, summaries)
	return nil
}

func runSessionShow(args []string) error {
	a := newOcrFlagSet("ocr session show")
	var repoDir string
	var asJSON bool
	a.StringVar(&repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.BoolVar(&asJSON, "json", false, "emit JSON instead of a table")
	if err := a.Parse(args); err != nil {
		return err
	}
	if a.showHelp {
		printSessionShowUsage()
		return nil
	}

	rest := a.fs.Args()
	if len(rest) == 0 {
		printSessionShowUsage()
		return fmt.Errorf("session show requires a session ID")
	}
	sessionID := rest[0]

	resolvedRepo, err := resolveWorkingDirForSession(repoDir)
	if err != nil {
		return err
	}
	summary, items, err := session.LoadDetail(resolvedRepo, sessionID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", sessionID, err)
	}

	if asJSON {
		payload := struct {
			Summary *session.Summary     `json:"summary"`
			Items   []session.ItemDetail `json:"items"`
		}{Summary: summary, Items: items}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	printSessionDetail(os.Stdout, summary, items)
	return nil
}

// resolveWorkingDirForSession accepts an explicit --repo flag value and falls
// back to the current working directory. Unlike resolveRepoDir it does not
// require the target to be a git repository, so users can inspect sessions
// even after archiving a checkout.
func resolveWorkingDirForSession(input string) (string, error) {
	dir, _, err := resolveWorkingDir(input, false)
	if err != nil {
		return "", err
	}
	return dir, nil
}

func printSessionTable(w io.Writer, summaries []session.Summary) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION ID\tMODE\tRANGE\tFILES\tCOMMENTS\tSTATUS\tSTARTED")
	for _, s := range summaries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			s.SessionID,
			displayMode(s.ReviewMode),
			describeRange(s),
			describeFiles(s),
			s.TotalComments,
			describeStatus(s),
			describeStart(s),
		)
	}
	tw.Flush()
}

func printSessionDetail(w io.Writer, s *session.Summary, items []session.ItemDetail) {
	fmt.Fprintf(w, "Session: %s\n", s.SessionID)
	fmt.Fprintf(w, "  File:      %s\n", s.FilePath)
	fmt.Fprintf(w, "  Repo:      %s\n", s.RepoDir)
	if s.GitBranch != "" {
		fmt.Fprintf(w, "  Branch:    %s\n", s.GitBranch)
	}
	if s.Model != "" {
		fmt.Fprintf(w, "  Model:     %s\n", s.Model)
	}
	fmt.Fprintf(w, "  Mode:      %s\n", displayMode(s.ReviewMode))
	if r := describeRange(*s); r != "" && r != "-" {
		fmt.Fprintf(w, "  Range:     %s\n", r)
	}
	if s.ResumedFrom != "" {
		fmt.Fprintf(w, "  Resumed:   from session %s\n", s.ResumedFrom)
	}
	fmt.Fprintf(w, "  Started:   %s\n", describeStart(*s))
	if !s.EndTime.IsZero() {
		fmt.Fprintf(w, "  Ended:     %s\n", s.EndTime.Local().Format("2006-01-02 15:04:05"))
	}
	if s.Duration > 0 {
		fmt.Fprintf(w, "  Duration:  %s\n", s.Duration.Round(time.Second))
	}
	fmt.Fprintf(w, "  Status:    %s\n", describeStatus(*s))
	fmt.Fprintf(w, "  Files:     %d completed, %d reused, %d failed\n",
		s.CompletedFiles, s.ReusedFiles, s.FailedFiles)
	fmt.Fprintf(w, "  Comments:  %d\n", s.TotalComments)
	if s.LLMFailures > 0 {
		fmt.Fprintf(w, "  LLM err:   %d\n", s.LLMFailures)
	}

	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Files:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  TYPE\tFILE\tCOMMENTS\tNOTE")
	for _, it := range items {
		note := ""
		switch it.Type {
		case "reused":
			note = "from " + shortSessionID(it.SourceSessionID)
		case "failed":
			note = truncate(it.Error, 60)
		}
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%s\n", it.Type, it.FilePath, it.Comments, note)
	}
	tw.Flush()
}

func displayMode(m string) string {
	if m == "" {
		return "-"
	}
	return m
}

func describeRange(s session.Summary) string {
	switch s.ReviewMode {
	case session.ReviewModeRange:
		if s.DiffFrom != "" || s.DiffTo != "" {
			return fmt.Sprintf("%s..%s", s.DiffFrom, s.DiffTo)
		}
	case session.ReviewModeCommit:
		if s.DiffCommit != "" {
			return s.DiffCommit
		}
	}
	return "-"
}

func describeFiles(s session.Summary) string {
	total := s.CompletedFiles + s.ReusedFiles
	if s.ReusedFiles > 0 {
		return fmt.Sprintf("%d (reused %d)", total, s.ReusedFiles)
	}
	return fmt.Sprintf("%d", total)
}

func describeStatus(s session.Summary) string {
	if s.Aborted {
		return "aborted"
	}
	if s.FailedFiles > 0 {
		return fmt.Sprintf("completed (%d fail)", s.FailedFiles)
	}
	return "completed"
}

func describeStart(s session.Summary) string {
	if s.StartTime.IsZero() {
		return "-"
	}
	return s.StartTime.Local().Format("2006-01-02 15:04:05")
}

func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\t", " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}

func printSessionUsage() {
	fmt.Println(`Usage:
  ocr session <sub-command>

Sub-commands:
  list, ls    List recent review sessions for the current repo
  show <id>   Show one session's metadata and per-file items

Use "ocr session list -h" or "ocr session show -h" for details.`)
}

func printSessionListUsage() {
	fmt.Println(`Usage:
  ocr session list [flags]
  ocr session ls [flags]

List review sessions previously persisted to ~/.opencodereview/sessions/. The
session id printed here can be passed to 'ocr review --resume <id>'.

Flags:
  --repo string   Root directory of the git repository (default: current dir)
  --json          Emit JSON instead of a table
  --limit int     Cap the number of listed sessions (default 20; 0 = unlimited)`)
}

func printSessionShowUsage() {
	fmt.Println(`Usage:
  ocr session show [flags] <session-id>

Show metadata and per-file items for a single session.

Flags:
  --repo string   Root directory of the git repository (default: current dir)
  --json          Emit JSON instead of a table`)
}
