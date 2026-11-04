// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func TestVersionProbeAllowsBriefInheritedOutputWait(t *testing.T) {
	binary := testutil.Install(t, t.TempDir(), "ocr", &testutil.Config{
		VersionPrefix: "ocr ",
		VersionSuffix: "1.2.3\n",
		FlushMS:       300,
	})
	version, err := probeVersion(binary, 5*time.Second)
	if err != nil || version != "ocr 1.2.3" {
		t.Fatalf("version=%q err=%v", version, err)
	}
}

func TestVersionProbeBoundsInheritedOutputWait(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	binary := testutil.Install(t, dir, "ocr", &testutil.Config{
		VersionText: "ocr-test",
		HoldPIDFile: pidFile,
		HoldInherit: true,
		HoldMS:      30000,
	})
	result := make(chan error, 1)
	go func() {
		_, err := probeVersion(binary, 2*time.Second)
		result <- err
	}()
	readChild := func() int {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		var data []byte
		var err error
		for time.Now().Before(deadline) {
			data, err = os.ReadFile(pidFile)
			if err == nil && strings.TrimSpace(string(data)) != "" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
	select {
	case err := <-result:
		pid := readChild()
		t.Cleanup(func() { _ = killPID(pid) })
		if err == nil {
			t.Fatal("inherited output did not fail version probe")
		}
		deadline := time.Now().Add(time.Second)
		for processRunning(pid) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if processRunning(pid) {
			t.Fatal("version probe left its child running")
		}
	case <-time.After(4 * time.Second):
		_ = killPID(readChild())
		<-result
		t.Fatal("version probe ignored its deadline while a descendant held output")
	}
}
