package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
)

// runDismiss dispatches the `ocr dismiss` subcommand, which records and
// manages dismissed findings (per-repo, under ~/.opencodereview/dismissals/).
// Dismissed findings are suppressed from `ocr review` output via the read-side
// DismissalFilter (see internal/agent). This command is the recording surface.
func runDismiss(args []string) error {
	if len(args) == 0 {
		printDismissUsage()
		return nil
	}
	switch args[0] {
	case "add":
		return runDismissAdd(args[1:])
	case "list", "ls":
		return runDismissList(args[1:])
	case "remove", "rm":
		return runDismissRemove(args[1:])
	case "-h", "--help":
		printDismissUsage()
		return nil
	default:
		return fmt.Errorf("unknown dismiss sub-command: %s\nRun 'ocr dismiss -h' for usage", args[0])
	}
}

// runDismissAdd records a dismissal: `ocr dismiss add <session-id> <finding-ref> [--repo DIR]`.
// <finding-ref> is either a 0-based index into the session's flattened comment
// list, or a path:line form (`file.go:42`) that resolves to the first comment
// (in JSONL file-record order) whose Path matches and whose [StartLine,EndLine]
// contains the line. If multiple comments qualify, the command reports all
// candidates with their indices and asks the user to disambiguate via the index.
func runDismissAdd(args []string) error {
	a := newOcrFlagSet("ocr dismiss add")
	var repoDir string
	a.StringVar(&repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	if err := a.Parse(args); err != nil {
		return err
	}
	if a.showHelp {
		printDismissAddUsage()
		return nil
	}

	rest := a.fs.Args()
	if len(rest) < 2 {
		printDismissAddUsage()
		return fmt.Errorf("dismiss add requires a session ID and a finding reference")
	}
	sessionID := rest[0]
	findingRef := rest[1]

	resolvedRepo, err := resolveWorkingDirForSession(repoDir)
	if err != nil {
		return err
	}

	comments, err := session.LoadComments(resolvedRepo, sessionID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", sessionID, err)
	}
	if len(comments) == 0 {
		return fmt.Errorf("session %q has no review comments to dismiss", sessionID)
	}

	target, err := resolveFindingRef(comments, findingRef)
	if err != nil {
		return err
	}

	// Load the existing store. On corrupt/unreadable, LoadDismissals returns a
	// non-nil store (for its path) alongside an error. We treat corruption as a
	// hard error for `dismiss add` rather than silently wiping: the user must
	// acknowledge it before recording new dismissals (D6 — file left untouched).
	store, err := session.LoadDismissals(resolvedRepo)
	if err != nil {
		path := resolvedRepo
		if store != nil {
			path = store.Path()
		}
		return fmt.Errorf("load dismissal store %q: %w (the corrupt file was left untouched; inspect or remove it manually)", path, err)
	}

	fp := session.DismissalFingerprint(target)
	store.Record(session.DismissalEntry{
		Fingerprint:     fp,
		Path:            target.Path,
		ContentPreview:  truncateForPreview(target.Content),
		DismissedAt:     time.Now().UTC().Format(time.RFC3339),
		SourceSessionID: sessionID,
	})
	if err := store.Save(); err != nil {
		return fmt.Errorf("save dismissal store: %w", err)
	}
	fmt.Printf("Dismissed %s:%d-%d from session %s\n", target.Path, target.StartLine, target.EndLine, sessionID)
	fmt.Printf("Fingerprint: %s\n", fp)
	fmt.Println("Run 'ocr review' again to suppress this finding from future reviews.")
	return nil
}

// runDismissList prints the recorded dismissals for a repo as a table.
func runDismissList(args []string) error {
	a := newOcrFlagSet("ocr dismiss list")
	var repoDir string
	a.StringVar(&repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	if err := a.Parse(args); err != nil {
		return err
	}
	if a.showHelp {
		printDismissListUsage()
		return nil
	}

	resolvedRepo, err := resolveWorkingDirForSession(repoDir)
	if err != nil {
		return err
	}
	store, err := session.LoadDismissals(resolvedRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: dismissal store corrupt (%v); showing no dismissals. The file was left untouched.\n", err)
	}
	entries := store.List()
	if len(entries) == 0 {
		fmt.Printf("No dismissed findings for %s\n", resolvedRepo)
		return nil
	}
	printDismissTable(os.Stdout, entries)
	return nil
}

// runDismissRemove deletes a dismissal by fingerprint (or its list index).
func runDismissRemove(args []string) error {
	a := newOcrFlagSet("ocr dismiss remove")
	var repoDir string
	a.StringVar(&repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	if err := a.Parse(args); err != nil {
		return err
	}
	if a.showHelp {
		printDismissRemoveUsage()
		return nil
	}

	rest := a.fs.Args()
	if len(rest) == 0 {
		printDismissRemoveUsage()
		return fmt.Errorf("dismiss remove requires a fingerprint or list index")
	}
	ref := rest[0]

	resolvedRepo, err := resolveWorkingDirForSession(repoDir)
	if err != nil {
		return err
	}
	store, err := session.LoadDismissals(resolvedRepo)
	if err != nil {
		return fmt.Errorf("load dismissal store: %w", err)
	}

	fp, err := resolveRemoveRef(store, ref)
	if err != nil {
		return err
	}
	if !store.Remove(fp) {
		return fmt.Errorf("no dismissed finding matching %q", ref)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("save dismissal store: %w", err)
	}
	fmt.Printf("Removed dismissal %s\n", fp)
	return nil
}

// resolveFindingRef resolves a 0-based index or path:line reference to a
// comment in the flattened session comment list (JSONL file-record order).
func resolveFindingRef(comments []model.LlmComment, ref string) (model.LlmComment, error) {
	// Index form: a non-negative integer.
	if n, err := strconv.Atoi(ref); err == nil {
		if n < 0 || n >= len(comments) {
			return model.LlmComment{}, fmt.Errorf("finding index %d out of range (session has %d comments; valid 0..%d)", n, len(comments), len(comments)-1)
		}
		return comments[n], nil
	}
	// path:line form: first comment in file-record order whose Path matches and
	// whose [StartLine, EndLine] contains the line. Multiple matches => report
	// candidates with indices and ask the user to disambiguate via the index form.
	path, line, err := parsePathLine(ref)
	if err != nil {
		return model.LlmComment{}, fmt.Errorf("finding reference must be a 0-based index or path:line (e.g. \"3\" or \"file.go:42\"): %v", err)
	}
	var matches []int
	for i, c := range comments {
		if c.Path == path && c.StartLine <= line && line <= c.EndLine {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return model.LlmComment{}, fmt.Errorf("no finding in session matches %s:%d", path, line)
	case 1:
		return comments[matches[0]], nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "multiple findings in session match %s:%d — disambiguate using the index form:\n", path, line)
		for _, idx := range matches {
			c := comments[idx]
			fmt.Fprintf(&b, "  [%d] %s:%d-%d  %s\n", idx, c.Path, c.StartLine, c.EndLine, truncateForPreview(c.Content))
		}
		return model.LlmComment{}, fmt.Errorf("%s", b.String())
	}
}

// resolveRemoveRef resolves a remove reference to a fingerprint. It accepts the
// full fingerprint, the short prefix (≥4 chars) shown by `dismiss list`, or a
// 0-based list index. Fingerprint forms are checked before the index form so an
// all-decimal hex prefix cannot be misread as an index.
func resolveRemoveRef(store *session.DismissalStore, ref string) (string, error) {
	entries := store.List()
	// Exact fingerprint match.
	if store.Contains(ref) {
		return ref, nil
	}
	// Short fingerprint prefix (first 12 chars, as printed by `dismiss list`).
	if len(ref) >= 4 {
		var match string
		count := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Fingerprint, ref) {
				match = e.Fingerprint
				count++
			}
		}
		if count == 1 {
			return match, nil
		}
	}
	// List index form (checked last so a decimal-looking fingerprint prefix is
	// never misread as an index).
	if n, err := strconv.Atoi(ref); err == nil && n >= 0 && n < len(entries) {
		return entries[n].Fingerprint, nil
	}
	return "", fmt.Errorf("no dismissed finding matching %q (use 'ocr dismiss list' to see fingerprints/indices)", ref)
}

// parsePathLine splits a "file.go:42" reference into path and line.
func parsePathLine(ref string) (string, int, error) {
	idx := strings.LastIndex(ref, ":")
	if idx <= 0 || idx == len(ref)-1 {
		return "", 0, fmt.Errorf("missing ':line' in %q", ref)
	}
	path := ref[:idx]
	line, err := strconv.Atoi(ref[idx+1:])
	if err != nil || line < 1 {
		return "", 0, fmt.Errorf("invalid line number in %q", ref)
	}
	return path, line, nil
}

// truncateForPreview caps a comment body for storage/display, collapsing
// newlines/tabs to spaces first so the preview stays one line.
func truncateForPreview(s string) string {
	const max = 160
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\t", " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func printDismissTable(w io.Writer, entries []session.DismissalEntry) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "INDEX\tFINGERPRINT\tPATH\tPREVIEW\tDISMISSED AT")
	for i, e := range entries {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", i, shortFingerprint(e.Fingerprint), e.Path, truncateForPreview(e.ContentPreview), e.DismissedAt)
	}
	tw.Flush()
}

func shortFingerprint(fp string) string {
	if len(fp) > 12 {
		return fp[:12]
	}
	return fp
}

func printDismissUsage() {
	fmt.Println(`Usage:
  ocr dismiss <sub-command> [flags]

Sub-commands:
  add <session-id> <finding-ref>   Record a dismissal of a finding from a session
  list, ls                         List dismissed findings for the current repo
  remove, rm <fingerprint|index>   Remove a recorded dismissal

<finding-ref> is a 0-based index into the session's flattened comment list, or a
path:line form (e.g. "file.go:42") that resolves to the first comment whose line
range contains the line.

Flags apply to all sub-commands:
  --repo string   Root directory of the git repository (default: current dir)

Use "ocr dismiss add -h", "ocr dismiss list -h", or "ocr dismiss remove -h" for details.`)
}

func printDismissAddUsage() {
	fmt.Println(`Usage:
  ocr dismiss add [flags] <session-id> <finding-ref>

Record a dismissal for a single finding from a review session. The finding's
fingerprint (SHA256 of path + line range + normalized content) is stored under
~/.opencodereview/dismissals/<encoded-repo>.json; subsequent 'ocr review' runs
suppress findings whose fingerprint matches.

Arguments:
  <session-id>    A session id from 'ocr session list'.
  <finding-ref>   A 0-based index into the session's flattened comment list, or
                  a path:line form (e.g. "file.go:42") resolving to the first
                  comment whose [start,end] line range contains the line.

Flags:
  --repo string   Root directory of the git repository (default: current dir)`)
}

func printDismissListUsage() {
	fmt.Println(`Usage:
  ocr dismiss list [flags]
  ocr dismiss ls [flags]

List dismissed findings recorded for the repository, ordered by dismissal time.

Flags:
  --repo string   Root directory of the git repository (default: current dir)`)
}

func printDismissRemoveUsage() {
	fmt.Println(`Usage:
  ocr dismiss remove [flags] <fingerprint|index>
  ocr dismiss rm [flags] <fingerprint|index>

Remove a recorded dismissal so the finding reappears in future reviews. The
reference may be the full fingerprint, the 12-char prefix shown by 'dismiss
list', or the list index printed in the INDEX column.

Flags:
  --repo string   Root directory of the git repository (default: current dir)`)
}
