// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/tool"
)

const (
	invalidPathRecoveryThreshold = 3
	invalidPathRefundLimit       = 3
	invalidPathCandidateLimit    = 8
)

type invalidPathRecovery struct {
	pending        map[string]int
	order          []string
	consecutive    int
	refundedRounds int
	candidateCache map[string][]string
}

func newInvalidPathRecovery() *invalidPathRecovery {
	return &invalidPathRecovery{
		pending:        make(map[string]int),
		candidateCache: make(map[string][]string),
	}
}

func (s *invalidPathRecovery) observe(rawPath string) {
	path := normalizeRecoveryPath(rawPath)
	if _, ok := s.pending[path]; !ok {
		s.order = append(s.order, path)
	}
	s.pending[path]++
	s.consecutive++
}

func (s *invalidPathRecovery) ready() bool {
	return s.consecutive >= invalidPathRecoveryThreshold
}

func (s *invalidPathRecovery) resetPending() {
	s.pending = make(map[string]int)
	s.order = nil
	s.consecutive = 0
}

func (s *invalidPathRecovery) finishBatch() {
	s.resetPending()
}

func (s *invalidPathRecovery) refundRound() bool {
	if s.refundedRounds >= invalidPathRefundLimit {
		return false
	}
	s.refundedRounds++
	return true
}

func normalizeRecoveryPath(raw string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/")
	cleaned := pathpkg.Clean(normalized)
	if cleaned == "." {
		return normalized
	}
	return strings.TrimPrefix(cleaned, "./")
}

func pathRecoveryQueries(path string) []string {
	base := pathpkg.Base(normalizeRecoveryPath(path))
	if base == "." || base == "/" || base == "" {
		return nil
	}
	queries := []string{base}
	ext := pathpkg.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if ext != "" && stem != "" && stem != base {
		queries = append(queries, stem)
	}
	return queries
}

func (r *Runner) recoverInvalidPaths(ctx context.Context, state *invalidPathRecovery) string {
	var out strings.Builder
	out.WriteString("OCR path recovery: repeated file_read requests referenced paths absent from the reviewed ref.\n")
	finder := lookupTool(r.deps.Tools, tool.FileFind)

	for _, rejectedPath := range state.order {
		attempts := state.pending[rejectedPath]
		fmt.Fprintf(&out, "- Rejected %q (%d attempt", rejectedPath, attempts)
		if attempts != 1 {
			out.WriteByte('s')
		}
		out.WriteString(").\n")

		candidates, cached := state.candidateCache[rejectedPath]
		if !cached {
			var err error
			candidates, err = findPathCandidates(ctx, finder, rejectedPath)
			if err != nil {
				out.WriteString("  Candidate search failed; continue reviewing the current diff.\n")
				continue
			}
			state.candidateCache[rejectedPath] = candidates
		}
		if len(candidates) == 0 {
			out.WriteString("  No candidate path was found in the reviewed ref.\n")
			continue
		}
		out.WriteString("  Candidate paths in the reviewed ref:\n")
		for _, candidate := range candidates {
			fmt.Fprintf(&out, "  - %s\n", candidate)
		}
	}
	out.WriteString("Use a candidate only if it is relevant; otherwise continue reviewing the current diff. Do not retry a rejected path.")
	return out.String()
}

func findPathCandidates(ctx context.Context, finder tool.Provider, rejectedPath string) ([]string, error) {
	if finder == nil {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var candidates []string
	for _, query := range pathRecoveryQueries(rejectedPath) {
		result, err := finder.Execute(ctx, map[string]any{
			"query_name":     query,
			"case_sensitive": false,
		})
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(result, "\n") {
			candidate := strings.TrimSpace(line)
			if candidate == "" || strings.HasPrefix(candidate, "//") {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			candidates = append(candidates, candidate)
			if len(candidates) >= invalidPathCandidateLimit {
				break
			}
		}
		if len(candidates) > 0 {
			break
		}
	}
	sort.Strings(candidates)
	return candidates, nil
}
