// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"runtime"

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

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(delegateCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(llmCmd)
	rootCmd.AddCommand(rulesCmd)
	rootCmd.AddCommand(aliasCmd)
	rootCmd.AddCommand(viewerCmd)
	rootCmd.AddCommand(completionCmd)
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
