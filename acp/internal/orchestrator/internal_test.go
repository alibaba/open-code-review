// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type repeatedByteReader struct{ remaining int }

func (r *repeatedByteReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

func TestStderrLongLineUsesBoundedMemory(t *testing.T) {
	// Generate the input without allocating it. A ReadString-based reader would
	// allocate the entire 32 MiB line before applying either configured limit.
	input := &repeatedByteReader{remaining: 32 << 20}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	count := 0
	tail, cut, warnings, err := readStderr(input, 1024, 64, func(event Event) {
		count++
		if len(event.Message) > 64 || !event.Truncated {
			t.Errorf("unbounded or unmarked event: %+v", event)
		}
	})
	runtime.ReadMemStats(&after)
	if err != nil || input.remaining != 0 || len(tail) != 1024 || !cut || len(warnings) != 1 || count != 2 {
		t.Fatalf("tail=%d cut=%v warnings=%d events=%d remaining=%d err=%v", len(tail), cut, len(warnings), count, input.remaining, err)
	}
	// Leave ample room for runtime bookkeeping; the input itself exceeds this
	// ceiling fourfold, so allocating a full line cannot pass.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("stderr reader allocated %d bytes for a bounded line", allocated)
	}
}

func TestDecodeResultValidationAndStatuses(t *testing.T) {
	for _, test := range []struct {
		args []string
		data string
		kind ErrorKind
	}{
		{nil, `{}`, ErrorInvalidRequest},
		{[]string{"review"}, ` `, ErrorDecode},
		{[]string{"other"}, `{}`, ErrorInvalidRequest},
		{[]string{"review"}, `{"status":"failed","comments":[]}`, ErrorResult},
		{[]string{"scan"}, `{"status":"failed","comments":[]}`, ErrorResult},
	} {
		_, err := decodeResult(test.args, []byte(test.data))
		assertErrorKind(t, err, test.kind)
	}
}

func TestDecodeResultAcceptsContractSuccessStatuses(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		data string
	}{
		{name: "review partial", args: []string{"review"}, data: " \n{\"status\":\"partial\",\"comments\":[]}\n "},
		{name: "review skipped", args: []string{"review"}, data: `{"status":"skipped","comments":[]}`},
		{name: "scan completed with errors", args: []string{"scan"}, data: `{"status":"completed_with_errors","comments":[]}`},
		{name: "scan skipped", args: []string{"scan"}, data: `{"status":"skipped","comments":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeResult(test.args, []byte(test.data)); err != nil {
				t.Fatalf("decodeResult: %v", err)
			}
		})
	}

	for _, args := range [][]string{{"review"}, {"scan"}} {
		if _, err := decodeResult(args, []byte(`{"status":"unknown","comments":[]}`)); err == nil {
			t.Fatalf("decodeResult(%q) accepted an unknown status", args[0])
		}
	}
}

func TestValidateRequestAdditionalFailures(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		binary string
		cwd    string
	}{
		{"", dir},
		{"ocr", ""},
		{"ocr", file},
	} {
		if err := validateRequest(test.binary, Request{CWD: test.cwd, Args: []string{"review"}}); err == nil {
			t.Fatalf("validateRequest(%q, %q) succeeded", test.binary, test.cwd)
		}
	}
}

func TestLimitedAndTailBuffers(t *testing.T) {
	limited := limitedBuffer{limit: 3}
	if n, err := limited.Write([]byte("abcdef")); n != 3 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("limited write = %d, %v", n, err)
	}
	if !limited.exceeded || limited.String() != "abc" {
		t.Fatalf("limited = %q exceeded=%v", limited.String(), limited.exceeded)
	}
	tail := tailBuffer{limit: 4}
	tail.Write([]byte("abcdef"))
	if tail.String() != "cdef" || !tail.cut {
		t.Fatalf("tail = %q cut=%v", tail.String(), tail.cut)
	}
	tail.Write([]byte("gh"))
	if tail.String() != "efgh" {
		t.Fatalf("tail after append = %q", tail.String())
	}
}

func TestTypedErrorWithoutCause(t *testing.T) {
	err := (&Error{Kind: ErrorCleanup}).Error()
	if err != string(ErrorCleanup) {
		t.Fatalf("Error() = %q", err)
	}
}

func TestCommandExitCodeAndLocalOutcomeError(t *testing.T) {
	if commandExitCode(nil) != 0 || commandExitCode(errors.New("x")) != -1 {
		t.Fatal("unexpected exit codes")
	}
	if localOutcomeError(context.Canceled) == nil {
		t.Fatal("expected local error")
	}
}

func TestKillProcessGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "while :; do sleep 1; done")
	if err := configureProcessGroup(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := killProcessGroup(cmd); err != nil {
		t.Fatalf("killProcessGroup: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("killed process did not exit")
	}
	if err := killProcessGroup(cmd); err != nil && !strings.Contains(err.Error(), "no such process") {
		t.Fatalf("second kill = %v", err)
	}
}
