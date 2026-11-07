// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package threadverify decides, one review thread at a time, whether an
// earlier finding is fixed at a given head commit. It drives the same
// llmloop.Runner tool-use loop as review and scan, and accepts a "fixed"
// verdict only when its quoted evidence matches the head commit verbatim.
package threadverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// Decisions a verdict may carry. Only Fixed allows a thread to be resolved.
const (
	Fixed     = "fixed"
	Unfixed   = "unfixed"
	Uncertain = "uncertain"
	Disputed  = "disputed"
)

// MaxThreads bounds how many threads one run verifies; it matches the
// publisher's per-run resolve cap, so verifying more could never be acted on.
const MaxThreads = 50

const verdictToolName = "thread_verdict"

var headSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Reply is one comment in a thread after the finding itself.
type Reply struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

// Thread is one earlier finding to verify.
type Thread struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	Line      int    `json:"line,omitempty"`
	// Outdated threads carry only the lines of the commit the finding was
	// made on, which need not match the head.
	Outdated          bool    `json:"outdated,omitempty"`
	OriginalStartLine int     `json:"original_start_line,omitempty"`
	OriginalLine      int     `json:"original_line,omitempty"`
	Finding           string  `json:"finding"`
	Replies           []Reply `json:"replies,omitempty"`
	// Digest identifies the thread's comments as the publisher read them. It
	// is copied onto the verdict untouched, so the publisher can refuse a
	// verdict about a conversation that changed afterwards.
	Digest string `json:"digest"`
}

// Input is the document the publisher hands to verify-threads.
type Input struct {
	HeadSHA string   `json:"head_sha"`
	Threads []Thread `json:"threads"`
}

// Evidence cites the head-commit lines that show a finding is fixed.
type Evidence struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Quote     string `json:"quote"`
}

// Verdict is the outcome for one thread.
type Verdict struct {
	ThreadID string    `json:"thread_id"`
	Decision string    `json:"decision"`
	Reason   string    `json:"reason,omitempty"`
	Evidence *Evidence `json:"evidence,omitempty"`
	Digest   string    `json:"digest"`
}

// Output is the verdict document the publisher's executor consumes.
type Output struct {
	HeadSHA  string    `json:"head_sha"`
	Verdicts []Verdict `json:"verdicts"`
}

// Args holds what Run needs. Tools must already hold the read-only providers
// (file_read, file_find, code_search) bound to HeadSHA; Run adds the verdict
// tool to it. ReadHead returns a file's content at HeadSHA.
type Args struct {
	LLMClient       llm.LLMClient
	Model           string
	Template        template.Template
	Tools           *tool.Registry
	ToolDefs        []llm.ToolDef
	Session         *session.SessionHistory
	MaxTokensBudget int64
	ReadHead        func(ctx context.Context, path string) (string, error)
}

// Run verifies every thread in in and returns one verdict per thread. A thread
// whose conversation errors, stops early or never records a verdict gets
// Uncertain, so nothing downstream reads a gap as a fix.
func Run(ctx context.Context, args Args, in Input) (Output, error) {
	if !headSHARe.MatchString(in.HeadSHA) {
		return Output{}, fmt.Errorf("head_sha must be a 40-character lowercase hex commit, got %q", in.HeadSHA)
	}
	rec := &recorder{readHead: args.ReadHead, verdicts: map[string]Verdict{}}
	args.Tools.Register(tool.NewBuiltin(tool.Dynamic(verdictToolName), rec.execute))
	runner := llmloop.NewRunner(llmloop.Deps{
		LLMClient:        args.LLMClient,
		Model:            args.Model,
		Template:         args.Template,
		Tools:            args.Tools,
		MainToolDefs:     toolDefs(args.ToolDefs),
		CommentCollector: tool.NewCommentCollector(),
		Session:          args.Session,
		MaxTokensBudget:  args.MaxTokensBudget,
	})
	defer runner.WaitBackground()

	out := Output{HeadSHA: in.HeadSHA, Verdicts: []Verdict{}}
	threads := in.Threads
	if len(threads) > MaxThreads {
		threads = threads[:MaxThreads]
	}
	for _, th := range threads {
		if th.ID == "" {
			continue
		}
		if ctx.Err() != nil {
			out.Verdicts = append(out.Verdicts, Verdict{ThreadID: th.ID, Decision: Uncertain, Reason: "run deadline reached before this thread", Digest: th.Digest})
			continue
		}
		// RunMainTask would still spend a grace round on every remaining
		// thread once the budget is gone.
		if args.MaxTokensBudget > 0 && runner.TotalTokensUsed() >= args.MaxTokensBudget {
			out.Verdicts = append(out.Verdicts, Verdict{ThreadID: th.ID, Decision: Uncertain, Reason: "token budget spent before this thread", Digest: th.Digest})
			continue
		}
		rec.begin(th.ID)
		completed, stop, err := runner.RunMainTask(ctx, messages(th), "verify-thread:"+th.ID)
		v, ok := rec.verdict(th.ID)
		switch {
		case err != nil:
			v = Verdict{ThreadID: th.ID, Decision: Uncertain, Reason: "verification failed: " + err.Error()}
		case !completed || !ok:
			v = Verdict{ThreadID: th.ID, Decision: Uncertain, Reason: "no verdict recorded: " + stop.String()}
		}
		v.Digest = th.Digest
		out.Verdicts = append(out.Verdicts, v)
	}
	return out, nil
}

func toolDefs(base []llm.ToolDef) []llm.ToolDef {
	keep := map[string]bool{
		tool.TaskDone.Name():   true,
		tool.FileRead.Name():   true,
		tool.FileFind.Name():   true,
		tool.CodeSearch.Name(): true,
	}
	var defs []llm.ToolDef
	for _, d := range base {
		if keep[d.Function.Name] {
			defs = append(defs, d)
		}
	}
	return append(defs, verdictToolDef)
}

// recorder holds the verdict tool's state. Threads run one at a time, so it
// only accepts a verdict for the thread currently being verified.
type recorder struct {
	mu       sync.Mutex
	current  string
	verdicts map[string]Verdict
	readHead func(ctx context.Context, path string) (string, error)
}

func (r *recorder) begin(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = id
}

func (r *recorder) verdict(id string) (Verdict, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.verdicts[id]
	return v, ok
}

func (r *recorder) execute(ctx context.Context, raw map[string]any) (string, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	var v Verdict
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("arguments do not match the thread_verdict schema: %w", err)
	}
	r.mu.Lock()
	current := r.current
	r.mu.Unlock()
	if v.ThreadID != current {
		return "", fmt.Errorf("thread_id must be %q, the thread under verification", current)
	}
	switch v.Decision {
	case Unfixed, Uncertain, Disputed:
		v.Evidence = nil
	case Fixed:
		if err := r.checkEvidence(ctx, v.Evidence); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("decision must be one of fixed, unfixed, disputed, uncertain")
	}
	if strings.TrimSpace(v.Reason) == "" {
		return "", errors.New("reason is required")
	}
	r.mu.Lock()
	r.verdicts[v.ThreadID] = v
	r.mu.Unlock()
	return "Verdict recorded. Call task_done.", nil
}

func (r *recorder) checkEvidence(ctx context.Context, e *Evidence) error {
	if e == nil || strings.TrimSpace(e.Quote) == "" || e.Path == "" {
		return errors.New("a fixed verdict needs evidence with path, start_line, end_line and quote")
	}
	content, err := r.readHead(ctx, e.Path)
	if err != nil {
		return fmt.Errorf("cannot read %s at the head commit: %w", e.Path, err)
	}
	if !QuoteMatches(content, e.StartLine, e.EndLine, e.Quote) {
		return fmt.Errorf("evidence.quote is not the exact text of %s lines %d-%d at the head commit; copy those lines from file_read without the line-number prefix", e.Path, e.StartLine, e.EndLine)
	}
	return nil
}

// QuoteMatches reports whether quote equals lines start..end (1-based,
// inclusive) of content, ignoring trailing whitespace on each line and
// trailing blank lines. The publisher applies the same rule before resolving.
func QuoteMatches(content string, start, end int, quote string) bool {
	lines := strings.Split(content, "\n")
	if start < 1 || end < start || end > len(lines) {
		return false
	}
	return normalize(strings.Join(lines[start-1:end], "\n")) == normalize(quote)
}

func normalize(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
