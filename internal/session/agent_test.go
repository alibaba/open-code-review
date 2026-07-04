package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentRecorderPersistsCorrelatedReadOnlyAgentRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repository := t.TempDir()
	recorder, err := OpenAgentRecorder(repository, "run-123", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenAgentRecorder() error = %v", err)
	}
	if err := recorder.Record("prepare", "sha256:bundle", AgentEvent{
		Files: 3, Warnings: 1, Partial: true, DurationMS: 25,
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := recorder.Finalize("sha256:bundle", AgentEvent{Findings: 2, ValidationValid: boolPointer(true)}); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}

	info, err := os.Stat(recorder.Path())
	if err != nil {
		t.Fatalf("stat session: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode = %o, want 600", info.Mode().Perm())
	}
	file, err := os.Open(recorder.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var records []map[string]any
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d, want start, event, end", len(records))
	}
	if records[0]["controlPlane"] != "agent" ||
		records[0]["model"] != "host-agent" ||
		records[0]["reviewMode"] != "agent" ||
		records[0]["bundleId"] != "sha256:bundle" ||
		records[0]["tokenUsage"] != "not_available" {
		t.Fatalf("session start = %+v", records[0])
	}
	if records[1]["type"] != "agent_event" ||
		records[1]["controlPlane"] != "agent" ||
		records[1]["event"] != "prepare" ||
		records[2]["type"] != "session_end" ||
		records[2]["controlPlane"] != "agent" {
		t.Fatalf("records = %+v", records)
	}
}

func TestAgentRecorderRestoresStartTimeWhenResuming(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repository := t.TempDir()
	started := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	directory := filepath.Join(home, ".opencodereview", "sessions", encodeRepoPath(repository))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "run-123.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_start","timestamp":"`+started+`"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	recorder, err := OpenAgentRecorder(repository, "run-123", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenAgentRecorder() error = %v", err)
	}
	if err := recorder.Finalize("sha256:bundle", AgentEvent{}); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	records := readAgentRecords(t, recorder.Path())
	duration, ok := records[len(records)-1]["duration_seconds"].(float64)
	if !ok || duration < 60*60 {
		t.Fatalf("duration_seconds = %v, want resumed duration from original start", records[len(records)-1]["duration_seconds"])
	}
}

func TestAgentRecorderFinalizeIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repository := t.TempDir()
	recorder, err := OpenAgentRecorder(repository, "run-dup", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenAgentRecorder() error = %v", err)
	}
	if err := recorder.Finalize("sha256:bundle", AgentEvent{Findings: 1}); err != nil {
		t.Fatalf("first Finalize() error = %v", err)
	}
	if err := recorder.Finalize("sha256:bundle", AgentEvent{Findings: 99}); err != nil {
		t.Fatalf("second Finalize() error = %v", err)
	}
	records := readAgentRecords(t, recorder.Path())
	sessionEnds := 0
	for _, record := range records {
		if record["type"] == "session_end" {
			sessionEnds++
		}
	}
	if sessionEnds != 1 {
		t.Fatalf("session_end records = %d, want 1", sessionEnds)
	}
}

func TestAgentRecorderDoesNotAppendEventsAfterFinalize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repository := t.TempDir()
	recorder, err := OpenAgentRecorder(repository, "run-ended", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenAgentRecorder() error = %v", err)
	}
	if err := recorder.Finalize("sha256:bundle", AgentEvent{FilesReviewed: []string{"main.go"}}); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if err := recorder.Record("context.read", "sha256:bundle", AgentEvent{ContextCalls: 1}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	records := readAgentRecords(t, recorder.Path())
	if records[len(records)-1]["type"] != "session_end" {
		t.Fatalf("last record = %+v, want session_end", records[len(records)-1])
	}
}

func TestAgentRecorderRecoversOrphanedEmptySessionFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repository := t.TempDir()
	directory := filepath.Join(home, ".opencodereview", "sessions", encodeRepoPath(repository))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "run-orphan.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	recorder, err := OpenAgentRecorder(repository, "run-orphan", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenAgentRecorder() error = %v", err)
	}
	records := readAgentRecords(t, recorder.Path())
	if len(records) != 1 || records[0]["type"] != "session_start" {
		t.Fatalf("records = %+v, want single session_start", records)
	}
}

func TestAgentRecorderRejectsInvalidSessionID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, runID := range []string{"../bad", "run..bad"} {
		if _, err := OpenAgentRecorder(t.TempDir(), runID, "sha256:bundle"); err == nil {
			t.Fatalf("OpenAgentRecorder(%q) error = nil, want invalid session ID", runID)
		}
	}
}

func TestAgentRecorderAllowsManifestToBundleCorrelation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repository := t.TempDir()
	directory := filepath.Join(home, ".opencodereview", "sessions", encodeRepoPath(repository))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "run-123.jsonl")
	if err := os.WriteFile(
		path,
		[]byte(`{"type":"session_start","timestamp":"2026-01-01T00:00:00Z","bundleId":"sha256:manifest"}`+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	recorder, err := OpenAgentRecorder(repository, "run-123", "sha256:nested-bundle")
	if err != nil {
		t.Fatalf("OpenAgentRecorder() error = %v", err)
	}
	if err := recorder.Record("validate", "sha256:nested-bundle", AgentEvent{Findings: 1}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	records := readAgentRecords(t, recorder.Path())
	if records[1]["bundleId"] != "sha256:nested-bundle" {
		t.Fatalf("event bundleId = %v, want nested bundle id", records[1]["bundleId"])
	}
}

func TestAgentRecorderWaitForStartTimesOutOnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-empty.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &AgentRecorder{path: path}
	_, err := waitForAgentSessionStart(recorder, "sha256:bundle", "run-empty")
	if err == nil || !strings.Contains(err.Error(), "has no session_start") {
		t.Fatalf("waitForAgentSessionStart() error = %v, want empty start error", err)
	}
}

func TestAgentSessionReadersIgnoreInvalidRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	if err := os.WriteFile(
		path,
		[]byte("{not-json}\n{\"type\":\"agent_event\"}\n{\"type\":\"session_start\",\"timestamp\":\"bad\"}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	fallback := time.Unix(456, 0)
	if got := readAgentSessionStart(path, fallback); !got.Equal(fallback) {
		t.Fatalf("readAgentSessionStart() = %v, want fallback", got)
	}
	bundleID, err := readAgentSessionBundleID(path)
	if err != nil {
		t.Fatalf("readAgentSessionBundleID() error = %v", err)
	}
	if bundleID != "" {
		t.Fatalf("bundleID = %q, want empty when session_start has no bundleId", bundleID)
	}
}

func readAgentRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var records []map[string]any
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func boolPointer(value bool) *bool {
	return &value
}
