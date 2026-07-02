package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var safeCodexRunID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// CodexEvent contains only metrics supplied by the host-agent workflow.
type CodexEvent struct {
	Files           int      `json:"files,omitempty"`
	Findings        int      `json:"findings,omitempty"`
	Warnings        int      `json:"warnings,omitempty"`
	ContextCalls    int      `json:"context_calls,omitempty"`
	Partial         bool     `json:"partial,omitempty"`
	DurationMS      int64    `json:"duration_ms,omitempty"`
	ValidationValid *bool    `json:"validation_valid,omitempty"`
	FilesReviewed   []string `json:"files_reviewed,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// CodexRecorder appends viewer-compatible host-agent events.
type CodexRecorder struct {
	mu       sync.Mutex
	path     string
	runID    string
	bundleID string
	started  time.Time
}

// OpenCodexRecorder opens or resumes one explicitly requested run ID.
func OpenCodexRecorder(repoDir, runID, bundleID string) (*CodexRecorder, error) {
	if runID == "" || !safeCodexRunID.MatchString(runID) {
		return nil, fmt.Errorf("session ID must contain only letters, digits, dot, underscore, or dash")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	directory := filepath.Join(home, ".opencodereview", "sessions", encodeRepoPath(repoDir))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	path := filepath.Join(directory, runID+".jsonl")
	recorder := &CodexRecorder{
		path: path, runID: runID, bundleID: bundleID, started: time.Now(),
	}
	info, statErr := os.Stat(path)
	if statErr == nil && info.Size() > 0 {
		recorder.started = readCodexSessionStart(path, recorder.started)
		return recorder, nil
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("stat session file: %w", statErr)
	}
	if err := recorder.write(map[string]any{
		"uuid":         generateUUID(),
		"parentUuid":   nil,
		"type":         "session_start",
		"sessionId":    runID,
		"timestamp":    recorder.started.UTC().Format(time.RFC3339),
		"cwd":          repoDir,
		"model":        "host-agent",
		"reviewMode":   "agent",
		"controlPlane": "agent",
		"bundleId":     bundleID,
		"tokenUsage":   "not_available",
	}); err != nil {
		return nil, err
	}
	return recorder, nil
}

// Path returns the persisted JSONL path.
func (recorder *CodexRecorder) Path() string {
	return recorder.path
}

// Record appends one correlated host-agent workflow event.
func (recorder *CodexRecorder) Record(event string, details CodexEvent) error {
	record := codexEventRecord(recorder, "agent_event", details)
	record["event"] = event
	return recorder.write(record)
}

// Finalize appends a viewer-compatible session end record.
func (recorder *CodexRecorder) Finalize(details CodexEvent) error {
	record := codexEventRecord(recorder, "session_end", details)
	record["duration_seconds"] = time.Since(recorder.started).Seconds()
	record["files_reviewed"] = details.FilesReviewed
	record["llm_failures"] = 0
	return recorder.write(record)
}

func codexEventRecord(
	recorder *CodexRecorder,
	recordType string,
	details CodexEvent,
) map[string]any {
	var fields map[string]any
	content, err := json.Marshal(details)
	if err == nil {
		err = json.Unmarshal(content, &fields)
	}
	if err != nil || fields == nil {
		fields = make(map[string]any)
	}
	fields["uuid"] = generateUUID()
	fields["parentUuid"] = nil
	fields["type"] = recordType
	fields["sessionId"] = recorder.runID
	fields["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	fields["controlPlane"] = "agent"
	fields["bundleId"] = recorder.bundleID
	fields["tokenUsage"] = "not_available"
	return fields
}

func readCodexSessionStart(path string, fallback time.Time) time.Time {
	file, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if recordType, _ := record["type"].(string); recordType != "session_start" {
			continue
		}
		timestamp, _ := record["timestamp"].(string)
		started, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return fallback
		}
		return started
	}
	return fallback
}

func (recorder *CodexRecorder) write(record map[string]any) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal agent session record: %w", err)
	}
	file, err := os.OpenFile(
		recorder.path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open agent session: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write agent session: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close agent session: %w", err)
	}
	return nil
}
