// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/alibaba/open-code-review/internal/gate"
	"github.com/spf13/cobra"
)

func newGateCmd() *cobra.Command {
	var input, format string
	var policy gate.Policy
	cmd := &cobra.Command{
		Use:   "gate --input <result.json|-> [flags]",
		Short: "Evaluate a saved review result for CI (no LLM required)",
		Long: `Evaluate coverage, comment delivery, and optional severity and revision checks.
Only pass exits 0. Both fail and inconclusive exit 1 after emitting the decision.
This command reads a trusted OCR JSON artifact; it does not authenticate the
artifact, rerun a review, check the live PR head, or change review behavior.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("--input is required (use - for stdin)")
			}
			if format != "text" && format != "json" {
				return fmt.Errorf("--format must be text or json")
			}
			p, err := policy.Normalize()
			if err != nil {
				return err
			}
			reader := cmd.InOrStdin()
			if input != "-" {
				file, err := os.Open(input)
				if err != nil {
					return fmt.Errorf("read gate input: %w", err)
				}
				defer file.Close()
				reader = file
			}
			result, err := gate.Evaluate(reader, p)
			if err != nil {
				return err
			}
			if err := writeGateResult(cmd.OutOrStdout(), format, result); err != nil {
				return fmt.Errorf("write gate result: %w", err)
			}
			if result.Status != gate.Pass {
				return fmt.Errorf("review gate %s; see gate checks for reasons", result.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "saved ocr review JSON result (- reads stdin)")
	cmd.Flags().StringVar(&policy.FailOnSeverity, "fail-on-severity", "", "block critical, high, medium, or low findings and higher (unset disables severity checks)")
	cmd.Flags().StringVar(&policy.ExpectedBase, "expected-base", "", "require this full resolved base object ID (not a branch name)")
	cmd.Flags().StringVar(&policy.ExpectedHead, "expected-head", "", "require this full reviewed head object ID (not a branch name)")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text or json")
	return cmd
}

func writeGateResult(out io.Writer, format string, result gate.Result) error {
	if format == "json" {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if _, err := fmt.Fprintf(out, "Review gate: %s\n", result.Status); err != nil {
		return err
	}
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(out, "- %s: %s (%s) - %s\n", check.Name, check.Status, check.Code, check.Message); err != nil {
			return err
		}
	}
	return nil
}
