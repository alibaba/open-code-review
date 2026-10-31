// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/storage"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

func main() {
	llm.AppVersion = Version
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	if telemetry.Init(ctx) {
		defer telemetry.ShutdownWithTimeout(ctx, 5*time.Second)
	}

	configureSessionStorage()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// configureSessionStorage installs the object-store backend that session records
// are mirrored to, so that a run's findings survive the machine that produced
// them.
//
// A storage section that cannot be opened disables mirroring rather than failing
// the command: the local cache still receives the session, and every command
// stays usable while the user fixes the configuration. The warning is emitted so
// the missing durable copy is never silent.
func configureSessionStorage() {
	cfg, err := storage.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] warning: object storage disabled: %v\n", err)
		return
	}
	backend, err := storage.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] warning: object storage disabled: %v\n", err)
		return
	}
	session.ConfigureStorage(backend)
}
