package session

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// jsonlWriter streams session records to a JSONL file under
// $HOME/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl.
// It is safe for concurrent use by multiple goroutines.
type jsonlWriter struct {
	mu         sync.Mutex
	sessionID  string
	repoDir    string
	gitBranch  string
	model      string
	reviewMode string
	diffFrom   string
	diffTo     string
	diffCommit string
	file       *os.File
	writer     *bufio.Writer
	lastUUID   string // tracks chain of records via parentUuid
}

// newJSONLWriter creates and opens a new JSONL writer for the given session.
func newJSONLWriter(sessionID, repoDir, gitBranch, model string, opts SessionOptions) (*jsonlWriter, error) {
	jw := &jsonlWriter{
		sessionID:  sessionID,
		repoDir:    repoDir,
		gitBranch:  gitBranch,
		model:      model,
		reviewMode: opts.ReviewMode,
		diffFrom:   opts.DiffFrom,
		diffTo:     opts.DiffTo,
		diffCommit: opts.DiffCommit,
	}
	if err := jw.open(); err != nil {
		return nil, err
	}
	return jw, nil
}

func generateUUID() string {
	b := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, b)
	if err != nil {
		// Fallback — extremely unlikely but keeps things working without panics.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func encodeRepoPath(p string) string {
	// Handle empty or invalid input
	if p == "" {
		return "empty"
	}

	vol := filepath.VolumeName(p)
	p = p[len(vol):]

	// Trim leading path separators
	p = strings.TrimLeft(p, "/\\")

	// Replace separators with -
	p = strings.ReplaceAll(p, "/", "-")
	p = strings.ReplaceAll(p, "\\", "-")

	// Replace colons (from Windows drive letters)
	vol = strings.ReplaceAll(vol, ":", "_")

	// Handle edge case where path was only separators or volume name
	result := vol + p
	if result == "" {
		return "empty"
	}
	return result
}

func (jw *jsonlWriter) open() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}

	sessionDir := filepath.Join(home, ".opencodereview", "sessions", encodeRepoPath(jw.repoDir))
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	filename := filepath.Join(sessionDir, jw.sessionID+".jsonl")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}

	jw.file = f
	jw.writer = bufio.NewWriter(f)
	return nil
}

func (jw *jsonlWriter) writeRecordLocked(rec map[string]any) {
	data, err := json.Marshal(rec)
	if err != nil {
		fmt.Printf("[ocr session] failed to marshal record: %v\n", err)
		return
	}
	jw.writer.Write(data)
	jw.writer.WriteByte('\n')
}

// WriteSessionStart writes the initial session_start record.
func (jw *jsonlWriter) WriteSessionStart(startTime time.Time) string {
	uuid := generateUUID()
	rec := map[string]any{
		"uuid":       uuid,
		"parentUuid": nil,
		"type":       "session_start",
		"sessionId":  jw.sessionID,
		"timestamp":  startTime.UTC().Format(time.RFC3339),
		"cwd":        jw.repoDir,
		"gitBranch":  jw.gitBranch,
		"model":      jw.model,
	}
	if jw.reviewMode != "" {
		rec["reviewMode"] = jw.reviewMode
	}
	if jw.diffFrom != "" {
		rec["diffFrom"] = jw.diffFrom
	}
	if jw.diffTo != "" {
		rec["diffTo"] = jw.diffTo
	}
	if jw.diffCommit != "" {
		rec["diffCommit"] = jw.diffCommit
	}

	jw.mu.Lock()
	defer jw.mu.Unlock()
	jw.writeRecordLocked(rec)
	jw.lastUUID = uuid
	return uuid
}

// WriteLLMRequest writes a request entry with the resolved messages.
func (jw *jsonlWriter) WriteLLMRequest(filePath string, taskType TaskType, requestNo int, messages any) string {
	uuid := generateUUID()

	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid":       uuid,
		"parentUuid": jw.lastUUID,
		"type":       "llm_request",
		"sessionId":  jw.sessionID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"filePath":   filePath,
		"taskType":   string(taskType),
		"request_no": requestNo,
		"messages":   messages,
	}
	jw.writeRecordLocked(rec)
	jw.lastUUID = uuid
	return uuid
}

// WriteLLMResponse writes a response entry with model, content, tool calls, usage.
func (jw *jsonlWriter) WriteLLMResponse(filePath string, taskType TaskType, content string, toolCalls []map[string]any, model string, usage TokenUsage, duration time.Duration) string {
	uuid := generateUUID()

	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid":        uuid,
		"parentUuid":  jw.lastUUID,
		"type":        "llm_response",
		"sessionId":   jw.sessionID,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"filePath":    filePath,
		"taskType":    string(taskType),
		"model":       model,
		"content":     content,
		"tool_calls":  toolCalls,
		"duration_ms": duration.Milliseconds(),
		"usage": map[string]int{
			"prompt_tokens":      usage.PromptTokens,
			"completion_tokens":  usage.CompletionTokens,
			"cache_read_tokens":  usage.CacheReadTokens,
			"cache_write_tokens": usage.CacheWriteTokens,
		},
	}
	jw.writeRecordLocked(rec)
	jw.lastUUID = uuid
	return uuid
}

// WriteLLMError writes an llm_error entry recording a failed LLM request.
func (jw *jsonlWriter) WriteLLMError(filePath string, taskType TaskType, requestNo int, errorMsg string, duration time.Duration) string {
	uuid := generateUUID()

	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid":        uuid,
		"parentUuid":  jw.lastUUID,
		"type":        "llm_error",
		"sessionId":   jw.sessionID,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"filePath":    filePath,
		"taskType":    string(taskType),
		"request_no":  requestNo,
		"error":       errorMsg,
		"duration_ms": duration.Milliseconds(),
	}
	jw.writeRecordLocked(rec)
	jw.lastUUID = uuid
	return uuid
}

// WriteToolCall writes a tool call result entry.
func (jw *jsonlWriter) WriteToolCall(filePath string, taskType TaskType, toolName, arguments, result string, ok bool, duration time.Duration) string {
	uuid := generateUUID()

	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid":        uuid,
		"parentUuid":  jw.lastUUID,
		"type":        "tool_call",
		"sessionId":   jw.sessionID,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"filePath":    filePath,
		"taskType":    string(taskType),
		"tool_name":   toolName,
		"arguments":   arguments,
		"result":      result,
		"ok":          ok,
		"duration_ms": duration.Milliseconds(),
	}
	jw.writeRecordLocked(rec)
	jw.lastUUID = uuid
	return uuid
}

// WriteSessionEnd writes the final session_end summary record and closes the file.
func (jw *jsonlWriter) WriteSessionEnd(duration time.Duration, filesReviewed []string, llmFailures int64) {
	uuid := generateUUID()

	jw.mu.Lock()
	defer jw.mu.Unlock()
	rec := map[string]any{
		"uuid":             uuid,
		"parentUuid":       jw.lastUUID,
		"type":             "session_end",
		"sessionId":        jw.sessionID,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"files_reviewed":   filesReviewed,
		"duration_seconds": duration.Seconds(),
		"llm_failures":     llmFailures,
	}
	jw.writeRecordLocked(rec)
	jw.lastUUID = uuid

	if jw.writer != nil {
		jw.writer.Flush()
	}
	if jw.file != nil {
		jw.file.Close()
	}
}

func (jw *jsonlWriter) flushAndClose() {
	jw.mu.Lock()
	defer jw.mu.Unlock()
	if jw.writer != nil {
		jw.writer.Flush()
	}
	if jw.file != nil {
		jw.file.Close()
	}
}
