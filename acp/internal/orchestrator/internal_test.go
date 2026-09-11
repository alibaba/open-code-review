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

func TestKillProcessGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "while :; do sleep 1; done")
	configureProcessGroup(cmd)
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
