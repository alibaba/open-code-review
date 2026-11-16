// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

func main() {
	llm.AppVersion = Version
	os.Exit(run())
}

func run() int {
	return runArgs(os.Args[1:])
}

func runArgs(args []string) int {
	rootCmd.SetArgs(args)
	ctx := context.Background()
	cmd, _, err := rootCmd.Find(args)
	// IDEs consume this query as a protocol, so even console telemetry must stay silent.
	pathQuery := err == nil && cmd.Parent() == configCmd && cmd.Name() == "path"
	if !pathQuery && telemetry.Init(ctx) {
		defer telemetry.ShutdownWithTimeout(ctx, 5*time.Second)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
