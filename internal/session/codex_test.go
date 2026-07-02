package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCodexRecorderPersistsCorrelatedReadOnlyAgentRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repository := t.TempDir()
	recorder, err := OpenCodexRecorder(repository, "run-123", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenCodexRecorder() error = %v", err)
	}
	if err := recorder.Record("prepare", CodexEvent{
		Files: 3, Warnings: 1, Partial: true, DurationMS: 25,
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := recorder.Finalize(CodexEvent{Findings: 2, ValidationValid: boolPointer(true)}); err != nil {
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

func TestCodexRecorderRestoresStartTimeWhenResuming(t *testing.T) {
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

	recorder, err := OpenCodexRecorder(repository, "run-123", "sha256:bundle")
	if err != nil {
		t.Fatalf("OpenCodexRecorder() error = %v", err)
	}
	if err := recorder.Finalize(CodexEvent{}); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	records := readCodexRecords(t, recorder.Path())
	duration, ok := records[len(records)-1]["duration_seconds"].(float64)
	if !ok || duration < 60*60 {
		t.Fatalf("duration_seconds = %v, want resumed duration from original start", records[len(records)-1]["duration_seconds"])
	}
}

func readCodexRecords(t *testing.T, path string) []map[string]any {
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
