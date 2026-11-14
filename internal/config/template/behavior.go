// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import (
	"encoding/json"
	"fmt"
	"slices"
)

type BehaviorTemplate struct {
	Version               string
	MainTask              LlmConversation
	MemoryCompressionTask LlmConversation
	FinalRound            LlmConversation
	MaxTokens             int
	MaxCompletionTokens   int
	MaxToolRequestTimes   int
}

func LoadBehaviorDefault() (*BehaviorTemplate, error) {
	data, err := templateFS.ReadFile("behavior_template.json")
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Version             string               `json:"version"`
		Main                manifestConversation `json:"MAIN_TASK"`
		Compression         manifestConversation `json:"MEMORY_COMPRESSION_TASK"`
		Final               manifestConversation `json:"FINAL_ROUND"`
		MaxTokens           int                  `json:"MAX_TOKENS"`
		MaxCompletionTokens int                  `json:"MAX_COMPLETION_TOKENS"`
		MaxRounds           int                  `json:"MAX_TOOL_REQUEST_TIMES"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	result := &BehaviorTemplate{Version: manifest.Version, MaxTokens: manifest.MaxTokens,
		MaxCompletionTokens: manifest.MaxCompletionTokens, MaxToolRequestTimes: manifest.MaxRounds}
	if result.MainTask, err = resolveConversation(manifest.Main); err != nil {
		return nil, err
	}
	if result.MemoryCompressionTask, err = resolveConversation(manifest.Compression); err != nil {
		return nil, err
	}
	if result.FinalRound, err = resolveConversation(manifest.Final); err != nil {
		return nil, err
	}
	return result, result.Validate()
}

func (tpl *BehaviorTemplate) Validate() error {
	if tpl.Version == "" || tpl.MaxTokens <= 0 || tpl.MaxCompletionTokens <= 0 || tpl.MaxToolRequestTimes <= 0 {
		return fmt.Errorf("behavior template requires a version and positive budgets")
	}
	expected := []string{"code_search", "context_read", "file_find", "file_read", "file_read_diff", "submit_analysis"}
	actual := slices.Clone(tpl.MainTask.Tools)
	slices.Sort(actual)
	if !slices.Equal(actual, expected) || !slices.Equal(tpl.FinalRound.Tools, []string{"submit_analysis"}) || len(tpl.MemoryCompressionTask.Tools) != 0 {
		return fmt.Errorf("behavior template has invalid tool bindings")
	}
	for _, conversation := range []LlmConversation{tpl.MainTask, tpl.MemoryCompressionTask, tpl.FinalRound} {
		if len(conversation.Messages) == 0 || len(conversation.ToolReferences) != 0 {
			return fmt.Errorf("behavior conversation requires messages and no tool references")
		}
	}
	return nil
}

func (tpl *BehaviorTemplate) ApplyLanguage(language string) {
	instruction := "\n\nWrite prose in " + resolveLang(language) + ". Keep schema keys, IDs, enum values, and source locations unchanged."
	applyLanguage(&tpl.MainTask, instruction)
	applyLanguage(&tpl.MemoryCompressionTask, instruction)
}
