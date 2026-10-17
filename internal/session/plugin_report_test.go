// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPluginReportsRoundTrip(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	t.Cleanup(func() { _ = sh.Finalize() })
	sh.RecordReviewItemFailed("ordinary.go", "ordinary.go", "ordinary.go", "ordinary", "unrelated failure")
	reports := []PluginReviewReport{
		{Plugin: "custom", Group: "b.go", Stage: "inspect", Complete: true, TokenLimit: 5000, Report: json.RawMessage(`{"checked":["return value"],"evidence":{"line":8}}`)},
		{Plugin: "custom", Group: "a.go", Stage: "inspect", Report: json.RawMessage(`{"pending":["caller"]}`), Error: "caller verification is incomplete"},
		{Plugin: "custom", Group: "b.go", Stage: "verify", Report: json.RawMessage(`{"saved":{"checked":true}}`), Error: "required plugin stage failed; see session error"},
	}
	for _, report := range reports {
		if err := sh.RecordPluginReview(report); err != nil {
			t.Fatalf("record stage %s: %v", report.Stage, err)
		}
	}
	want := []PluginReviewReport{reports[1], reports[0], reports[2]}
	checkSaved := func() {
		t.Helper()
		got, err := LoadPluginReviewReports(repoDir, sh.SessionID)
		if err != nil {
			t.Fatalf("load reports: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("saved reports = %+v, want %+v", got, want)
		}
	}
	checkSaved()
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	checkSaved()

	path, err := SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	records := readJSONLRecords(t, path)
	for i, rec := range records {
		if rec["type"] != "plugin_review" {
			continue
		}
		if rec["sessionId"] != sh.SessionID || rec["uuid"] == "" || i == 0 || rec["parentUuid"] != records[i-1]["uuid"] {
			t.Fatalf("invalid plugin record envelope: %+v", rec)
		}
		if _, err := time.Parse(time.RFC3339, rec["timestamp"].(string)); err != nil {
			t.Fatalf("invalid plugin record timestamp: %v", err)
		}
	}
	listed, err := ListSessions(repoDir)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list sessions = %+v, error = %v", listed, err)
	}
	data, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "review_plugin") || listed[0].FailedFiles != 1 {
		t.Fatalf("plugin reports changed session summary: %s", data)
	}
}

func TestPluginReportsIgnoreOrdinarySession(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{})
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPluginReviewReports(repoDir, sh.SessionID)
	if err != nil || len(got) != 0 {
		t.Fatalf("ordinary session reports = %+v, error = %v", got, err)
	}
	if _, err := LoadPluginReviewReports(repoDir, "missing"); err == nil {
		t.Fatal("missing session returned no error")
	}
	if _, err := LoadPluginReviewReports(repoDir, "../outside"); err == nil {
		t.Fatal("invalid session ID returned no error")
	}
}

func TestPluginReportMarshalErrorPreservesChain(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{})
	t.Cleanup(func() { _ = sh.Finalize() })
	parent := sh.persist.lastUUID
	err := sh.RecordPluginReview(PluginReviewReport{Report: json.RawMessage(`{"unfinished":`)})
	if err == nil || !strings.Contains(err.Error(), "marshal plugin_review") {
		t.Fatalf("invalid report error = %v", err)
	}
	if sh.persist.lastUUID != parent {
		t.Fatal("failed marshal advanced the record chain")
	}
	report := PluginReviewReport{Plugin: "custom", Group: "a.go", Stage: "inspect", Complete: true}
	if err := sh.RecordPluginReview(report); err != nil {
		t.Fatalf("valid report after marshal error: %v", err)
	}
	got, err := LoadPluginReviewReports(repoDir, sh.SessionID)
	if err != nil || !reflect.DeepEqual(got, []PluginReviewReport{report}) {
		t.Fatalf("saved reports = %+v, error = %v", got, err)
	}
}

type pluginReportFailingWriter struct{ err error }

func (w pluginReportFailingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPluginReportWriteErrorsPreserveChain(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "write", size: 1},
		{name: "flush", size: 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.New("session storage failed")
			sh := &SessionHistory{persist: &jsonlWriter{
				sessionID: "session", lastUUID: "parent",
				writer: bufio.NewWriterSize(pluginReportFailingWriter{failure}, tc.size),
			}}
			err := sh.RecordPluginReview(PluginReviewReport{Plugin: "custom", Group: "a.go", Stage: "inspect"})
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), tc.name+" plugin_review") {
				t.Fatalf("%s error = %v", tc.name, err)
			}
			if sh.persist.lastUUID != "parent" {
				t.Fatal("failed delivery advanced the record chain")
			}
		})
	}
}

func TestPluginReportWriterInitializationError(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	if err := os.WriteFile(filepath.Join(tmpHome, ".opencodereview"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	sh := New(t.TempDir(), "main", "test-model", SessionOptions{})
	err := sh.RecordPluginReview(PluginReviewReport{})
	if err == nil || err != sh.persistInitErr {
		t.Fatalf("report did not return writer creation error: %v", err)
	}
}

func TestPluginReportWithoutPersistence(t *testing.T) {
	for _, sh := range []*SessionHistory{nil, {}} {
		if err := sh.RecordPluginReview(PluginReviewReport{}); err != nil {
			t.Fatalf("non-persisted session: %v", err)
		}
	}
}
