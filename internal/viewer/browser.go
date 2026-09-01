// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// Open mode values accepted by the viewer's --open flag. The three-state shape
// mirrors --color (cmd/opencodereview/color.go) so "auto, but let me override
// the heuristic in both directions" works the same way across the CLI.
const (
	OpenAuto   = "auto"
	OpenAlways = "always"
	OpenNever  = "never"
)

// ValidateOpenMode rejects unknown --open values instead of silently treating
// them as auto, so a typo like --open=yes is reported rather than ignored.
func ValidateOpenMode(mode string) error {
	switch mode {
	case OpenAuto, OpenAlways, OpenNever:
		return nil
	default:
		return fmt.Errorf("invalid --open value %q: must be one of auto, always, never", mode)
	}
}

// browserWarnOut is where browser-opener problems are reported. Failing to open
// a browser never stops the server, so these are warnings, never returned
// errors. It is a variable so tests can observe them without racing on the
// os.Stderr global.
var browserWarnOut io.Writer = os.Stderr

// browserWarnf emits one warning using the "[ocr] WARNING:" prefix the rest of
// the codebase uses for non-fatal problems.
func browserWarnf(format string, args ...any) {
	fmt.Fprintf(browserWarnOut, "[ocr] WARNING: "+format+"\n", args...)
}

// browserWaitWindow bounds how long runBrowserCmd waits for an opener to exit.
// An opener that hands the URL to an already-running browser exits promptly and
// its status is meaningful; one that becomes the browser holds the process for
// the whole session. Waiting only this long distinguishes the two without
// blocking the warning path forever.
var browserWaitWindow = 3 * time.Second

// shouldAutoOpen resolves the open decision against the live environment.
func shouldAutoOpen(mode string) (bool, string) {
	return shouldAutoOpenEnv(
		mode,
		term.IsTerminal(os.Stdout.Fd()),
		os.Getenv("SSH_CONNECTION"),
		os.Getenv("DISPLAY"),
		os.Getenv("WAYLAND_DISPLAY"),
		runtime.GOOS,
	)
}

// shouldAutoOpenEnv reports whether to open a browser and, when auto mode
// declines, why. The reason is surfaced on the ready line: a suppressed
// auto-open is otherwise indistinguishable from a broken one, which is exactly
// the situation of the remote and headless users the heuristic protects.
//
// Being on a remote host only rules out a browser when nothing is forwarded, so
// SSH_CONNECTION is judged together with the display variables rather than on
// its own — that keeps `ssh -X` working without an explicit override. What auto
// still cannot detect is a non-terminal stdout that nonetheless has a desktop
// (a pipe, an IDE runner) or a WSL host reaching the browser through wslu with
// no DISPLAY at all; OpenAlways exists for those and skips every check.
func shouldAutoOpenEnv(mode string, stdoutTTY bool, sshConn, display, wayland, goos string) (bool, string) {
	switch mode {
	case OpenNever:
		return false, ""
	case OpenAlways:
		return true, ""
	}
	if !stdoutTTY {
		return false, "stdout is not a terminal"
	}
	hasDisplay := display != "" || wayland != ""
	if sshConn != "" && !hasDisplay {
		return false, "SSH session with no forwarded display"
	}
	if goos == "linux" && !hasDisplay {
		return false, "no DISPLAY or WAYLAND_DISPLAY"
	}
	return true, ""
}

// browserCandidates returns the argv lists to try, in preference order.
//
// $BROWSER comes first when set, following the freedesktop convention the Go
// toolchain also honors (cmd/internal/browser): a colon-separated list of
// commands, each either embedding a %s placeholder for the URL or receiving it
// as a trailing argument. It is read on Unix only — the separator would split
// a Windows path at its drive letter, and the variable is not a Windows idiom.
//
// Reference Gist: https://gist.github.com/sevkin/9798d67b2cb9d07cb05f89f14ba682f8
func browserCandidates(goos, browserEnv, url string) [][]string {
	var out [][]string
	if goos != "windows" {
		for _, entry := range strings.Split(browserEnv, ":") {
			if argv := browserEnvArgv(entry, url); argv != nil {
				out = append(out, argv)
			}
		}
	}

	switch goos {
	case "windows":
		// rundll32 passes the URL through verbatim, where "cmd /c start" would
		// treat & as a command separator and needs an extra empty title argument.
		out = append(out, []string{"rundll32", "url.dll,FileProtocolHandler", url})
	case "darwin":
		out = append(out, []string{"open", url})
	default: // "linux", "freebsd", "openbsd", "netbsd"
		out = append(out, []string{"xdg-open", url})
	}
	return out
}

// browserEnvArgv turns one $BROWSER entry into an argv, or nil if it is blank.
func browserEnvArgv(entry, url string) []string {
	fields := strings.Fields(entry)
	if len(fields) == 0 {
		return nil
	}
	substituted := false
	for i, f := range fields {
		if strings.Contains(f, "%s") {
			fields[i] = strings.ReplaceAll(f, "%s", url)
			substituted = true
		}
	}
	if !substituted {
		fields = append(fields, url)
	}
	return fields
}

// runBrowserCmd starts one opener and reports whether it worked. Checking only
// Start() is not enough: it returns as soon as fork/exec succeeds, while
// xdg-open exits non-zero when no handler is registered and open exits non-zero
// when no application claims the scheme. Both would otherwise leave the user
// with neither a browser nor a warning.
func runBrowserCmd(cmd *exec.Cmd) error {
	// Without this the child's stderr goes to /dev/null and a failure reduces to
	// "exit status 3", which says nothing about why. Only the branch below that
	// waits for Wait() reads it back: at that point exec has finished copying, so
	// the read cannot race the writer.
	var stderr strings.Builder
	if cmd.Stderr == nil {
		cmd.Stderr = &stderr
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return fmt.Errorf("%w: %s", err, msg)
			}
		}
		return err
	case <-time.After(browserWaitWindow):
		// Still running: it took over as the browser. Report success now, but keep
		// watching — an opener that hangs and then fails would otherwise look
		// indistinguishable from one that worked. Deliberately does not touch
		// stderr, which the writer still owns.
		name := cmd.Path
		go func() {
			if err := <-done; err != nil {
				browserWarnf("browser opener %s failed after being treated as successful: %v", name, err)
			}
		}()
		return nil
	}
}

// openBrowser resolves the candidate list for this host and runs it.
func openBrowser(url string) error {
	return openBrowserCandidates(browserCandidates(runtime.GOOS, os.Getenv("BROWSER"), url))
}

// openBrowserCandidates tries each candidate in turn. Every failure is reported,
// not just the first: with $BROWSER set to something uninstalled and no platform
// opener present either, hearing only about $BROWSER would suggest fixing that
// variable is enough.
func openBrowserCandidates(candidates [][]string) error {
	var errs []error
	for _, argv := range candidates {
		err := runBrowserCmd(exec.Command(argv[0], argv[1:]...))
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", argv[0], err))
	}
	if len(errs) == 0 {
		return errors.New("no browser opener available")
	}
	return errors.Join(errs...)
}
