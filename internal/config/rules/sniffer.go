// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// sniffTimeout bounds a single content peek. A peek is a best-effort hint, so
// exceeding it yields "" (no sniff) rather than failing the whole resolution.
const sniffTimeout = 5 * time.Second

// systemLayer is the subset of *SystemRule that composedResolver depends on.
// Declaring it lets the lowest layer be decorated (see sniffer) without any
// caller — or the Resolver interface itself — changing.
type systemLayer interface {
	Resolve(path string) string
	resolveDetail(path string) RuleDetail
	CanonicalConfig() []string
}

// sniffer decorates the system rule layer to disambiguate the ".m" extension,
// which MATLAB and Objective-C both use. system_rules.json maps "**/*.m" to
// matlab.md; when a ".m" file's first non-blank line looks like Objective-C,
// this returns objc.md instead.
//
// It wraps the *system layer* rather than the composed resolver on purpose:
// user-configured layers (custom / project / global) must keep outranking the
// system layer, including when a user rule sets merge_system_rule. Decorating
// the outermost resolver would let the sniff discard a user's own ".m" rule.
//
// It is deliberately stateless (no peek cache): Resolve is called from the
// concurrent per-file review goroutines, so caching would need a mutex, and a
// peek costs at most one read per ".m" file.
type sniffer struct {
	inner    systemLayer
	repoDir  string
	ref      string // git ref to read at; "" reads the working tree
	runner   *gitcmd.Runner
	objcRule string // rule_docs/objc.md
}

// Resolve returns objc.md for a ".m" file whose content sniffs as
// Objective-C, and otherwise defers to the wrapped system layer.
func (s *sniffer) Resolve(path string) string {
	if s.sniffsAsObjC(path) {
		return s.objcRule
	}
	return s.inner.Resolve(path)
}

// resolveDetail mirrors Resolve while preserving the wrapped layer's matched
// Pattern verbatim (it stays a plain glob) and recording the override in
// SniffedAs instead, so callers that need to know can check it without a
// serialized field silently changing shape.
func (s *sniffer) resolveDetail(path string) RuleDetail {
	detail := s.inner.resolveDetail(path)
	if s.sniffsAsObjC(path) {
		detail.Rule = s.objcRule
		detail.SniffedAs = "objc"
	}
	return detail
}

// CanonicalConfig forwards the wrapped layer's fields and appends the objc
// rule text, so editing objc.md changes the run manifest's rule_config_sha256.
func (s *sniffer) CanonicalConfig() []string {
	fields := s.inner.CanonicalConfig()
	if s.objcRule != "" {
		fields = append(fields, "layer", "system", "objc", s.objcRule)
	}
	return fields
}

// sniffsAsObjC reports whether path is a ".m" file whose content identifies it
// as Objective-C. Non-".m" paths never trigger a read.
func (s *sniffer) sniffsAsObjC(path string) bool {
	if !strings.HasSuffix(strings.ToLower(path), ".m") {
		return false
	}
	return looksLikeObjC(s.peekFirstLine(path))
}

// peekFirstLine returns the first non-blank line of path, read at s.ref when
// set (so refs that are not checked out still resolve correctly) and from the
// working tree otherwise. Any failure yields "", which leaves the path-based
// match — matlab.md — in place.
func (s *sniffer) peekFirstLine(path string) string {
	if s.ref != "" {
		return firstNonBlankLine(s.showAtRef(path))
	}

	f, err := os.Open(filepath.Join(s.repoDir, path))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			return line
		}
	}
	return ""
}

// showAtRef reads path's content at s.ref via `git show <ref>:<path>`.
// Returns "" when the file does not exist at that ref or git fails.
func (s *sniffer) showAtRef(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), sniffTimeout)
	defer cancel()

	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", s.ref + ":" + path}

	if s.runner != nil {
		out, err := s.runner.Output(ctx, s.repoDir, args...)
		if err != nil {
			return ""
		}
		return string(out)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.repoDir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return stdout.String()
}

// objcSniffPrefixes are first-line signals for Objective-C. MATLAB comments
// start with "%" and a MATLAB file cannot legally begin with "/" at all, so a
// C-style comment opener is itself a reliable ObjC signal — which matters
// because the Xcode file template and most license headers put a comment, not
// a directive, on line 1. "#if" covers "#ifdef"/"#ifndef" too (both start
// with it) as well as a bare platform guard like "#if TARGET_OS_IPHONE".
// Deliberately not widened to a bare "#": Octave, which also uses ".m",
// treats "#" as a comment character, so that would misclassify a real
// Octave/MATLAB file.
var objcSniffPrefixes = []string{
	"#import", "#include", "#pragma", "#if", "#define",
	"@import", "@interface", "@implementation", "@class", "@protocol",
	"//", "/*",
}

// looksLikeObjC reports whether the first non-blank line of content looks
// like Objective-C rather than MATLAB. It returns false (keeping the default
// MATLAB behavior) when content is empty or inconclusive.
func looksLikeObjC(firstLine string) bool {
	if firstLine == "" {
		return false
	}
	for _, prefix := range objcSniffPrefixes {
		if strings.HasPrefix(firstLine, prefix) {
			return true
		}
	}
	return false
}

func firstNonBlankLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
