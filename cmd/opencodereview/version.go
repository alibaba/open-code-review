// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set via ldflags: -X main.Version=x.y.z -X main.GitCommit=abc123 -X main.BuildDate=2026-01-01T00:00:00Z
var (
	Version   = "dev"
	GitCommit = ""
	BuildDate = ""
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func printVersion() {
	fmt.Print(versionString())
}
