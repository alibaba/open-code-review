// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:     "session",
	Aliases: []string{"sessions"},
	Short:   "List and inspect saved review sessions",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var sessionListRepoDir string
var sessionListJSON bool
var sessionListLimit int

var sessionListCmd = &cobra.Command{
	Use:     "list [flags]",
	Aliases: []string{"ls"},
	Short:   "List recent review sessions for the current repo",
	Long:    "List review sessions previously persisted to ~/.opencodereview/sessions/.\nThe session id printed here can be passed to 'ocr review --resume <id>'.",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionList()
	},
}

var sessionShowRepoDir string
var sessionShowJSON bool

var sessionShowCmd = &cobra.Command{
	Use:               "show [flags] <session-id>",
	Short:             "Show one session's metadata and per-file items",
	Args:              exactArgs(1),
	ValidArgsFunction: completeSessionIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionShow(args[0])
	},
}

var sessionCommentsRepoDir string
var sessionCommentsJSON bool
var sessionCommentsSeverity string
var sessionCommentsCategory string

var sessionCommentsCmd = &cobra.Command{
	Use:               "comments [flags] <session-id>",
	Short:             "Show the review comments recorded in one session",
	Long:              "Print every review comment persisted in a session, formatted like 'ocr review' terminal output.\nUse --json for machine-readable output and --severity/--category to filter findings.",
	Args:              exactArgs(1),
	ValidArgsFunction: completeSessionIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionComments(args[0])
	},
}

// completeSessionIDs offers the persisted session ids for the current repo
// (or --repo, when already typed) as shell completions, newest first, with a
// short summary as the completion description.
func completeSessionIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	repo, _ := cmd.Flags().GetString("repo")
	resolvedRepo, err := resolveWorkingDirForSession(repo)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	summaries, err := session.ListSessions(resolvedRepo)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	completions := make([]string, 0, len(summaries))
	for _, s := range summaries {
		if !strings.HasPrefix(s.SessionID, toComplete) {
			continue
		}
		desc := fmt.Sprintf("%s · %d comments · %s", describeStart(s), s.TotalComments, describeStatus(s))
		completions = append(completions, s.SessionID+"\t"+desc)
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	sessionListCmd.Flags().StringVar(&sessionListRepoDir, "repo", "", "root directory of the git repository (default: current dir)")
	sessionListCmd.Flags().BoolVar(&sessionListJSON, "json", false, "emit JSON instead of a table")
	sessionListCmd.Flags().IntVar(&sessionListLimit, "limit", 20, "cap the number of listed sessions (0 = unlimited)")

	sessionShowCmd.Flags().StringVar(&sessionShowRepoDir, "repo", "", "root directory of the git repository (default: current dir)")
	sessionShowCmd.Flags().BoolVar(&sessionShowJSON, "json", false, "emit JSON instead of a table")

	sessionCommentsCmd.Flags().StringVar(&sessionCommentsRepoDir, "repo", "", "root directory of the git repository (default: current dir)")
	sessionCommentsCmd.Flags().BoolVar(&sessionCommentsJSON, "json", false, "emit the comments as a JSON array")
	sessionCommentsCmd.Flags().StringVar(&sessionCommentsSeverity, "severity", "", "comma-separated severities to include (critical, high, medium, low)")
	sessionCommentsCmd.Flags().StringVar(&sessionCommentsCategory, "category", "", "comma-separated categories to include (e.g. bug, security)")
	sessionCommentsCmd.RegisterFlagCompletionFunc("severity", completeEnum("critical", "high", "medium", "low"))
	sessionCommentsCmd.RegisterFlagCompletionFunc("category", completeEnum("bug", "security", "performance", "maintainability", "test", "style", "documentation", "other"))

	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	sessionCmd.AddCommand(sessionCommentsCmd)
}

func runSessionList() error {
	resolvedRepo, err := resolveWorkingDirForSession(sessionListRepoDir)
	if err != nil {
		return err
	}
	summaries, err := session.ListSessions(resolvedRepo)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	if sessionListLimit > 0 && len(summaries) > sessionListLimit {
		summaries = summaries[:sessionListLimit]
	}

	if sessionListJSON {
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

func runSessionShow(sessionID string) error {
	resolvedRepo, err := resolveWorkingDirForSession(sessionShowRepoDir)
	if err != nil {
		return err
	}
	summary, items, err := session.LoadDetail(resolvedRepo, sessionID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", sessionID, err)
	}

	if sessionShowJSON {
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

func runSessionComments(sessionID string) error {
	resolvedRepo, err := resolveWorkingDirForSession(sessionCommentsRepoDir)
	if err != nil {
		return err
	}
	comments, err := session.LoadComments(resolvedRepo, sessionID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", sessionID, err)
	}
	filtered := filterComments(comments, sessionCommentsSeverity, sessionCommentsCategory)

	if sessionCommentsJSON {
		if filtered == nil {
			filtered = []model.LlmComment{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	if len(filtered) == 0 {
		if len(comments) == 0 {
			fmt.Printf("No comments recorded in session %s.\n", sessionID)
		} else {
			fmt.Printf("No comments match the given filters (%d recorded in session %s).\n", len(comments), sessionID)
		}
		return nil
	}
	for _, c := range filtered {
		renderComment(c)
	}
	return nil
}

// filterComments keeps comments whose severity and category are in the given
// comma-separated, case-insensitive filter lists. Empty filters keep everything.
func filterComments(comments []model.LlmComment, severities, categories string) []model.LlmComment {
	sevSet := parseFilterSet(severities)
	catSet := parseFilterSet(categories)
	if sevSet == nil && catSet == nil {
		return comments
	}
	var out []model.LlmComment
	for _, c := range comments {
		if sevSet != nil && !sevSet[strings.ToLower(c.Severity)] {
			continue
		}
		if catSet != nil && !catSet[strings.ToLower(c.Category)] {
			continue
		}
		out = append(out, c)
	}
	return out
}

func parseFilterSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			set[part] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
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
	if l := s.ResumeLineage; l != nil {
		fmt.Fprintf(w, "  Parent:    run %s\n", l.ParentRunID)
		if l.IsTransition() {
			fmt.Fprintf(w, "  Transition: %s → %s\n",
				describeTarget(l.SourceProvider, l.SourceModel),
				describeTarget(l.TargetProvider, l.TargetModel))
		}
	}
	fmt.Fprintf(w, "  Started:   %s\n", describeStart(*s))
	if !s.EndTime.IsZero() {
		fmt.Fprintf(w, "  Ended:     %s\n", s.EndTime.Local().Format("2006-01-02 15:04:05"))
	}
	if s.Duration > 0 {
		fmt.Fprintf(w, "  Duration:  %s\n", s.Duration.Round(time.Second))
	}
	fmt.Fprintf(w, "  Status:    %s\n", describeStatus(*s))
	if s.RunManifest != nil {
		fmt.Fprintf(w, "  Coverage:  %d selected = %d completed + %d reused + %d failed + %d waived\n",
			s.SelectedFiles, s.CompletedFiles, s.ReusedFiles, s.FailedFiles, s.WaivedFiles)
	} else {
		fmt.Fprintf(w, "  Files:     %d completed, %d reused, %d failed (legacy checkpoints)\n",
			s.CompletedFiles, s.ReusedFiles, s.FailedFiles)
	}
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

// describeTarget renders a provider/model pair for the transition line. Either
// side may be empty (a non-provider endpoint records no provider name).
func describeTarget(provider, model string) string {
	switch {
	case provider == "" && model == "":
		return "-"
	case provider == "":
		return model
	case model == "":
		return provider
	}
	return provider + "/" + model
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
	if s.RunManifest != nil {
		parts := []string{fmt.Sprintf("%d", s.SelectedFiles)}
		if s.ReusedFiles > 0 {
			parts = append(parts, fmt.Sprintf("reused %d", s.ReusedFiles))
		}
		if s.FailedFiles > 0 {
			parts = append(parts, fmt.Sprintf("failed %d", s.FailedFiles))
		}
		if s.WaivedFiles > 0 {
			parts = append(parts, fmt.Sprintf("waived %d", s.WaivedFiles))
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return parts[0] + " (" + strings.Join(parts[1:], ", ") + ")"
	}
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
	if s.RunManifest != nil {
		switch s.RunManifest.TerminalState {
		case session.StateComplete, session.StatePartial, session.StateFailed, session.StateSkipped:
			return string(s.RunManifest.TerminalState)
		default:
			return "unknown"
		}
	}
	if s.FailedFiles > 0 {
		return fmt.Sprintf("legacy (%d fail)", s.FailedFiles)
	}
	return "legacy"
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
