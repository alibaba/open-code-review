// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/threadverify"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"
)

type verifyThreadsOptions struct {
	inputPath       string
	outputPath      string
	repoDir         string
	provider        string
	model           string
	timeout         int
	maxTokensBudget int
}

var verifyThreadsOpts verifyThreadsOptions

// verifyThreadsCmd is driven by the GitHub Action's publisher, which builds
// the input from threads it has already proven are its own.
var verifyThreadsCmd = &cobra.Command{
	Use:    "verify-threads",
	Short:  "Decide whether earlier review findings are fixed at a head commit",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeVerifyThreads(cmd.Context(), verifyThreadsOpts)
	},
}

func init() {
	f := verifyThreadsCmd.Flags()
	f.StringVar(&verifyThreadsOpts.inputPath, "input", "", "threads JSON written by the publisher (required)")
	f.StringVar(&verifyThreadsOpts.outputPath, "output", "", "verdicts JSON destination (required)")
	f.StringVar(&verifyThreadsOpts.repoDir, "repo-dir", "", "repository directory (default: current directory)")
	f.StringVar(&verifyThreadsOpts.provider, "provider", "", "configured provider to use for this run")
	f.StringVar(&verifyThreadsOpts.model, "model", "", "model to use for this run")
	f.IntVar(&verifyThreadsOpts.timeout, "timeout", 15, "whole-run budget in minutes (0 = unlimited)")
	f.IntVar(&verifyThreadsOpts.maxTokensBudget, "max-tokens-budget", 0, "aggregate token budget for the run (0 = unlimited)")
	rootCmd.AddCommand(verifyThreadsCmd)
}

func executeVerifyThreads(ctx context.Context, opts verifyThreadsOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.inputPath == "" || opts.outputPath == "" {
		return fmt.Errorf("--input and --output are required")
	}
	raw, err := os.ReadFile(opts.inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	var in threadverify.Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("parse input: %w", err)
	}

	cc, err := loadCommonContext(opts.repoDir, "", "", 0, 0, true)
	if err != nil {
		return err
	}
	rt, err := loadLLMRuntime(cc.Template, "", llm.ResolveOptions{Provider: opts.provider, Model: opts.model, Context: ctx})
	if err != nil {
		return err
	}
	maxTokens, err := resolveMaxTokens(cc.Template.MaxTokens, rt.AppCfg, 0)
	if err != nil {
		return err
	}
	cc.Template.MaxTokens = maxTokens

	fr := &tool.FileReader{RepoDir: cc.RepoDir, Mode: tool.ModeCommit, Ref: in.HeadSHA, Runner: cc.GitRunner}
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Register(tool.NewFileFind(fr))
	reg.Register(tool.NewCodeSearch(fr))

	if opts.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.timeout)*time.Minute)
		defer cancel()
	}
	out, err := threadverify.Run(ctx, threadverify.Args{
		LLMClient:       rt.Client,
		Model:           rt.Model,
		Template:        *cc.Template,
		Tools:           reg,
		ToolDefs:        rt.MainToolDefs,
		Session:         session.New(cc.RepoDir, "", rt.Model, session.SessionOptions{ReviewMode: "verify-threads", LLMSource: rt.Source}),
		MaxTokensBudget: int64(opts.maxTokensBudget),
		ReadHead:        fr.Read,
	}, in)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(opts.outputPath, append(data, '\n'), 0o600)
}
