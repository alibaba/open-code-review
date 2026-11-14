// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"encoding/json"
	"fmt"
)

func (runner *Runner) readContext(args map[string]any) (Observation, error) {
	kind, _ := args["kind"].(string)
	id, _ := args["id"].(string)
	observation := Observation{ToolName: "context_read", Arguments: args, SourceKind: kind, Locations: []Location{}}
	var content string
	if kind == "observation" {
		previous, exists := runner.store.get(id)
		if !exists {
			observation.ErrorClass = "unavailable"
			return observation, fmt.Errorf("observation ID is unavailable")
		}
		data, err := json.Marshal(previous)
		if err != nil {
			return observation, err
		}
		content = string(data)
	} else {
		for _, captured := range runner.contexts {
			if captured.Kind == kind && captured.ID == id {
				content = captured.Content
				captured.Content = ""
				observation.Context = &captured
				break
			}
		}
		if observation.Context == nil {
			observation.ErrorClass = "unavailable"
			return observation, fmt.Errorf("context kind and ID are unavailable; no external fetch is permitted")
		}
	}
	cursor, _ := args["cursor"].(float64)
	maxBytes := 16384
	if specified, ok := args["max_bytes"].(float64); ok {
		maxBytes = int(specified)
	}
	page, next, err := readPage(content, int(cursor), maxBytes)
	if err != nil {
		return observation, err
	}
	observation.ReturnedContent = page
	observation.Continuation = next
	observation.Truncated = next != ""
	return observation, nil
}
