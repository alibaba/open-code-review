// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package process

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func TestKillProcessGroup(t *testing.T) {
	binary := testutil.Install(t, t.TempDir(), "ocr", &testutil.Config{Block: true})
	cmd := exec.Command(binary, "review", "block")
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	defer Close(cmd)
	if err := Kill(cmd); err != nil {
		t.Fatalf("killProcessGroup: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("killed process did not exit")
	}
	if err := Kill(cmd); err != nil && !strings.Contains(err.Error(), "no such process") && !errors.Is(err, os.ErrProcessDone) && !strings.Contains(err.Error(), "process already finished") && !strings.Contains(err.Error(), "Access is denied") && !strings.Contains(err.Error(), "invalid argument") {
		t.Fatalf("second kill = %v", err)
	}
}
