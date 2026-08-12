// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	findingStateRelativePath = ".scratch/finding-counts.json"
	findingIDVersion         = "ocr:finding:v1"
	findingStatusActive      = "active_for_agent"
	findingStatusDeferred    = "deferred_for_human"
)

type findingCounterFile struct {
	BaseSHA            string                         `json:"base_sha"`
	ProcessedReviewIDs []string                       `json:"processed_review_ids"`
	Findings           map[string]findingCounterEntry `json:"findings"`
}

type findingCounterEntry struct {
	ConsecutiveReviewCount int    `json:"consecutive_review_count"`
	AutomationStatus       string `json:"automation_status"`
}

var findingStateMu sync.Mutex

// annotateAndRecordFindings adds adapter-owned finding metadata to a complete
// native OCR result and updates the worktree-local consecutive-review state.
// Partial, failed, cancelled, and skipped results are returned byte-for-byte
// unchanged by the caller and must never reach this function.
func annotateAndRecordFindings(repoDir, baseSHA string, raw []byte) ([]byte, error) {
	result, ok := resultObject(raw)
	if !ok {
		return nil, errors.New("complete OCR result is not a JSON object")
	}
	sessionID := resultSessionID(result)
	if sessionID == "" {
		return nil, errors.New("complete OCR result is missing session_id")
	}

	var comments []map[string]json.RawMessage
	commentsPresent := false
	if rawComments, exists := result["comments"]; exists {
		commentsPresent = true
		if err := json.Unmarshal(rawComments, &comments); err != nil {
			return nil, fmt.Errorf("complete OCR result has invalid comments: %w", err)
		}
	}
	if comments == nil {
		comments = []map[string]json.RawMessage{}
	}

	findingStateMu.Lock()
	defer findingStateMu.Unlock()

	state, exists, err := loadFindingCounter(repoDir)
	if err != nil {
		return nil, err
	}
	if exists && !strings.EqualFold(state.BaseSHA, baseSHA) {
		return nil, fmt.Errorf("finding counter base_sha %q does not match review base %q", state.BaseSHA, baseSHA)
	}
	if !exists {
		state = findingCounterFile{
			BaseSHA:            baseSHA,
			ProcessedReviewIDs: []string{},
			Findings:           map[string]findingCounterEntry{},
		}
	}
	if state.ProcessedReviewIDs == nil {
		state.ProcessedReviewIDs = []string{}
	}
	if state.Findings == nil {
		state.Findings = map[string]findingCounterEntry{}
	}

	processed := containsString(state.ProcessedReviewIDs, sessionID)
	currentIDs := make([]string, 0, len(comments))
	currentSet := make(map[string]struct{}, len(comments))
	for _, comment := range comments {
		id := findingIDForRawComment(baseSHA, comment)
		if _, seen := currentSet[id]; !seen {
			currentIDs = append(currentIDs, id)
			currentSet[id] = struct{}{}
		}
	}

	if !processed {
		for id := range state.Findings {
			if _, present := currentSet[id]; !present {
				delete(state.Findings, id)
			}
		}
		for _, id := range currentIDs {
			entry := state.Findings[id]
			if entry.ConsecutiveReviewCount < 3 {
				entry.ConsecutiveReviewCount++
			}
			if entry.ConsecutiveReviewCount >= 3 {
				entry.AutomationStatus = findingStatusDeferred
			} else {
				entry.AutomationStatus = findingStatusActive
			}
			state.Findings[id] = entry
		}
		state.ProcessedReviewIDs = append(state.ProcessedReviewIDs, sessionID)
	}

	for _, comment := range comments {
		id := findingIDForRawComment(baseSHA, comment)
		entry := state.Findings[id]
		if entry.AutomationStatus == "" {
			entry.AutomationStatus = findingStatusActive
		}
		comment["finding_id"] = mustJSON(id)
		comment["consecutive_review_count"] = mustJSON(entry.ConsecutiveReviewCount)
		comment["automation_status"] = mustJSON(entry.AutomationStatus)
	}
	if commentsPresent {
		result["comments"], err = json.Marshal(comments)
		if err != nil {
			return nil, fmt.Errorf("encode annotated comments: %w", err)
		}
	}
	annotated, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode annotated OCR result: %w", err)
	}
	if err := writeFindingCounter(repoDir, state); err != nil {
		return nil, err
	}
	return annotated, nil
}

func findingCounterPath(repoDir string) string {
	return filepath.Join(repoDir, filepath.FromSlash(findingStateRelativePath))
}

func loadFindingCounter(repoDir string) (findingCounterFile, bool, error) {
	raw, err := os.ReadFile(findingCounterPath(repoDir))
	if errors.Is(err, os.ErrNotExist) {
		return findingCounterFile{}, false, nil
	}
	if err != nil {
		return findingCounterFile{}, false, fmt.Errorf("read %s: %w", findingStateRelativePath, err)
	}
	var state findingCounterFile
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return findingCounterFile{}, false, fmt.Errorf("invalid %s: %w", findingStateRelativePath, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return findingCounterFile{}, false, fmt.Errorf("invalid %s: multiple JSON values", findingStateRelativePath)
	} else if !errors.Is(err, io.EOF) {
		return findingCounterFile{}, false, fmt.Errorf("invalid %s: %w", findingStateRelativePath, err)
	}
	if err := validateFindingCounter(state); err != nil {
		return findingCounterFile{}, false, fmt.Errorf("invalid %s: %w", findingStateRelativePath, err)
	}
	return state, true, nil
}

func validateFindingCounter(state findingCounterFile) error {
	if len(state.BaseSHA) != 40 {
		return errors.New("base_sha must be a full 40-character commit SHA")
	}
	if _, err := hex.DecodeString(state.BaseSHA); err != nil {
		return errors.New("base_sha must contain only hexadecimal characters")
	}
	seen := make(map[string]struct{}, len(state.ProcessedReviewIDs))
	for _, id := range state.ProcessedReviewIDs {
		if id == "" {
			return errors.New("processed_review_ids cannot contain empty IDs")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("processed_review_ids contains duplicate %q", id)
		}
		seen[id] = struct{}{}
	}
	for id, entry := range state.Findings {
		if id == "" {
			return errors.New("findings cannot contain an empty ID")
		}
		if entry.ConsecutiveReviewCount < 0 {
			return fmt.Errorf("finding %q has a negative count", id)
		}
		if entry.AutomationStatus != findingStatusActive && entry.AutomationStatus != findingStatusDeferred {
			return fmt.Errorf("finding %q has invalid automation_status %q", id, entry.AutomationStatus)
		}
	}
	return nil
}

func writeFindingCounter(repoDir string, state findingCounterFile) error {
	scratchDir := filepath.Dir(findingCounterPath(repoDir))
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		return fmt.Errorf("create .scratch: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", findingStateRelativePath, err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(scratchDir, ".finding-counts-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary finding counter: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("set finding counter permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary finding counter: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary finding counter: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary finding counter: %w", err)
	}
	if err := os.Rename(tmpName, findingCounterPath(repoDir)); err != nil {
		return fmt.Errorf("replace %s: %w", findingStateRelativePath, err)
	}
	keep = true
	dir, err := os.Open(scratchDir)
	if err != nil {
		return fmt.Errorf("open .scratch directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync .scratch directory: %w", err)
	}
	return nil
}

func findingIDForRawComment(baseSHA string, comment map[string]json.RawMessage) string {
	identity := struct {
		Version   string `json:"version"`
		BaseSHA   string `json:"base_sha"`
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Category  string `json:"category"`
		Severity  string `json:"severity"`
		Content   string `json:"content"`
	}{
		Version:   findingIDVersion,
		BaseSHA:   baseSHA,
		Path:      normalizeFindingPath(rawString(comment, "path")),
		StartLine: rawInt(comment, "start_line"),
		EndLine:   rawInt(comment, "end_line"),
		Category:  strings.ToLower(strings.TrimSpace(rawString(comment, "category"))),
		Severity:  strings.ToLower(strings.TrimSpace(rawString(comment, "severity"))),
		Content:   normalizeFindingContent(rawString(comment, "content")),
	}
	canonical, _ := json.Marshal(identity)
	sum := sha256.Sum256(canonical)
	return findingIDVersion + ":" + hex.EncodeToString(sum[:])
}

func normalizeFindingPath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	for strings.HasPrefix(path, "./") {
		path = strings.TrimPrefix(path, "./")
	}
	return path
}

func normalizeFindingContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Join(strings.Fields(content), " ")
}

func rawString(object map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}

func rawInt(object map[string]json.RawMessage, key string) int {
	var value int
	_ = json.Unmarshal(object[key], &value)
	return value
}

func mustJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isCompleteOCRResult(result map[string]json.RawMessage) bool {
	var status string
	_ = json.Unmarshal(result["status"], &status)
	var manifest struct {
		TerminalState string `json:"terminal_state"`
	}
	if rawManifest, ok := result["manifest"]; ok {
		if json.Unmarshal(rawManifest, &manifest) == nil && manifest.TerminalState != "" {
			return manifest.TerminalState == "complete"
		}
	}
	return status == "complete" || status == "success"
}
