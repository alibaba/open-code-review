// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"encoding/json"
	"fmt"

	openai "github.com/openai/openai-go/v3"
)

// openAIReplayFields are the response details that the official SDK request
// conversion cannot retain: exact content presence and provider extensions.
type openAIReplayFields struct {
	providerFields   map[string]any
	contentPresent   bool
	contentNull      bool
	content          string
	reasoningPresent bool
	reasoningContent string
	reasoningDetails []any
	toolCalls        []any
	functionCall     map[string]any
	audio            any
}

// openAIReplayExtensionAllowlist names the non-standard top-level response
// fields with a known request-side replay contract. Every other unknown
// field is treated as response metadata and never echoed back: providers can
// reject fields they did not define for input.
var openAIReplayExtensionAllowlist = map[string]bool{
	"reasoning_details": true, // OpenRouter reasoning block preservation
}

// providersRejectingReasoningReplay are OpenAI-protocol providers that
// document a hard error when reasoning_content appears in input messages.
var providersRejectingReasoningReplay = map[string]bool{
	"deepseek": true,
}

// openAIReplayPolicy captures per-provider replay quirks that change what a
// valid continuation request may carry.
type openAIReplayPolicy struct {
	replayReasoningContent bool
}

func (c *OpenAIClient) replayPolicy() openAIReplayPolicy {
	return openAIReplayPolicy{
		replayReasoningContent: !providersRejectingReasoningReplay[c.cfg.Provider],
	}
}

func (c *OpenAIClient) nativeReplayEnabled() bool {
	return c.cfg.AssistantReplay == AssistantReplayNative
}

func openAIReplayMessageFromResponse(message openai.ChatCompletionMessage, policy openAIReplayPolicy) (json.RawMessage, error) {
	response, err := decodeOpenAIObject(message.RawJSON())
	if err != nil {
		return nil, err
	}
	fields := openAIReplayFields{providerFields: make(map[string]any)}
	for field, value := range response {
		switch field {
		case "role", "refusal", "annotations", "function_call":
			// The SDK converts request-safe standard fields. Annotations are
			// response metadata and deliberately have no request equivalent.
		case "content":
			fields.contentPresent = true
			fields.contentNull = value == nil
			fields.content, _ = value.(string)
		case "reasoning_content":
			fields.reasoningPresent = true
			fields.reasoningContent, _ = value.(string)
		case "audio":
			fields.audio = value
		case "tool_calls":
			fields.toolCalls, _ = value.([]any)
		default:
			fields.providerFields[field] = value
		}
	}
	return marshalOpenAIReplayMessage(message.ToAssistantMessageParam(), fields, policy)
}

func marshalOpenAIReplayMessage(param openai.ChatCompletionAssistantMessageParam, fields openAIReplayFields, policy openAIReplayPolicy) (json.RawMessage, error) {
	rawParam, err := json.Marshal(param)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAI assistant request message: %w", err)
	}
	request, err := decodeOpenAIObject(string(rawParam))
	if err != nil {
		return nil, err
	}
	for field, value := range fields.providerFields {
		if !openAIReplayExtensionAllowlist[field] {
			continue
		}
		request[field] = value
	}
	if fields.contentPresent {
		if fields.contentNull {
			request["content"] = nil
		} else {
			request["content"] = fields.content
		}
	}
	if fields.reasoningPresent && policy.replayReasoningContent {
		request["reasoning_content"] = fields.reasoningContent
	}
	if len(fields.reasoningDetails) > 0 {
		request["reasoning_details"] = fields.reasoningDetails
	}
	if len(fields.toolCalls) > 0 {
		request["tool_calls"] = overlayOpenAIToolCallExtensions(request["tool_calls"], fields.toolCalls)
	}
	if len(fields.functionCall) > 0 {
		request["function_call"] = overlayOpenAIFunctionReplay(request["function_call"], fields.functionCall)
	}
	if audio := openAIRequestAudio(fields.audio); audio != nil {
		request["audio"] = audio
	}
	request["role"] = "assistant"
	return marshalOpenAIRawObject(request)
}

func overlayOpenAIToolCallExtensions(requestValue any, responseItems []any) []any {
	requestItems, _ := requestValue.([]any)
	for position, value := range requestItems {
		responsePosition := openAIIndexedItemPosition(responseItems, map[string]any{"index": json.Number(fmt.Sprint(position))}, position, true)
		var responseItem map[string]any
		if responsePosition >= 0 {
			responseItem, _ = responseItems[responsePosition].(map[string]any)
		}
		requestItem, ok := value.(map[string]any)
		if !ok {
			// The SDK's typed union produced no request variant — gateways
			// that omit the tool call "type" field trigger this. Rebuild the
			// item from the provider's own response JSON rather than replay
			// a null.
			if rebuilt := rebuildOpenAIToolCallFromResponse(responseItem); rebuilt != nil {
				requestItems[position] = rebuilt
			}
			continue
		}
		for field, fieldValue := range responseItem {
			switch field {
			case "index", "id", "type":
			case "function":
				requestItem[field] = overlayOpenAIFunctionReplay(requestItem[field], asOpenAIObject(fieldValue))
			case "custom":
				requestItem[field] = overlayOpenAICustomToolExtensions(requestItem[field], asOpenAIObject(fieldValue))
			default:
				requestItem[field] = fieldValue
			}
		}
	}
	return requestItems
}

// rebuildOpenAIToolCallFromResponse reconstructs a request tool call item
// verbatim from the provider's response JSON, defaulting the "type"
// discriminator when the provider omitted it.
func rebuildOpenAIToolCallFromResponse(responseItem map[string]any) map[string]any {
	if responseItem == nil {
		return nil
	}
	rebuilt := cloneOpenAIObject(responseItem)
	delete(rebuilt, "index")
	if _, ok := rebuilt["type"].(string); !ok {
		rebuilt["type"] = "function"
	}
	return rebuilt
}

func overlayOpenAIFunctionReplay(requestValue any, response map[string]any) map[string]any {
	request := cloneOpenAIObject(asOpenAIObject(requestValue))
	for field, value := range response {
		if field == "name" || field == "arguments" {
			// The SDK owns canonical tool-call assembly. Its accumulator does not
			// retain the deprecated streaming function_call field, however, so use
			// the adapter's accumulated standard value only when the typed request
			// conversion did not provide one.
			if _, present := request[field]; present {
				continue
			}
		}
		request[field] = value
	}
	return request
}

func overlayOpenAICustomToolExtensions(requestValue any, response map[string]any) map[string]any {
	request := cloneOpenAIObject(asOpenAIObject(requestValue))
	for field, value := range response {
		if field != "name" && field != "input" {
			request[field] = value
		}
	}
	return request
}

func openAIRequestAudio(value any) map[string]any {
	audio, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	id, _ := audio["id"].(string)
	if id == "" {
		return nil
	}
	return map[string]any{"id": id}
}

// openAIReplayToolCallsAreValid reports whether every tool call in the
// envelope can be replayed verbatim: an object carrying its id and type
// discriminator. An envelope failing this must be discarded in favor of the
// normalized rebuild — sending it would fail the whole next request.
func openAIReplayToolCallsAreValid(rawMessage json.RawMessage) bool {
	object, err := decodeOpenAIObject(string(rawMessage))
	if err != nil {
		return false
	}
	value, present := object["tool_calls"]
	if !present {
		return true
	}
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, itemValue := range items {
		item, ok := itemValue.(map[string]any)
		if !ok {
			return false
		}
		if id, ok := item["id"].(string); !ok || id == "" {
			return false
		}
		if kind, ok := item["type"].(string); !ok || kind == "" {
			return false
		}
	}
	return true
}

func decodeOpenAIObject(raw string) (map[string]any, error) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("decode OpenAI message JSON: %w", err)
	}
	return object, nil
}

func marshalOpenAIRawObject(object map[string]any) (json.RawMessage, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimSpace(buffer.Bytes())), nil
}
