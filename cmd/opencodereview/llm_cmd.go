package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
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
	var lang string
	if appCfg != nil {
		lang = appCfg.Language
	}
	task.ApplyLanguage(lang)

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
			MaxTokens: 2048,
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

	content := resp.Content()
	if content == "" {
		content = "(empty response)"
	}
	fmt.Printf("%s\n", content)
	fmt.Println("✓ Connection test successful")
	return nil
}

func runLLMProviders() {
	providers := llm.ListProviders()
	fmt.Println("\nBuilt-in providers:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  NAME\tPROTOCOL\tBASE URL\n")
	fmt.Fprintf(w, "  ----\t--------\t--------\n")
	for _, p := range providers {
		fmt.Fprintf(w, "  %s\t%s\t%s\n", p.Name, p.Protocol, p.BaseURL)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to flush output: %v\n", err)
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
