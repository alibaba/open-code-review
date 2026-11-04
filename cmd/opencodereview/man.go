// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var manCmd = &cobra.Command{
	Use:    "man <directory>",
	Short:  "Generate manual pages",
	Hidden: true,
	Args:   exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := args[0]
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		header := &doc.GenManHeader{
			Title:   "OCR",
			Section: "1",
			Source:  "open-code-review",
			Manual:  "OpenCodeReview Manual",
		}
		return doc.GenManTree(rootCmd, header, dir)
	},
}
