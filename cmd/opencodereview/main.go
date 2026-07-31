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

	if err := rootCmd.Execute(); err != nil {
		msg := err.Error()
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		if suggestion := suggestFlagFromError(msg); suggestion != "" {
			fmt.Fprint(os.Stderr, suggestion)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
}
