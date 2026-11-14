// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type Context struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Version         string `json:"version"`
	CollectedAt     string `json:"collected_at"`
	Source          string `json:"source"`
	Revision        string `json:"revision,omitempty"`
	Environment     string `json:"environment,omitempty"`
	ExecutionStatus string `json:"execution_status,omitempty"`
	Content         string `json:"content,omitempty"`
	ContentHash     string `json:"content_hash,omitempty"`
}

type Observation struct {
	ID              string         `json:"observation_id"`
	ToolName        string         `json:"tool_name"`
	Arguments       map[string]any `json:"normalized_arguments"`
	ComparisonID    string         `json:"comparison_id"`
	SourceKind      string         `json:"source_kind"`
	Side            string         `json:"side,omitempty"`
	Revision        string         `json:"revision,omitempty"`
	Locations       []Location     `json:"locations"`
	Context         *Context       `json:"context,omitempty"`
	ContentHash     string         `json:"content_hash"`
	ReturnedContent string         `json:"returned_content"`
	Truncated       bool           `json:"truncated"`
	Continuation    string         `json:"continuation,omitempty"`
	ErrorClass      string         `json:"error_class,omitempty"`
}

type observationStore struct {
	path         string
	comparisonID string
	items        []Observation
}

func newObservationStore(directory, comparisonID string) (*observationStore, error) {
	file := filepath.Join(directory, "observations.jsonl")
	handle, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if err := handle.Close(); err != nil {
		return nil, err
	}
	return &observationStore{path: file, comparisonID: comparisonID}, nil
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func (store *observationStore) append(observation Observation) (string, error) {
	observation.ID = fmt.Sprintf("obs-%06d", len(store.items)+1)
	observation.ComparisonID = store.comparisonID
	observation.ContentHash = contentHash(observation.ReturnedContent)
	observation.Locations = compactLocations(observation.Locations)
	data, err := json.Marshal(observation)
	if err != nil {
		return "", err
	}
	handle, err := os.OpenFile(store.path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := handle.Write(append(data, '\n'))
	closeErr := handle.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	store.items = append(store.items, observation)
	return string(data), nil
}

func compactLocations(locations []Location) []Location {
	sort.Slice(locations, func(first, second int) bool {
		left, right := locations[first], locations[second]
		if left.Side != right.Side {
			return left.Side < right.Side
		}
		if left.Revision != right.Revision {
			return left.Revision < right.Revision
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.StartLine < right.StartLine
	})
	result := []Location{}
	for _, location := range locations {
		if len(result) > 0 {
			previous := &result[len(result)-1]
			if previous.Side == location.Side && previous.Revision == location.Revision && previous.Path == location.Path && location.StartLine <= previous.EndLine+1 {
				previous.EndLine = max(previous.EndLine, location.EndLine)
				continue
			}
		}
		result = append(result, location)
	}
	return result
}

func (store *observationStore) get(id string) (Observation, bool) {
	for _, observation := range store.items {
		if observation.ID == id {
			return observation, true
		}
	}
	return Observation{}, false
}

func validContextKind(kind string) bool {
	switch kind {
	case "background", "issue", "constraint", "verification", "dependency":
		return true
	default:
		return false
	}
}

func readPage(content string, cursor, maxBytes int) (string, string, error) {
	if cursor < 0 || cursor > len(content) || (cursor < len(content) && !utf8.RuneStart(content[cursor])) {
		return "", "", fmt.Errorf("cursor must be a UTF-8 byte boundary within the content")
	}
	if maxBytes < 4 || maxBytes > 65536 {
		return "", "", fmt.Errorf("max_bytes must be between 4 and 65536")
	}
	end := min(len(content), cursor+maxBytes)
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end--
	}
	next := ""
	if end < len(content) {
		next = fmt.Sprint(end)
	}
	return content[cursor:end], next, nil
}

func contextIndex(contexts []Context) []Context {
	result := make([]Context, len(contexts))
	for index, captured := range contexts {
		captured.Content = ""
		result[index] = captured
	}
	return result
}

func validateContexts(contexts []Context) error {
	seen := map[string]bool{}
	for _, captured := range contexts {
		if !validContextKind(captured.Kind) || strings.TrimSpace(captured.ID) == "" || captured.Source == "" || seen[captured.Kind+":"+captured.ID] {
			return fmt.Errorf("context requires a supported kind, unique ID, and source")
		}
		if !utf8.ValidString(captured.Content) {
			return fmt.Errorf("context must contain UTF-8 text")
		}
		seen[captured.Kind+":"+captured.ID] = true
	}
	return nil
}
