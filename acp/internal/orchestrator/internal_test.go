// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestCauseRecorderKeepsFirstRecordedCause(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name   string
		first  OutcomeKind
		second OutcomeKind
	}{
		{name: "cancellation first", first: OutcomeCancelled, second: OutcomeTimedOut},
		{name: "timeout first", first: OutcomeTimedOut, second: OutcomeCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := newCauseRecorder()
			recorder.record(test.first, now)
			recorder.record(test.second, now.Add(time.Nanosecond))
			cause := <-recorder.ch
			if cause.kind != test.first || !cause.at.Equal(now) {
				t.Fatalf("cause = %+v, want kind %s at %s", cause, test.first, now)
			}
		})
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
