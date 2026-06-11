// OpenCodeReview is an AI-powered code review CLI tool.
// It reads git diffs, sends them to a configurable LLM service, and generates review comments.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/telemetry"
)

func main() {
	llm.AppVersion = Version
	llm.InitEmbeddedLoader()

	ctx := context.Background()
	if telemetry.Init(ctx) {
		defer telemetry.ShutdownWithTimeout(ctx, 5*time.Second)
	}

	if err := dispatch(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// dispatch routes top-level subcommands or global flags.
func dispatch() error {
	args := os.Args[1:]

	// No args → default to review with empty args (will trigger usage/help)
	if len(args) == 0 {
		printTopLevelUsage()
		return nil
	}

	switch args[0] {
	case "--version", "-V":
		printVersion()
		return nil
	case "version":
		printVersion()
		return nil
	case "review", "r":
		return runReview(args[1:])
	case "config":
		return runConfig(args[1:])
	case "llm":
		return runLLM(args[1:])
	case "rules":
		return runRules(args[1:])
	case "viewer":
		return runViewer(args[1:])
	case "publish":
		return runPublish(args[1:])
	case "-h", "--help":
		printTopLevelUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s\nRun 'ocr' for usage", args[0])
	}
}

func printTopLevelUsage() {
	fmt.Println(`OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  publish      Post review results to a code host (gitflic)
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.

GitHub: https://github.com/alibaba/open-code-review`)
}
