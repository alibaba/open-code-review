// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os/exec"
	"path"
	"strings"
	"time"
)

type FileReadEvidence struct {
	TargetCommit  string
	CandidatePath string
	Read          *FileReadContent
}

type FileReadContent struct {
	FilePath    string `json:"file_path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	TotalLines  int    `json:"total_lines"`
	IsTruncated bool   `json:"is_truncated"`
}

func (fr *FileReader) evidenceTarget() string {
	if fr.Mode != ModeCommit && fr.Mode != ModeRange {
		return ""
	}
	// A symbolic ref could move between the failed and successful calls.
	if len(fr.Ref) != 40 && len(fr.Ref) != 64 {
		return ""
	}
	for _, c := range fr.Ref {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return ""
		}
	}
	return fr.Ref
}

func (fr *FileReader) readCandidate(parentCtx context.Context, filePath string) string {
	if filePath == "" || path.IsAbs(filePath) || path.Clean(filePath) != filePath ||
		filePath == ".." || strings.HasPrefix(filePath, "../") || strings.ContainsAny(filePath, "\\\x00") {
		return ""
	}
	var candidate string
	switch path.Ext(filePath) {
	case ".ts":
		candidate = filePath + "x"
	case ".tsx":
		candidate = strings.TrimSuffix(filePath, "x")
	default:
		return ""
	}

	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
	defer cancel()
	args := []string{"--literal-pathspecs", "ls-tree", "-z", "--full-tree", "--end-of-options", fr.Ref, "--", filePath, candidate}
	var output []byte
	var err error
	if fr.Runner != nil {
		output, err = fr.Runner.Output(ctx, fr.RepoDir, args...)
	} else {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = fr.RepoDir
		output, err = cmd.Output()
	}
	if err != nil {
		return ""
	}

	foundCandidate := false
	// A read error can also mean cancellation or I/O failure. The tree must
	// establish absence before an alternate path can be treated as a candidate.
	for _, entry := range strings.Split(string(output), "\x00") {
		if entry == "" {
			continue
		}
		header, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || name == filePath {
			return ""
		}
		if name == candidate && fields[1] == "blob" && (fields[0] == "100644" || fields[0] == "100755") {
			foundCandidate = true
		}
	}
	if foundCandidate {
		return candidate
	}
	return ""
}
