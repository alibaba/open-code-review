// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

// rawSubDir is the subdirectory of ~/.opencodereview that holds the raw LLM
// captures. It is deliberately separate from sessionSubDir: the main session
// JSONL is a resume checkpoint, while raw captures are unparsed bytes
// consumed by downstream post-processing, and the two must never write into
// each other's files.
var rawSubDir = "raw"

// RawFileWriter implements llm.RawWriter by appending one JSONL line per
// HTTP attempt to $HOME/.opencodereview/raw/<encoded-repo-path>/<session-id>.jsonl.
// The layout mirrors the session JSONL's so both can be discovered by repo.
// It is safe for concurrent use: with concurrency > 1 many per-file subtasks
// issue LLM calls at once, and the middleware runs once per HTTP attempt.
type RawFileWriter struct {
	mu        sync.Mutex
	sessionID string
	file      *os.File
	writer    *bufio.Writer
	encoder   *json.Encoder
}

// NewRawFileWriter opens (creating if needed) the raw capture file for
// sessionID under repoDir's encoded raw directory. Directories are created
// 0700 and the file 0600, matching the session JSONL: captures contain full
// prompts, completions and API keys' host endpoints, so they are per-user
// only.
// The file is opened O_APPEND: a resume reuses the session ID, and prior
// runs' records must survive the reopened writer.
func NewRawFileWriter(repoDir, sessionID string) (*RawFileWriter, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".opencodereview", rawSubDir, encodeRepoPath(repoDir))
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create raw dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open raw file: %w", err)
	}
	w := &RawFileWriter{
		sessionID: sessionID,
		file:      f,
		writer:    bufio.NewWriter(f),
	}
	w.encoder = json.NewEncoder(w.writer)
	w.encoder.SetEscapeHTML(false)
	return w, nil
}

// Write appends rec as one JSONL line and flushes it to disk. The
// per-record flush means a crashed run keeps every record written up to the
// crash — the same durability trade-off the session JSONL writer makes.
// Write errors are dropped: raw capture must never fail a review.
func (w *RawFileWriter) Write(rec llm.RawRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	rec.SessionID = w.sessionID
	if rec.Timestamp == "" {
		rec.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if err := w.encoder.Encode(rec); err != nil {
		return
	}
	_ = w.writer.Flush()
}

// Close flushes any buffered data and closes the underlying file.
func (w *RawFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.writer.Flush(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}
