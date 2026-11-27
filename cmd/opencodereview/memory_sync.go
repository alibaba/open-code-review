// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/spf13/cobra"
)

func parseEvidenceLines(lines string) (int, int, error) {
	if lines == "" {
		return 0, 0, nil
	}
	first, last, found := strings.Cut(lines, "-")
	if !found {
		last = first
	}
	start, err1 := strconv.Atoi(first)
	end, err2 := strconv.Atoi(last)
	if err1 != nil || err2 != nil || start < 1 || end < start {
		return 0, 0, fmt.Errorf("evidence lines must be a positive line or an inclusive range like 15-20")
	}
	return start, end, nil
}

func parseMemoryTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected an RFC3339 memory cutoff: %w", err)
	}
	return t, nil
}

func newMemoryFeedback(open memoryOpener) *cobra.Command {
	var action, reason, replacement string
	cmd := &cobra.Command{Use: "feedback <id>", Args: exactArgs(1), Short: "Confirm, reject or replace a business rule with a reason"}
	cmd.Flags().StringVar(&action, "action", "", "confirm, reject or replace")
	cmd.Flags().StringVar(&reason, "reason", "", "reason for the decision")
	cmd.Flags().StringVar(&replacement, "replacement", "", "ID of the replacement rule")
	_ = cmd.MarkFlagRequired("action")
	_ = cmd.MarkFlagRequired("reason")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		store, tree, err := open(cmd, true)
		if err != nil {
			return err
		}
		defer store.Close()
		fact, err := store.Feedback(cmd.Context(), tree, args[0], knowledge.Feedback{Action: action, Reason: reason, ReplacementID: replacement})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(fact)
	}
	return cmd
}

func newMemorySync(open memoryOpener) *cobra.Command {
	var provider, model, scope, rulePath, excludes, asOf string
	var sources, paths, at []string
	cmd := &cobra.Command{Use: "sync", Args: cobra.NoArgs, Short: "Discover business rules from committed code, history, configuration and review discussions"}
	addProviderFlag(cmd, &provider)
	addModelFlag(cmd, &model)
	addRuleFlag(cmd, &rulePath)
	addExcludeFlag(cmd, &excludes)
	cmd.Flags().StringVar(&scope, "scope", "", "limit discovery to a repository path")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "prioritize evidence for a changed repository path (repeatable)")
	cmd.Flags().StringArrayVar(&at, "at", nil, "prioritize source around path:line at the selected commit (repeatable)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "only use discussions known by this RFC3339 timestamp")
	cmd.Flags().StringArrayVar(&sources, "discussions", nil, "PR/MR URL or local discussion JSON (repeatable)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		store, tree, err := open(cmd, true)
		if err != nil {
			return err
		}
		defer store.Close()
		cc, err := loadCommonContext(tree.Repo, rulePath, tree.Commit, 0, 0, true)
		if err != nil {
			return err
		}
		applyCLIExcludes(cc, splitPaths(excludes))
		rt, err := loadLLMRuntime(cc.Template, llm.ResolveOptions{Provider: provider, Model: model, Context: ctx})
		if err != nil {
			return err
		}
		focus := map[string][]int{}
		for _, location := range at {
			p, line, _ := strings.Cut(location, ":")
			n, err := strconv.Atoi(line)
			if p == "" || err != nil || n <= 0 {
				return fmt.Errorf("--at expects a repository path and positive line number, e.g. src/charge.go:42")
			}
			focus[p] = append(focus[p], n)
			paths = append(paths, p)
		}
		cutoff, err := parseMemoryTime(asOf)
		if err != nil {
			return err
		}
		options := knowledge.DiscoverOptions{Scope: scope, Paths: paths, FocusLines: focus, AsOf: cutoff}
		out, err := syncKnowledge(ctx, store, tree, cc, rt, options, sources, nil)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}
	return cmd
}

type memorySyncReport struct {
	Revision          string         `json:"revision"`
	Provider          string         `json:"provider"`
	Model             string         `json:"model"`
	Cached            bool           `json:"cached"`
	Sources           int            `json:"sources,omitempty"`
	Rejected          int            `json:"rejected,omitempty"`
	Truncated         bool           `json:"truncated,omitempty"`
	Facts             int            `json:"facts,omitempty"`
	DiscussionSources int            `json:"discussion_sources,omitempty"`
	Tokens            int64          `json:"tokens,omitempty"`
	SourceKinds       map[string]int `json:"source_kinds,omitempty"`
	Limits            []string       `json:"limits,omitempty"`
	RejectionReasons  []string       `json:"rejection_reasons,omitempty"`
}

type reviewDiscoveryBudget struct {
	limit int
	usage llm.UsageInfo
}

func syncKnowledge(ctx context.Context, store *knowledge.Store, tree knowledge.Tree, cc *commonContext, rt *llmRuntime, opts knowledge.DiscoverOptions, sources []string, budget *reviewDiscoveryBudget) (memorySyncReport, error) {
	out := memorySyncReport{Revision: tree.Commit, Provider: rt.Provider, Model: rt.Model}
	for _, source := range sources {
		notes, err := knowledge.LoadDiscussions(ctx, tree.Repo, source)
		if err != nil {
			return out, err
		}
		opts.Discussions = append(opts.Discussions, notes...)
	}
	opts.Paths = slices.Clone(opts.Paths)
	slices.Sort(opts.Paths)
	opts.Paths = slices.Compact(opts.Paths)
	opts.FileFilter = cc.FileFilter
	if cc.FileFilter != nil {
		tree.AllowPath = func(p string) bool { return !cc.FileFilter.IsUserExcluded(p) }
	}
	now := time.Now().UTC()
	cutoff := opts.AsOf
	if cutoff.IsZero() {
		cutoff = tree.Now
	}
	if cutoff.IsZero() && budget == nil {
		var err error
		cutoff, err = tree.HistoricalDiscussionCutoff(ctx)
		if err != nil {
			return out, err
		}
	}
	tree.Now = cutoff
	if tree.Now.IsZero() {
		tree.Now = now
	}
	version, nextEdit := "business-discovery-v2", ""
	if len(opts.Discussions) != 0 {
		version = "business-discovery-v6"
		if opts.Scope != "" && opts.Scope != "." && slices.ContainsFunc(opts.Discussions, func(d knowledge.Discussion) bool { return d.Path == "" }) {
			// V6 omitted a retirement notice when a pathless edition was
			// ineligible at an explicitly selected file.
			version = "business-discovery-v7"
		}
		opts.AsOf = cutoff
		if opts.AsOf.IsZero() {
			nextEdit = nextDiscussionEdit(opts.Discussions, now)
		}
	}
	var landings []string
	for _, d := range opts.Discussions {
		if key := d.LandingReceipt(tree.Now); key != "" {
			landings = append(landings, key)
		}
	}
	keyData, err := json.Marshal(struct {
		Version, Revision, Provider, Model string
		Live                               bool
		Options                            knowledge.DiscoverOptions
		Sources                            []string
		NextEdit                           string   `json:",omitempty"`
		Landings                           []string `json:",omitempty"`
	}{Version: version, Revision: tree.Commit, Provider: rt.Provider, Model: rt.Model, Live: budget != nil, Options: opts, Sources: sources, NextEdit: nextEdit, Landings: landings})
	if err != nil {
		return out, err
	}
	key := knowledge.Digest(keyData)
	previous, err := store.ListAt(ctx, "", "", tree.Now)
	if err != nil {
		return out, err
	}
	refreshed, err := tree.RefreshSchemas(ctx, previous, opts.Scope)
	if err != nil {
		return out, err
	}
	out.Cached, err = store.Scanned(ctx, key)
	if err != nil {
		return out, err
	}
	if out.Cached {
		return out, store.Save(ctx, refreshed)
	}
	if budget != nil && opts.AsOf.IsZero() {
		opts.AsOf = now
	}
	var skipped string
	result, err := tree.Discover(ctx, opts, func(ctx context.Context, system, corpus string) (string, error) {
		messages := []llm.Message{llm.NewTextMessage("system", system), llm.NewTextMessage("user", corpus)}
		tokens := llmloop.CountMessagesTokens(messages)
		if tokens > llmloop.PromptTokenLimit(cc.Template.MaxTokens) {
			skipped = "discovery input exceeds the configured context window"
		}
		if budget != nil && budget.limit > 0 && int64(tokens)+out.Tokens >= int64(budget.limit) {
			skipped = "discovery would exceed the aggregate review token budget"
		}
		if skipped != "" {
			return "", fmt.Errorf("%s; narrow --scope or raise the budget", skipped)
		}
		resp, err := rt.Client.CompletionsWithCtx(ctx, llm.ChatRequest{Model: rt.Model, Messages: messages, MaxTokens: cc.Template.CompletionTokenLimit()})
		if err != nil {
			return "", err
		}
		if resp.Usage != nil {
			out.Tokens += resp.Usage.TotalTokens
			if budget != nil {
				budget.usage.PromptTokens += resp.Usage.PromptTokens
				budget.usage.CompletionTokens += resp.Usage.CompletionTokens
				budget.usage.CacheReadTokens += resp.Usage.CacheReadTokens
				budget.usage.CacheWriteTokens += resp.Usage.CacheWriteTokens
			}
		}
		return resp.VisibleContent(), nil
	})
	discussionUpdates := result.DiscussionObservations()
	if skipped != "" && budget != nil && ctx.Err() == nil {
		// A later run with adequate room must be able to retry extraction.
		out.Sources, out.SourceKinds, out.Facts = result.Sources, result.SourceKinds, len(refreshed)
		out.Truncated, out.Limits = true, append(result.Limits, skipped)
		fmt.Fprintf(os.Stderr, "[ocr] Skipped business knowledge extraction: %s. Continuing with stored evidence.\n", skipped)
		out.DiscussionSources = len(result.DiscussionSources)
		return out, store.Save(ctx, append(refreshed, discussionUpdates...))
	}
	if err != nil {
		if ctx.Err() == nil && len(discussionUpdates) != 0 {
			// A transport or decoding failure leaves extraction retryable. The
			// authorized, independently validated source does not depend on it.
			if saveErr := store.Save(ctx, discussionUpdates); saveErr != nil {
				return out, fmt.Errorf("%w; saving discussion sources: %v", err, saveErr)
			}
		}
		return out, err
	}
	result.Facts = append(result.Facts, refreshed...)
	out.Sources, out.Rejected, out.Truncated, out.Facts = result.Sources, result.Rejected, result.Truncated, len(result.Facts)
	out.SourceKinds, out.Limits, out.RejectionReasons = result.SourceKinds, result.Limits, result.RejectionReasons
	out.DiscussionSources = len(result.DiscussionSources)
	if !result.Cacheable() {
		return out, store.Save(ctx, append(result.Facts, discussionUpdates...))
	}
	return out, store.SaveDiscovery(ctx, key, append(result.Facts, discussionUpdates...))
}

// A live receipt stays valid until another supplied edition becomes visible.
// An explicit replay cutoff already forms part of the receipt.
func nextDiscussionEdit(discussions []knowledge.Discussion, now time.Time) string {
	var next time.Time
	for _, d := range discussions {
		stamp := d.UpdatedAt
		if stamp == "" {
			stamp = d.CreatedAt
		}
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil && at.After(now) && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	if next.IsZero() {
		return ""
	}
	return next.UTC().Format(time.RFC3339Nano)
}

func syncReviewBase(ctx context.Context, cc *commonContext, opts reviewOptions, code *reviewCodeTools, rt *llmRuntime, budget *reviewDiscoveryBudget) error {
	store, err := openMemory(ctx, opts.memoryDB, cc.RepoDir, cc.GitRunner, true)
	if err != nil {
		return err
	}
	defer store.Close()
	if code.resolution.BaseIsEmpty {
		// The first commit has no accepted repository history to learn from.
		return nil
	}
	base := code.resolution.ResolvedBase
	runner, err := cc.GitRunner.LocalForRepository(ctx, cc.RepoDir)
	if err != nil {
		return err
	}
	refs := []string{base}
	if head := code.resolution.ResolvedHead; head != "" {
		refs = append(refs, head)
	}
	args := append([]string{"diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-only", "-z"}, refs...)
	raw, err := runner.Output(ctx, cc.RepoDir, append(args, "--")...)
	if err != nil {
		return err
	}
	if code.resolution.ResolvedHead == "" {
		untracked, err := runner.Output(ctx, cc.RepoDir, "ls-files", "--others", "--exclude-standard", "-z")
		if err != nil {
			return err
		}
		raw = append(raw, untracked...)
	}
	var paths []string
	for _, p := range bytes.Split(raw, []byte{0}) {
		if len(p) > 0 && (cc.FileFilter == nil || !cc.FileFilter.IsUserExcluded(string(p))) {
			paths = append(paths, string(p))
		}
	}
	args = append([]string{"--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--unified=0", "--no-color"}, refs...)
	args = append(args, "--")
	focus := map[string][]int{}
	if len(paths) > 0 {
		args = append(args, paths[:min(len(paths), 32)]...)
		err = runner.Stream(ctx, cc.RepoDir, func(stdout io.Reader) error {
			patch, err := io.ReadAll(io.LimitReader(stdout, 2<<20))
			if err != nil {
				return err
			}
			for _, change := range diff.ParsePatch(string(patch)) {
				for _, h := range diff.HunkRanges(change.Diff) {
					if h.OldStart > 0 && len(focus[change.OldPath]) < 32 {
						focus[change.OldPath] = append(focus[change.OldPath], h.OldStart)
					}
				}
			}
			_, err = io.Copy(io.Discard, stdout)
			return err
		}, args...)
		if err != nil {
			return err
		}
	}
	tree := knowledge.Tree{Repo: cc.RepoDir, Commit: base, Git: runner, Now: opts.memoryTime}
	report, err := syncKnowledge(ctx, store, tree, cc, rt, knowledge.DiscoverOptions{Paths: paths, FocusLines: focus, AsOf: opts.memoryTime}, opts.memoryDiscussions, budget)
	if err == nil && !report.Cached {
		fmt.Fprintf(os.Stderr, "[ocr] Memory sync at %.12s: %d sampled sources, %d rules, %d original discussions, %d rejected, truncated=%t\n", base, report.Sources, report.Facts, report.DiscussionSources, report.Rejected, report.Truncated)
	}
	return err
}
