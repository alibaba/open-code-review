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

var runLLMTestPath = runLLMTestWithConfigPath

func runLLMTest() error {
	cfgPath, err := defaultConfigPath()
	if err != nil {
		return err
	}
	return runLLMTestPath(cfgPath)
}

func runLLMTestWithConfigPath(configPath string) error {
	appCfg, err := LoadAppConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ep, err := llm.ResolveEndpoint(configPath)
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
	llmClient := llm.NewLLMClient(ep, nil, nil)

	messages := make([]llm.Message, 0, len(task.Messages))
	for _, m := range task.Messages {
		messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
	}

	tools := testToolDefs(task.Tool)

	// Each request gets the configured budget, as it did when this test made only
	// one. A second turn inheriting an exhausted deadline would fail exactly the
	// way a provider rejecting that turn does, which is the distinction the tool
	// round trip exists to draw.
	send := func(msgs []llm.Message) (*llm.ChatResponse, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return llmClient.CompletionsWithCtx(ctx, llm.ChatRequest{
			Model:     ep.Model,
			Messages:  msgs,
			Tools:     tools,
			MaxTokens: 2048,
		})
	}

	resp, err := send(messages)
	if err != nil {
		return fmt.Errorf("llm request failed: %w", err)
	}

	// A single request never exercises the turn that follows a tool call, which
	// is where providers with their own tool-call metadata reject the
	// conversation (#1357). Replay the turn so the test covers it, answering
	// every call the model made rather than only the self-test one.
	var toolCalled bool
	if task.Tool != nil {
		if findTestToolCall(resp, task.Tool.Name) != nil {
			toolCalled = true
			messages = append(messages, llm.NewToolCallMessage(resp.VisibleContent(), resp.ToolCalls(), resp.Native(), resp.ReasoningContent()))
			messages = append(messages, toolResultMessages(resp, task.Tool)...)
			if resp, err = send(messages); err != nil {
				return fmt.Errorf("llm request after tool call failed: %w", err)
			}
		}
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
	if note := toolRoundTripNote(len(tools) > 0, toolCalled); note != "" {
		fmt.Println(note)
	}
	return nil
}

// testToolDefs offers the configured self-test tool, or none when the task
// defines no tool.
func testToolDefs(spec *testconnection.ToolSpec) []llm.ToolDef {
	if spec == nil || spec.Name == "" {
		return nil
	}
	return []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  spec.Parameters,
		},
	}}
}

// findTestToolCall returns the model's call to the self-test tool, ignoring any
// other tool it asked for. An empty name belongs to a task that offers no tool
// and matches nothing, so an unnamed tool call cannot stand in for one.
func findTestToolCall(resp *llm.ChatResponse, name string) *llm.ToolCall {
	if name == "" {
		return nil
	}
	for _, tc := range resp.ToolCalls() {
		if tc.Function.Name == name {
			return &tc
		}
	}
	return nil
}

// unofferedToolResult answers a tool the test never offered. It reports the
// call as not executed rather than inventing an outcome, and it has to say
// something: an empty result is itself rejected by some providers.
const unofferedToolResult = "Error: this tool is not available in ocr llm test and was not executed."

// toolResultMessages answers every tool call in the turn. Both supported
// protocols reject an assistant turn whose tool calls are not all answered, so
// leaving one out would fail the next request for a reason unrelated to how the
// provider handles tool-call metadata — the only thing this test is measuring.
func toolResultMessages(resp *llm.ChatResponse, spec *testconnection.ToolSpec) []llm.Message {
	calls := resp.ToolCalls()
	out := make([]llm.Message, 0, len(calls))
	for _, tc := range calls {
		result := unofferedToolResult
		if spec != nil && tc.Function.Name == spec.Name {
			result = spec.Result
		}
		out = append(out, llm.NewToolResultMessage(tc.ID, result))
	}
	return out
}

// toolRoundTripNote reports what the round trip proved. A provider that never
// calls the tool leaves it unproven, and saying so is the point: a bare success
// line is what let an endpoint that rejects every post-tool-call request look
// healthy here while failing every review (#1357).
func toolRoundTripNote(offered, called bool) string {
	switch {
	case !offered:
		return ""
	case called:
		return "✓ Tool-call round trip verified"
	default:
		return "! Tool-call round trip unverified: the model did not call the test tool"
	}
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
