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
	ctx := context.Background()
	if shouldInitializeTelemetry(os.Args[1:]) && telemetry.Init(ctx) {
		defer telemetry.ShutdownWithTimeout(ctx, 5*time.Second)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// The interactive shell has no review work of its own. Its review subprocess
// initializes telemetry, keeping console exporters off the active TUI screen.
func shouldInitializeTelemetry(args []string) bool {
	cmd, _, err := rootCmd.Find(args)
	return err != nil || cmd.Name() != "tui"
}
