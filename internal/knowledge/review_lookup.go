// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var reviewLookupSymbol = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_]*$`)

// LookupReviewEvidence samples literal uses of a symbol in the selected Git
// view. A missing sample cannot establish that a symbol or behavior is absent.
func (t Tree) LookupReviewEvidence(ctx context.Context, area, symbol string, near ...int) ([]Citation, error) {
	if !validScope(area) || len(symbol) > 128 || !reviewLookupSymbol.MatchString(symbol) || t.Git == nil || !t.Workspace && !validOID(t.Commit) {
		return nil, errors.New("invalid repository lookup")
	}
	runner, err := t.Git.LocalForRepository(ctx, t.Repo)
	if err != nil {
		return nil, err
	}
	t.Git = runner
	opts := DiscoverOptions{allowPath: t.AllowPath}
	// A named workspace file may be untracked and therefore invisible to grep.
	if opts.allowed(area) {
		if data, err := t.read(ctx, area); err == nil {
			return t.reviewLookupSamples(area, symbol, data, 4, near...), nil
		}
	}
	args := []string{"--literal-pathspecs", "grep", "--no-textconv", "-I", "-l", "-z", "-F", "-e", symbol}
	if !t.Workspace {
		args = append(args, t.Commit)
	}
	raw, _, err := t.discoveryGit(ctx, 12<<10, append(args, "--", area)...)
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil, fmt.Errorf("search reviewed source: %w", err)
	}
	var result []Citation
	read := 0
	for len(raw) != 0 && len(result) < 4 && read < 8 {
		entry, rest, complete := bytes.Cut(raw, []byte{0})
		if !complete {
			break
		}
		raw = rest
		p := string(entry)
		if !t.Workspace {
			var ok bool
			if p, ok = strings.CutPrefix(p, t.Commit+":"); !ok {
				continue
			}
		}
		if !inDiscoveryScope(p, area) || !opts.allowed(p) {
			continue
		}
		read++
		if data, err := t.read(ctx, p); err == nil {
			result = append(result, t.reviewLookupSamples(p, symbol, data, min(2, 4-len(result)))...)
		}
	}
	return result, ctx.Err()
}

func (t Tree) reviewLookupSamples(p, symbol string, data []byte, limit int, near ...int) []Citation {
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) || discoveryCredential.Match(data) {
		return nil
	}
	// A lone CR can make displayed line numbers differ from the reviewed file.
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var result []Citation
	scan := func(first, end, maximum int) {
		last := 0
		for i := first; i < end && len(result) < maximum; i++ {
			if i < last || !discoveryIdentifier(lines[i], symbol) || slices.ContainsFunc(result, func(c Citation) bool {
				return c.StartLine <= i+1 && i+1 <= c.EndLine
			}) {
				continue
			}
			c := reviewSourceSpan(p, lines, max(last+1, i-2), min(len(lines), i+6))
			if len(c.Quote) > 1000 || c.Quote == "" {
				c = reviewSourceSpan(p, lines, i+1, i+1)
			}
			if len(c.Quote) > 1000 || c.Quote == "" || strings.ContainsRune(c.Quote, '\r') {
				continue
			}
			if t.Workspace {
				c.Revision = "workspace"
			} else {
				c.Revision = t.Commit
			}
			result = append(result, c)
			last = c.EndLine
		}
	}
	// Common variable names may occur in many earlier, unrelated functions.
	// Keep a sample at each cited use before filling the remaining file budget.
	for _, line := range near {
		if 0 < line && line <= len(lines) && len(result) < limit {
			scan(max(0, line-4), min(len(lines), line+3), len(result)+1)
		}
	}
	scan(0, len(lines), limit)
	return result
}
