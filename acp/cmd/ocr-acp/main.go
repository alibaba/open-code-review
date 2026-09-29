// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/adapter"
	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/llmresolve"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
)

func main() {
	binary := flag.String("ocr-binary", "", "OCR executable path")
	provider := flag.String("parser-provider", "", "Parser LLM provider")
	model := flag.String("parser-model", "", "Parser LLM model")
	baseURL := flag.String("parser-base-url", "", "Parser LLM base URL")
	turnTimeout := flag.Duration("turn-timeout", 0, "Maximum duration of a turn including parsing (zero disables)")
	flag.Parse()
	if *turnTimeout < 0 {
		fmt.Fprintln(os.Stderr, "turn-timeout must not be negative")
		os.Exit(1)
	}
	resolved, err := orchestrator.ResolveBinary(orchestrator.ResolveOptions{Explicit: *binary})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	parser, err := configuredParser(llmresolve.Options{Provider: *provider, Model: *model, BaseURL: *baseURL})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	agent := adapter.NewAgent(resolved.Path, orchestrator.NewRunner(resolved.Path), parser)
	agent.TurnTimeout = *turnTimeout
	output, err := inheritedPipe(os.Stdout, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer output.Close()
	input, err := inheritedPipe(os.Stdin, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer input.Close()
	conn := adapter.NewConnection(agent, output, input)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-conn.Done():
	case <-ctx.Done():
		stop()
	}
	cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := agent.Shutdown(cleanup); err != nil {
		fmt.Fprintln(os.Stderr, "ACP shutdown timed out")
		os.Exit(1)
	}
}

func configuredParser(opts llmresolve.Options) (*intent.Parser, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	configured := opts.Provider != "" || opts.Model != "" || opts.BaseURL != "" || opts.APIKey != ""
	for _, key := range []string{llmresolve.EnvProvider, llmresolve.EnvModel, llmresolve.EnvBaseURL, llmresolve.EnvAPIKey} {
		configured = configured || getenv(key) != ""
	}
	if !configured {
		return nil, nil
	}
	ep, err := llmresolve.Resolve(opts)
	if err != nil {
		return nil, fmt.Errorf("invalid parser configuration: check parser-provider (openai or anthropic), parser-model, parser-base-url and OCR_ACP_PARSER_API_KEY: %w", err)
	}
	client, err := llmresolve.NewClient(ep)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize configured parser client: %w", err)
	}
	return intent.NewParser(client, nil), nil
}
