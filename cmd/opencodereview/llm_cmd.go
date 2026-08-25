// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/alibaba/open-code-review/internal/config/testconnection"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/spf13/cobra"
)

var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "LLM utility commands",
	Long:  "LLM utility commands.",
	Example: `  ocr llm test                   Verify LLM connectivity and configuration
  ocr llm providers              List available built-in providers`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var llmTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a test conversation to the configured LLM model",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLLMTest()
	},
}

var llmProvidersCmd = &cobra.Command{
	Use:   "providers",
	Short: "List all built-in LLM providers",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runLLMProviders()
	},
}

func init() {
	llmCmd.AddCommand(llmTestCmd)
	llmCmd.AddCommand(llmProvidersCmd)
}

func runLLMTest() error {
	cfgPath, err := resolveConfigPath()
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

	// No retry collector: llm test is a connectivity probe, not a review, and the
	// retry report only describes ocr review.
	llmClient := llm.NewLLMClient(ep, nil)

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
	if region, profile, ok := bedrockContext(llmClient); ok {
		// Bedrock has no configured URL — the region decides the host — so
		// report what was resolved instead. A request that reached the wrong
		// region otherwise fails in a way that looks like a bad model ID.
		fmt.Printf("Region:  %s\n", region)
		if profile != "" {
			fmt.Printf("Profile: %s\n", profile)
		} else {
			fmt.Printf("Profile: (from the ambient AWS chain)\n")
		}
	} else {
		fmt.Printf("URL:    %s\n", ep.URL)
	}
	fmt.Printf("Model:  %s\n", model)

	content := resp.Content()
	if content == "" {
		content = "(empty response)"
	}
	fmt.Printf("%s\n", content)
	fmt.Println("✓ Connection test successful")
	return nil
}

// bedrockContext reports the region and profile a Bedrock client resolved.
// ok is false for every other client, which keeps the test output unchanged for
// URL-based providers.
func bedrockContext(client llm.LLMClient) (region, profile string, ok bool) {
	c, isAnthropic := client.(*llm.AnthropicClient)
	if !isAnthropic {
		return "", "", false
	}
	return c.BedrockContext()
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
