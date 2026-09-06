// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

const (
	budgetPreflightWarn    = "warn"
	budgetPreflightConfirm = "confirm"
	budgetPreflightAbort   = "abort"
)

var (
	// Budget preflight is command-admission state rather than runtime agent
	// configuration. registerReviewFlags/registerScanFlags bind these values so
	// fresh Cobra commands used by the compatibility tests accept the same flag
	// surface as the production commands.
	reviewBudgetPreflight = budgetPreflightWarn
	scanBudgetPreflight   = budgetPreflightWarn
)

func init() {
	reviewCmd.PreRunE = chainPreRunE(reviewCmd.PreRunE, runReviewBudgetPreflight)
	scanCmd.PreRunE = chainPreRunE(scanCmd.PreRunE, runScanBudgetPreflight)
}

func chainPreRunE(first, second func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if first != nil {
			if err := first(cmd, args); err != nil {
				return err
			}
		}
		return second(cmd, args)
	}
}

func normalizeBudgetPreflight(raw string) (string, error) {
	policy := strings.ToLower(strings.TrimSpace(raw))
	switch policy {
	case budgetPreflightWarn, budgetPreflightConfirm, budgetPreflightAbort:
		return policy, nil
	default:
		return "", fmt.Errorf("invalid --budget-preflight value %q: must be one of warn, confirm, abort", raw)
	}
}

func resolveBudgetPreflightMaxTokens(templateDefault, cliOverride int) (int, error) {
	cfgPath, err := defaultConfigPath()
	if err != nil {
		return 0, err
	}
	appCfg, err := LoadAppConfig(cfgPath)
	if err != nil {
		return 0, fmt.Errorf("load app config: %w", err)
	}
	return resolveMaxTokens(templateDefault, appCfg, cliOverride)
}

func runReviewBudgetPreflight(cmd *cobra.Command, _ []string) error {
	policy, err := normalizeBudgetPreflight(reviewBudgetPreflight)
	if err != nil {
		return err
	}
	reviewBudgetPreflight = policy

	// Preview never calls the LLM, and warn is the backwards-compatible default:
	// the existing review path already prints its estimate and warning once the
	// run starts. Avoid a second git pass for both cases.
	if reviewOpts.preview || policy == budgetPreflightWarn {
		return nil
	}
	if err := validateReviewOptions(&reviewOpts); err != nil {
		return err
	}
	if reviewOpts.maxTokensBudget <= 0 {
		return fmt.Errorf("--budget-preflight=%s requires --max-tokens-budget to be greater than 0", policy)
	}

	contentRef, _ := tool.ParseReviewMode(reviewOpts.from, reviewOpts.to, reviewOpts.commit).
		RefValue(reviewOpts.to, reviewOpts.commit)
	cc, err := loadCommonContext(reviewOpts.repoDir, reviewOpts.rulePath, contentRef,
		reviewOpts.maxTools, reviewOpts.maxGitProcs, true)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(reviewOpts.excludes))
	if err := validateReviewRefs(cc.RepoDir, reviewOpts); err != nil {
		return err
	}

	resumeState, err := loadReviewResumeState(cc.RepoDir, reviewOpts)
	if err != nil {
		return err
	}

	// Admission must use the same effective max_tokens as the real run because
	// oversized diffs are excluded against that ceiling before dispatch. Unlike
	// the old headline warning, confirm/abort can block execution, so estimating
	// against a different ceiling can produce a false rejection.
	maxTokens, err := resolveBudgetPreflightMaxTokens(cc.Template.MaxTokens, reviewOpts.maxTokens)
	if err != nil {
		return err
	}
	cc.Template.MaxTokens = maxTokens

	est, err := agent.EstimatePreflight(cmd.Context(), agent.Args{
		RepoDir:    cc.RepoDir,
		From:       reviewOpts.from,
		To:         reviewOpts.to,
		Commit:     reviewOpts.commit,
		ReviewMode: reviewModeFromOptions(reviewOpts),
		Template:   *cc.Template,
		FileFilter: cc.FileFilter,
		GitRunner:  cc.GitRunner,
		Resume:     resumeState,
	})
	if err != nil {
		return fmt.Errorf("budget preflight: %w", err)
	}

	return enforceBudgetPreflight(policy, "review", est.TotalTokens,
		int64(reviewOpts.maxTokensBudget), os.Stdin, os.Stderr, term.IsTerminal(os.Stdin.Fd()))
}

func runScanBudgetPreflight(cmd *cobra.Command, _ []string) error {
	policy, err := normalizeBudgetPreflight(scanBudgetPreflight)
	if err != nil {
		return err
	}
	scanBudgetPreflight = policy

	if scanOpts.preview || policy == budgetPreflightWarn {
		return nil
	}
	if err := validateScanOptions(&scanOpts); err != nil {
		return err
	}

	cc, err := loadCommonContext(scanOpts.repoDir, scanOpts.rulePath, "",
		scanOpts.maxTools, scanOpts.maxGitProcs, false)
	if err != nil {
		return err
	}
	applyCLIExcludes(cc, splitPaths(scanOpts.excludes))

	scanTpl, err := template.LoadScanDefault()
	if err != nil {
		return fmt.Errorf("load scan template: %w", err)
	}
	if err := scanTpl.Validate(); err != nil {
		return fmt.Errorf("invalid scan template: %w", err)
	}
	if scanOpts.maxTools > scanTpl.MaxToolRequestTimes {
		scanTpl.MaxToolRequestTimes = scanOpts.maxTools
	}
	if scanOpts.batch != "" {
		scanTpl.BatchStrategy = scanOpts.batch
	}

	budget := scanTpl.MaxTokensBudget
	if scanOpts.maxTokensBudget > 0 {
		budget = int64(scanOpts.maxTokensBudget)
	}
	if budget <= 0 {
		return fmt.Errorf("--budget-preflight=%s requires --max-tokens-budget to be greater than 0", policy)
	}

	scanPaths := splitPaths(scanOpts.paths)
	resumeState, err := loadScanResumeState(cc.RepoDir, scanOpts, scanPaths)
	if err != nil {
		return err
	}

	// Match the real scan's max_tokens precedence without resolving an LLM
	// endpoint: the estimate is local-only and needs only the app config value.
	maxTokens, err := resolveBudgetPreflightMaxTokens(scanTpl.MaxTokens, scanOpts.maxTokens)
	if err != nil {
		return err
	}
	scanTpl.MaxTokens = maxTokens

	est, err := scan.EstimatePreflight(cmd.Context(), scan.Args{
		RepoDir:          cc.RepoDir,
		Paths:            scanPaths,
		Template:         *scanTpl,
		FileFilter:       cc.FileFilter,
		GitRunner:        cc.GitRunner,
		MaxFileSizeBytes: scanTpl.MaxFileSizeBytes,
		SkipPlan:         scanOpts.noPlan,
		SkipDedup:        scanOpts.noDedup,
		SkipSummary:      scanOpts.noSummary,
		Resume:           resumeState,
	})
	if err != nil {
		return fmt.Errorf("budget preflight: %w", err)
	}

	return enforceBudgetPreflight(policy, "scan", est.TotalTokens, budget,
		os.Stdin, os.Stderr, term.IsTerminal(os.Stdin.Fd()))
}

// enforceBudgetPreflight applies the user-selected admission policy. Confirmation
// deliberately fails closed when stdin is not a terminal so CI can never hang
// waiting for input. A confirmed run still keeps --max-tokens-budget as its
// runtime dispatch guard; confirmation only authorizes starting a run whose
// rough estimate is already above that guard's configured budget.
func enforceBudgetPreflight(policy, operation string, estimated, budget int64,
	in io.Reader, out io.Writer, interactive bool,
) error {
	if budget <= 0 || estimated <= budget {
		return nil
	}

	switch policy {
	case budgetPreflightWarn:
		return nil
	case budgetPreflightAbort:
		return fmt.Errorf("%s budget preflight aborted: estimated usage %s exceeds token budget %s",
			operation, formatBudgetTokens(estimated), formatBudgetTokens(budget))
	case budgetPreflightConfirm:
		if !interactive {
			return fmt.Errorf("%s budget preflight needs interactive stdin: estimated usage %s exceeds token budget %s; use --budget-preflight=abort in CI, or --budget-preflight=warn to keep the existing non-interactive behavior",
				operation, formatBudgetTokens(estimated), formatBudgetTokens(budget))
		}
		fmt.Fprintf(out, "[ocr] estimated %s cost (%s tokens) exceeds --max-tokens-budget (%s); the runtime budget gate will still stop new dispatch when it triggers, while already in-flight work may finish. Start anyway? [y/N] ",
			operation, formatBudgetTokens(estimated), formatBudgetTokens(budget))
		answer, readErr := bufio.NewReader(in).ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read budget confirmation: %w", readErr)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return nil
		default:
			return fmt.Errorf("%s cancelled by budget preflight: estimated usage %s exceeds token budget %s",
				operation, formatBudgetTokens(estimated), formatBudgetTokens(budget))
		}
	default:
		// normalizeBudgetPreflight should make this unreachable; keep the helper
		// defensive for direct tests/callers.
		return fmt.Errorf("invalid budget preflight policy %q", policy)
	}
}

func formatBudgetTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
