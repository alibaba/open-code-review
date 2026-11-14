// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/spf13/cobra"
)

func newConfigShowCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:         "show",
		Short:       "Inspect non-secret LLM configuration without acquiring credentials",
		Long:        "Print offline LLM configuration metadata as JSON. Credential commands, OAuth refresh and model discovery are never executed. URLs, headers, request bodies and credential values are omitted.",
		Example:     "  ocr config show --format json",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"offline": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "json" {
				return fmt.Errorf("unsupported output format; use json")
			}
			path, err := defaultConfigPath()
			if err != nil {
				return err
			}
			inspection, err := llm.InspectEndpoint(path)
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(inspection)
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "Output format (json)")
	return cmd
}
