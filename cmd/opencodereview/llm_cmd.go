package main

import (
	"context"
	"fmt"
	"time"

	"github.com/open-code-review/open-code-review/internal/config/testconnection"
	"github.com/open-code-review/open-code-review/internal/llm"
)

func runLLM(args []string) error {
	if len(args) == 0 {
		printLLMUsage()
		return nil
	}

	switch args[0] {
	case "test":
		return runLLMTest()
	case "providers":
		runLLMProviders()
		return nil
	default:
		return fmt.Errorf("unknown llm sub-command: %s\nRun 'ocr llm' for usage", args[0])
	}
}

func runLLMTest() error {
	cfgPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	appCfg, err := LoadAppConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ep, err := llm.ResolveEndpoint(cfgPath)
	if err != nil {
		return fmt.Errorf("resolve LLM endpoint: %w", err)
	}

	task, err := testconnection.LoadDefault()
	if err != nil {
		return fmt.Errorf("load test task config: %w", err)
	}
	if appCfg != nil {
		task.ApplyLanguage(appCfg.Language)
	}

	timeout := 30 * time.Second
	if task.Timeout > 0 {
		timeout = time.Duration(task.Timeout) * time.Second
	}

	llmClient := llm.NewLLMClient(ep)

	messages := make([]llm.Message, 0, len(task.Messages))
	for _, m := range task.Messages {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}

	resp, err := func() (*llm.ChatResponse, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return llmClient.CompletionsWithCtx(ctx, llm.ChatRequest{
			Model:     ep.Model,
			Messages:  messages,
			MaxTokens: 256,
		})
	}()
	if err != nil {
		return fmt.Errorf("llm request failed: %w", err)
	}

	model := ep.Model
	if resp.Model != "" {
		model = resp.Model
	}
	fmt.Printf("Source: %s\n", ep.Source)
	fmt.Printf("URL:    %s\n", ep.URL)
	fmt.Printf("Model:  %s\n", model)
	fmt.Printf("%s\n", resp.Content())
	return nil
}

func runLLMProviders() {
	providers := llm.ListProviders()
	fmt.Println("\nBuilt-in providers:")
	fmt.Printf("  %-14s %-10s %s\n", "NAME", "PROTOCOL", "BASE URL")
	fmt.Printf("  %-14s %-10s %s\n", "----", "--------", "--------")
	for _, p := range providers {
		fmt.Printf("  %-14s %-10s %s\n", p.Name, p.Protocol, p.BaseURL)
	}
	fmt.Println("\nUse 'ocr config provider' to configure a provider interactively.")
	fmt.Println("Use 'ocr config set provider <name>' to switch providers non-interactively.")
}

func printLLMUsage() {
	fmt.Println(`LLM utility commands.

Usage:
  ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers

Examples:
  ocr llm test                   Verify LLM connectivity and configuration
  ocr llm providers              List available built-in providers`)
}
