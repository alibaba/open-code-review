package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var safeAgentRunID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

const maxAgentSessionLineBytes = 10 * 1024 * 1024

// AgentEvent contains only metrics supplied by the host-agent workflow.
type AgentEvent struct {
	Files           int      `json:"files,omitempty"`
	Findings        int      `json:"findings,omitempty"`
	Warnings        int      `json:"warnings,omitempty"`
	ContextCalls    int      `json:"context_calls,omitempty"`
	Partial         bool     `json:"partial,omitempty"`
	DurationMS      int64    `json:"duration_ms,omitempty"`
	ValidationValid *bool    `json:"validation_valid,omitempty"`
	FilesReviewed   []string `json:"files_reviewed"`
	Error           string   `json:"error,omitempty"`
}

// AgentRecorder appends viewer-compatible host-agent events.
type AgentRecorder struct {
	mu       sync.Mutex
	path     string
	runID    string
	bundleID string
	started  time.Time
}

// OpenAgentRecorder opens or resumes one explicitly requested run ID.
func OpenAgentRecorder(repoDir, runID, bundleID string) (*AgentRecorder, error) {
	if runID == "" || strings.Contains(runID, "..") || !safeAgentRunID.MatchString(runID) {
		return nil, fmt.Errorf("session ID must contain only letters, digits, dot, underscore, or dash and cannot contain '..'")
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
	recorder := &AgentRecorder{
		path: path, runID: runID, bundleID: bundleID, started: time.Now(),
	}
	info, statErr := os.Stat(path)
	if statErr == nil {
		if info.Size() > 0 {
			return finishResumeAgentRecorder(recorder, bundleID, runID)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove orphaned session file: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("stat session file: %w", statErr)
	}
	if err := recorder.writeExclusiveStart(map[string]any{
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
		if !os.IsExist(err) {
			return nil, err
		}
		resumed, resumeErr := waitForAgentSessionStart(recorder, bundleID, runID)
		if resumeErr != nil {
			return nil, resumeErr
		}
		return resumed, nil
	}
	return recorder, nil
}

func waitForAgentSessionStart(
	recorder *AgentRecorder,
	bundleID string,
	runID string,
) (*AgentRecorder, error) {
	const attempts = 20
	for attempt := 0; attempt < attempts; attempt++ {
		info, statErr := os.Stat(recorder.path)
		if statErr != nil {
			return nil, fmt.Errorf("stat session file after create race: %w", statErr)
		}
		if info.Size() > 0 {
			return finishResumeAgentRecorder(recorder, bundleID, runID)
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("session %q exists but has no session_start record", runID)
}

func finishResumeAgentRecorder(
	recorder *AgentRecorder,
	bundleID string,
	runID string,
) (*AgentRecorder, error) {
	recorder.started = readAgentSessionStart(recorder.path, recorder.started)
	existingBundleID, readErr := readAgentSessionBundleID(recorder.path)
	if readErr != nil {
		return nil, readErr
	}
	if existingBundleID != "" {
		recorder.bundleID = existingBundleID
	} else if bundleID != "" {
		recorder.bundleID = bundleID
	}
	return recorder, nil
}

// Path returns the persisted JSONL path.
func (recorder *AgentRecorder) Path() string {
	return recorder.path
}

// Record appends one correlated host-agent workflow event.
func (recorder *AgentRecorder) Record(event string, bundleID string, details AgentEvent) error {
	record := agentEventRecord(recorder, "agent_event", bundleID, details)
	record["event"] = event
	return recorder.write(record, true)
}

// Finalize appends a viewer-compatible session end record.
func (recorder *AgentRecorder) Finalize(bundleID string, details AgentEvent) error {
	record := agentEventRecord(recorder, "session_end", bundleID, details)
	record["duration_seconds"] = time.Since(recorder.started).Seconds()
	record["files_reviewed"] = details.FilesReviewed
	record["llm_failures"] = 0
	return recorder.write(record, true)
}

func agentEventRecord(
	recorder *AgentRecorder,
	recordType string,
	bundleID string,
	details AgentEvent,
) map[string]any {
	var fields map[string]any
	content, err := json.Marshal(details)
	if err == nil {
		err = json.Unmarshal(content, &fields)
	}
	if err != nil || fields == nil {
		fields = make(map[string]any)
	}
	if details.FilesReviewed == nil {
		fields["files_reviewed"] = []string{}
	}
	fields["uuid"] = generateUUID()
	fields["parentUuid"] = nil
	fields["type"] = recordType
	fields["sessionId"] = recorder.runID
	fields["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	fields["controlPlane"] = "agent"
	if bundleID != "" {
		fields["bundleId"] = bundleID
	} else if recorder.bundleID != "" {
		fields["bundleId"] = recorder.bundleID
	}
	fields["tokenUsage"] = "not_available"
	return fields
}

func readAgentSessionStart(path string, fallback time.Time) time.Time {
	file, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer file.Close()

	scanner := newAgentSessionScanner(file)
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

func readAgentSessionBundleID(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open session file: %w", err)
	}
	defer file.Close()

	scanner := newAgentSessionScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if recordType, _ := record["type"].(string); recordType != "session_start" {
			continue
		}
		bundleID, _ := record["bundleId"].(string)
		return bundleID, nil
	}
	return "", nil
}

func agentSessionHasEnd(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := newAgentSessionScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if recordType, _ := record["type"].(string); recordType == "session_end" {
			return true
		}
	}
	return false
}

func (recorder *AgentRecorder) writeExclusiveStart(record map[string]any) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal agent session record: %w", err)
	}
	file, err := os.OpenFile(
		recorder.path,
		os.O_CREATE|os.O_WRONLY|os.O_EXCL,
		0o600,
	)
	if err != nil {
		return err
	}
	if err := lockSessionFile(file); err != nil {
		_ = file.Close()
		return fmt.Errorf("lock agent session: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		unlockErr := unlockSessionFile(file)
		closeErr := file.Close()
		if unlockErr != nil {
			return fmt.Errorf("unlock agent session after write failure: %w", unlockErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close agent session after write failure: %w", closeErr)
		}
		return fmt.Errorf("write agent session: %w", err)
	}
	unlockErr := unlockSessionFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock agent session: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close agent session: %w", closeErr)
	}
	return nil
}

func (recorder *AgentRecorder) write(record map[string]any, skipIfEnded bool) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal agent session record: %w", err)
	}
	file, err := os.OpenFile(
		recorder.path,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("open agent session: %w", err)
	}
	if err := lockSessionFile(file); err != nil {
		_ = file.Close()
		return fmt.Errorf("lock agent session: %w", err)
	}
	if skipIfEnded {
		ended, endErr := agentSessionFileHasEnd(file)
		if endErr != nil {
			unlockSessionFile(file)
			_ = file.Close()
			return endErr
		}
		if ended {
			unlockErr := unlockSessionFile(file)
			closeErr := file.Close()
			if unlockErr != nil {
				return fmt.Errorf("unlock agent session: %w", unlockErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close agent session: %w", closeErr)
			}
			return nil
		}
	}
	_, writeErr := file.Write(append(encoded, '\n'))
	unlockErr := unlockSessionFile(file)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write agent session: %w", writeErr)
	}
	if unlockErr != nil {
		return fmt.Errorf("unlock agent session: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close agent session: %w", closeErr)
	}
	return nil
}

func agentSessionFileHasEnd(file *os.File) (bool, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return false, fmt.Errorf("seek agent session: %w", err)
	}
	scanner := newAgentSessionScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if recordType, _ := record["type"].(string); recordType == "session_end" {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("scan agent session: %w", err)
	}
	return false, nil
}

func newAgentSessionScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), maxAgentSessionLineBytes)
	return scanner
}
