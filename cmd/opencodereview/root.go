// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ocr",
	Short: "OpenCodeReview - AI-Powered Code Review CLI",
	Long: `OpenCodeReview - AI-Powered Code Review CLI

An AI-powered code review tool that reads git diffs, sends them to a
configurable LLM service, and generates review comments.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Runs for every subcommand, always before any RunE: validate --color once
	// flags are parsed, then resolve the color decision from them.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := validateColorMode(colorMode); err != nil {
			return err
		}
		colorEnabled = resolveColor()
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		v, _ := cmd.Flags().GetBool("version")
		if v {
			printVersion()
			return nil
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.SetFlagErrorFunc(flagErrorWithSuggestion)
	rootCmd.Flags().BoolP("version", "V", false, "version for ocr")
	addColorFlags(rootCmd)

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if !commandNeedsGit(cmd) {
			return nil
		}
		if err := gitcmd.CheckGitVersion(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		return nil
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(delegateCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(llmCmd)
	rootCmd.AddCommand(rulesCmd)
	rootCmd.AddCommand(viewerCmd)
	rootCmd.AddCommand(completionCmd)
}

func commandNeedsGit(cmd *cobra.Command) bool {
	// `ocr --version` / `-V` is handled by the root command's RunE.
	if v, _ := cmd.Flags().GetBool("version"); v {
		return false
	}
	switch cmd.Name() {
	case "review", "scan", "delegate", "rules":
		return true
	default:
		return false
	}
}

func versionString() string {
	s := fmt.Sprintf("open-code-review %s", Version)
	if GitCommit != "" {
		s += fmt.Sprintf(" (%s)", GitCommit)
	}
	s += fmt.Sprintf(" %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if BuildDate != "" {
		s += fmt.Sprintf("built at: %s\n", BuildDate)
	}
	s += "https://github.com/alibaba/open-code-review\n"
	return s
}
