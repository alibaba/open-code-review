// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Environment keys used to steer the helper process below.
const (
	helperModeEnv   = "OCR_VIEWER_BROWSER_HELPER"
	helperSleepEnv  = "OCR_VIEWER_BROWSER_HELPER_SLEEP"
	helperExitEnv   = "OCR_VIEWER_BROWSER_HELPER_EXIT"
	helperStderrEnv = "OCR_VIEWER_BROWSER_HELPER_STDERR"
)

// TestBrowserHelperProcess is not a real test. It is the child process the
// tests below exec, standing in for xdg-open/open/rundll32 so exit-status
// handling can be exercised without a browser and without platform-specific
// shell commands. It does nothing unless the parent set helperModeEnv.
func TestBrowserHelperProcess(t *testing.T) {
	if os.Getenv(helperModeEnv) != "1" {
		return
	}
	if msg := os.Getenv(helperStderrEnv); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	if d, err := time.ParseDuration(os.Getenv(helperSleepEnv)); err == nil {
		time.Sleep(d)
	}
	code, _ := strconv.Atoi(os.Getenv(helperExitEnv))
	os.Exit(code)
}

// helperArgv is the argv that re-execs this test binary as the helper.
func helperArgv() []string {
	return []string{os.Args[0], "-test.run=^TestBrowserHelperProcess$"}
}

// helperCmd builds a helper command with its steering variables set on the Cmd
// itself, for the tests that hand an *exec.Cmd to runBrowserCmd directly.
func helperCmd(sleep time.Duration, exitCode int) *exec.Cmd {
	return helperCmdStderr(sleep, exitCode, "")
}

// helperCmdStderr is helperCmd with the child also writing msg to stderr.
func helperCmdStderr(sleep time.Duration, exitCode int, msg string) *exec.Cmd {
	argv := helperArgv()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(),
		helperModeEnv+"=1",
		helperSleepEnv+"="+sleep.String(),
		helperExitEnv+"="+strconv.Itoa(exitCode),
		helperStderrEnv+"="+msg,
	)
	return cmd
}

// setHelperEnv steers children of this process into the helper branch.
// openBrowserCandidates builds its own exec.Cmd from an argv, so the inherited
// environment is the only channel available for configuring the child.
func setHelperEnv(t *testing.T, sleep time.Duration, exitCode int) {
	t.Helper()
	t.Setenv(helperModeEnv, "1")
	t.Setenv(helperSleepEnv, sleep.String())
	t.Setenv(helperExitEnv, strconv.Itoa(exitCode))
}

// setBrowserWaitWindow overrides the wait window for one test. Tests in this
// package run sequentially, so mutating the package variable is safe.
func setBrowserWaitWindow(t *testing.T, d time.Duration) {
	t.Helper()
	prev := browserWaitWindow
	browserWaitWindow = d
	t.Cleanup(func() { browserWaitWindow = prev })
}

// syncBuffer collects warnings written from runBrowserCmd's watcher goroutine
// while the test reads them, so the assertion does not race the writer.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureBrowserWarnings redirects browserWarnf for one test.
func captureBrowserWarnings(t *testing.T) *syncBuffer {
	t.Helper()
	var buf syncBuffer
	prev := browserWarnOut
	browserWarnOut = &buf
	t.Cleanup(func() { browserWarnOut = prev })
	return &buf
}

// waitForWarning polls until a warning shows up, so the test neither sleeps a
// fixed amount nor hangs forever if the watcher goroutine never fires.
func waitForWarning(t *testing.T, buf *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("no warning containing %q within 10s; got %q", want, buf.String())
}

func TestRunBrowserCmd_ExitZero(t *testing.T) {
	setBrowserWaitWindow(t, 30*time.Second)
	if err := runBrowserCmd(helperCmd(0, 0)); err != nil {
		t.Errorf("runBrowserCmd(exit 0) = %v, want nil", err)
	}
}

// TestRunBrowserCmd_NonZeroExit is the regression test for openers that fork
// successfully and then fail: xdg-open exits non-zero when no handler is
// registered, open exits non-zero when no application claims the scheme.
// Checking only Start() reported success for both, leaving the user with
// neither a browser nor a warning.
func TestRunBrowserCmd_NonZeroExit(t *testing.T) {
	setBrowserWaitWindow(t, 30*time.Second)
	err := runBrowserCmd(helperCmd(0, 3))
	if err == nil {
		t.Fatal("runBrowserCmd(exit 3) = nil, want an error")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runBrowserCmd(exit 3) = %v, want *exec.ExitError", err)
	}
	if got := exitErr.ExitCode(); got != 3 {
		t.Errorf("exit code = %d, want 3", got)
	}
}

// TestRunBrowserCmd_NonZeroExitIncludesStderr checks that the opener's own
// diagnostic survives. Without it a failure reduces to "exit status 3", which
// does not tell the user what to fix.
func TestRunBrowserCmd_NonZeroExitIncludesStderr(t *testing.T) {
	setBrowserWaitWindow(t, 30*time.Second)
	err := runBrowserCmd(helperCmdStderr(0, 3, "xdg-open: no method available"))
	if err == nil {
		t.Fatal("runBrowserCmd(exit 3 with stderr) = nil, want an error")
	}
	if !strings.Contains(err.Error(), "no method available") {
		t.Errorf("error = %q, want it to carry the opener's stderr", err)
	}
	// The exit status must still be recoverable through the wrapping.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("error = %v, want *exec.ExitError to remain unwrappable", err)
	}
}

// TestRunBrowserCmd_PreservesCallerStderr makes sure the capture does not
// hijack a Stderr the caller already set.
func TestRunBrowserCmd_PreservesCallerStderr(t *testing.T) {
	setBrowserWaitWindow(t, 30*time.Second)
	var sink strings.Builder
	cmd := helperCmdStderr(0, 0, "hello from the opener")
	cmd.Stderr = &sink
	if err := runBrowserCmd(cmd); err != nil {
		t.Fatalf("runBrowserCmd = %v, want nil", err)
	}
	if !strings.Contains(sink.String(), "hello from the opener") {
		t.Errorf("caller's Stderr got %q, want the child's output", sink.String())
	}
}

func TestRunBrowserCmd_StartFailure(t *testing.T) {
	if err := runBrowserCmd(exec.Command("ocr-viewer-no-such-browser-opener")); err == nil {
		t.Fatal("runBrowserCmd(missing binary) = nil, want an error")
	}
}

// TestRunBrowserCmd_StillRunningIsSuccess covers the opener that becomes the
// browser and never exits: past the wait window it must be reported as success
// rather than blocking the caller for the whole session.
func TestRunBrowserCmd_StillRunningIsSuccess(t *testing.T) {
	setBrowserWaitWindow(t, 50*time.Millisecond)
	warnings := captureBrowserWarnings(t)

	cmd := helperCmd(time.Minute, 1)
	if err := runBrowserCmd(cmd); err != nil {
		t.Errorf("runBrowserCmd(long-running) = %v, want nil", err)
	}

	// The watcher goroutine outlives runBrowserCmd by design, so this test has to
	// retire it before returning: killing the child and waiting for the resulting
	// warning is what keeps it from writing into the next test's capture.
	_ = cmd.Process.Kill()
	waitForWarning(t, warnings, "failed after being treated as successful")
}

// TestRunBrowserCmd_LateFailureWarns covers the watcher goroutine: an opener
// that outlives the wait window is reported as success, but if it then fails the
// user still gets told. Without this the hang-then-fail case would be silent,
// since the caller has already stopped trying other candidates.
func TestRunBrowserCmd_LateFailureWarns(t *testing.T) {
	setBrowserWaitWindow(t, 50*time.Millisecond)
	warnings := captureBrowserWarnings(t)

	if err := runBrowserCmd(helperCmd(300*time.Millisecond, 7)); err != nil {
		t.Fatalf("runBrowserCmd(outlives window) = %v, want nil", err)
	}
	waitForWarning(t, warnings, "failed after being treated as successful")
	waitForWarning(t, warnings, "exit status 7")
}

func TestOpenBrowserCandidates_FallsThroughToNextCandidate(t *testing.T) {
	setBrowserWaitWindow(t, 30*time.Second)
	setHelperEnv(t, 0, 0)
	err := openBrowserCandidates([][]string{
		{"ocr-viewer-no-such-browser-opener"},
		helperArgv(),
	})
	if err != nil {
		t.Errorf("openBrowserCandidates(missing, working) = %v, want nil", err)
	}
}

// TestOpenBrowserCandidates_SkipsNonZeroExit proves the fallthrough is driven by
// exit status, not just by whether the binary exists.
func TestOpenBrowserCandidates_SkipsNonZeroExit(t *testing.T) {
	setBrowserWaitWindow(t, 30*time.Second)
	setHelperEnv(t, 0, 4)
	err := openBrowserCandidates([][]string{helperArgv()})
	if err == nil {
		t.Fatal("openBrowserCandidates(exit 4) = nil, want an error")
	}
}

// TestOpenBrowserCandidates_AllFail pins that *every* candidate's failure is
// reported. Naming only $BROWSER would suggest fixing that variable is enough
// when the platform opener is missing too.
func TestOpenBrowserCandidates_AllFail(t *testing.T) {
	err := openBrowserCandidates([][]string{
		{"ocr-viewer-no-such-browser-opener"},
		{"ocr-viewer-no-such-browser-opener-either"},
	})
	if err == nil {
		t.Fatal("openBrowserCandidates(all missing) = nil, want an error")
	}
	for _, want := range []string{
		"ocr-viewer-no-such-browser-opener:",
		"ocr-viewer-no-such-browser-opener-either:",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestOpenBrowserCandidates_Empty(t *testing.T) {
	err := openBrowserCandidates(nil)
	if err == nil || !strings.Contains(err.Error(), "no browser opener available") {
		t.Errorf("openBrowserCandidates(nil) = %v, want a no-opener error", err)
	}
}

// TestOpenBrowser covers the env-reading wrapper without launching a real
// browser: $BROWSER is the first candidate on Unix, so pointing it at the helper
// means openBrowser returns before it ever reaches the platform default.
func TestOpenBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("$BROWSER is not consulted on Windows, so the platform opener would really run")
	}
	self := os.Args[0]
	if strings.ContainsAny(self, " :") {
		t.Skipf("test binary path %q cannot round-trip through $BROWSER's separators", self)
	}
	setBrowserWaitWindow(t, 30*time.Second)
	setHelperEnv(t, 0, 0)
	t.Setenv("BROWSER", self+" -test.run=^TestBrowserHelperProcess$ %s")

	if err := openBrowser("http://localhost:5483"); err != nil {
		t.Errorf("openBrowser = %v, want nil", err)
	}
}

// TestShouldAutoOpen covers the env-reading wrapper. Only the two modes that
// short-circuit before any environment probe are deterministic here: under
// `go test` stdout is not a terminal, so auto always declines.
func TestShouldAutoOpen(t *testing.T) {
	if open, reason := shouldAutoOpen(OpenNever); open || reason != "" {
		t.Errorf("shouldAutoOpen(never) = (%v, %q), want (false, \"\")", open, reason)
	}
	if open, reason := shouldAutoOpen(OpenAlways); !open || reason != "" {
		t.Errorf("shouldAutoOpen(always) = (%v, %q), want (true, \"\")", open, reason)
	}
	open, reason := shouldAutoOpen(OpenAuto)
	if open {
		t.Error("shouldAutoOpen(auto) = true under go test, want false (stdout is not a tty)")
	}
	if reason == "" {
		t.Error("shouldAutoOpen(auto) gave no reason; the ready line would not explain the suppression")
	}
}
