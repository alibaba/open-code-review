// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
)

// openAIStreamChoiceState owns both normalized streaming progress and the
// provider-native assistant message being assembled for replay. Standard
// fields follow the Chat Completions delta contract; provider fields use
// explicit, adapter-scoped merge policies rather than guessed JSON semantics.
type openAIStreamChoiceState struct {
	content          openAITextDeltaState
	contentNull      bool
	refusal          openAITextDeltaState
	reasoningContent openAITextDeltaState
	reasoningDetails []any
	toolCalls        []any
	functionCall     map[string]any
	audio            any
	providerFields   map[string]any
	finished         bool
}

type openAITextDeltaState struct {
	text strings.Builder
	seen bool
}

func (s *openAITextDeltaState) add(value any) {
	text, ok := value.(string)
	if !ok {
		return
	}
	s.seen = true
	s.text.WriteString(text)
}

func (s *openAITextDeltaState) String() string {
	return s.text.String()
}

func (s *openAIStreamChoiceState) addDelta(rawJSON string) error {
	if rawJSON == "" {
		return nil
	}
	delta, err := decodeOpenAIObject(rawJSON)
	if err != nil {
		return fmt.Errorf("decode assistant delta: %w", err)
	}
	for field, value := range delta {
		switch field {
		case "role", "annotations":
			// Role is fixed when the message is finalized. Annotations are
			// response metadata, not valid assistant request fields.
		case "content":
			s.addContentDelta(value)
		case "refusal":
			s.refusal.add(value)
		case "reasoning_content":
			s.reasoningContent.add(value)
		case "reasoning_details":
			if items, ok := value.([]any); ok {
				s.reasoningDetails = mergeOpenAIReasoningDetails(s.reasoningDetails, items)
			}
		case "tool_calls":
			if items, ok := value.([]any); ok {
				s.toolCalls = mergeOpenAIToolCallDeltas(s.toolCalls, items)
			}
		case "function_call":
			if functionCall, ok := value.(map[string]any); ok {
				s.functionCall = mergeOpenAIFunctionCallDelta(s.functionCall, functionCall)
			}
		case "audio":
			s.audio = value
		default:
			if s.providerFields == nil {
				s.providerFields = make(map[string]any)
			}
			// Unknown provider state is treated as an opaque snapshot. Without
			// a provider contract, concatenating or recursively merging it would
			// invent semantics the adapter cannot know.
			s.providerFields[field] = value
		}
	}
	return nil
}

func (s *openAIStreamChoiceState) addContentDelta(value any) {
	switch value := value.(type) {
	case string:
		s.contentNull = false
		s.content.add(value)
	case nil:
		if !s.content.seen {
			s.content.seen = true
			s.contentNull = true
		}
	}
}

func (s *openAIStreamChoiceState) applyNormalizedFields(msg *ResponseMessage) {
	if s.content.seen {
		if s.contentNull {
			msg.Content = nil
		} else {
			content := s.content.String()
			msg.Content = &content
		}
	}
	if s.reasoningContent.seen {
		msg.ReasoningContent = s.reasoningContent.String()
	}
}

func (s *openAIStreamChoiceState) finalize(message openai.ChatCompletionMessage) (json.RawMessage, error) {
	fields := openAIReplayFields{
		providerFields:   s.providerFields,
		contentPresent:   s.content.seen,
		contentNull:      s.contentNull,
		content:          s.content.String(),
		reasoningPresent: s.reasoningContent.seen,
		reasoningContent: s.reasoningContent.String(),
		reasoningDetails: s.reasoningDetails,
		toolCalls:        s.toolCalls,
		functionCall:     s.functionCall,
		audio:            s.audio,
	}
	if s.refusal.seen {
		message.Refusal = s.refusal.String()
	}
	return marshalOpenAIReplayMessage(message.ToAssistantMessageParam(), fields)
}

func mergeOpenAIReasoningDetails(current, delta []any) []any {
	for position, value := range delta {
		incoming, ok := value.(map[string]any)
		if !ok {
			current = append(current, value)
			continue
		}
		targetPosition := openAIIndexedItemPosition(current, incoming, position, false)
		if targetPosition < 0 {
			current = append(current, cloneOpenAIObject(incoming))
			continue
		}
		target, _ := current[targetPosition].(map[string]any)
		for field, fieldValue := range incoming {
			if field == "text" {
				if existing, ok := target[field].(string); ok {
					if text, ok := fieldValue.(string); ok {
						target[field] = existing + text
						continue
					}
				}
			}
			target[field] = fieldValue
		}
	}
	return current
}

func mergeOpenAIToolCallDeltas(current, delta []any) []any {
	for position, value := range delta {
		incoming, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, hasIndex := incoming["index"]; !hasIndex && len(current) > 0 {
			if _, hasID := incoming["id"]; !hasID {
				// A fragment without index or id is a continuation. Chunks
				// carry one fragment each, so its position within the chunk
				// says nothing about which call it belongs to — continue the
				// most recent one.
				mergeOpenAIToolCallFields(asOpenAIObject(current[len(current)-1]), incoming, position)
				continue
			}
		}
		targetPosition := openAIIndexedItemPosition(current, incoming, position, true)
		if targetPosition < 0 {
			incoming = cloneOpenAIObject(incoming)
			incoming["index"] = json.Number(fmt.Sprint(normalizedOpenAIItemIndex(incoming, position, true)))
			current = append(current, incoming)
			continue
		}
		mergeOpenAIToolCallFields(asOpenAIObject(current[targetPosition]), incoming, position)
	}
	return current
}

func mergeOpenAIToolCallFields(target, incoming map[string]any, position int) {
	if target == nil {
		return
	}
	for field, fieldValue := range incoming {
		switch field {
		case "index":
			target[field] = json.Number(fmt.Sprint(normalizedOpenAIItemIndex(incoming, position, true)))
		case "function":
			if function, ok := fieldValue.(map[string]any); ok {
				target[field] = mergeOpenAIFunctionCallDelta(asOpenAIObject(target[field]), function)
			}
		default:
			target[field] = fieldValue
		}
	}
}

func mergeOpenAIFunctionCallDelta(current, delta map[string]any) map[string]any {
	if current == nil {
		current = make(map[string]any)
	}
	for field, value := range delta {
		switch field {
		case "name", "arguments":
			if existing, ok := current[field].(string); ok {
				if text, ok := value.(string); ok {
					current[field] = existing + text
					continue
				}
			}
		}
		current[field] = value
	}
	return current
}

func openAIIndexedItemPosition(items []any, candidate map[string]any, fallback int, clampNegative bool) int {
	wanted := normalizedOpenAIItemIndex(candidate, fallback, clampNegative)
	for position, value := range items {
		item, ok := value.(map[string]any)
		if ok && normalizedOpenAIItemIndex(item, position, clampNegative) == wanted {
			return position
		}
	}
	return -1
}

func normalizedOpenAIItemIndex(item map[string]any, fallback int, clampNegative bool) int64 {
	index, ok := item["index"].(json.Number)
	if !ok {
		return int64(fallback)
	}
	parsed, err := index.Int64()
	if err != nil {
		return int64(fallback)
	}
	if clampNegative && parsed < 0 {
		return 0
	}
	return parsed
}

func asOpenAIObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func cloneOpenAIObject(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source)+1)
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
