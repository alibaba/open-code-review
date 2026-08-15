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
	llm.InitEmbeddedLoader()

	ctx := context.Background()
	if telemetry.Init(ctx) {
		defer telemetry.ShutdownWithTimeout(ctx, 5*time.Second)
	}

func expandConfiguredAliases(args []string) ([]string, error) {
	configPath, err := defaultConfigPath()
	if err != nil {
		return args, nil
	}
	cfg, err := LoadAppConfig(configPath)
	if err != nil {
		// Log the error so users know their config is broken
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		return args, nil
	}
	if cfg == nil {
		return args, nil
	}
	return expandAliases(args, cfg.Aliases)
}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
