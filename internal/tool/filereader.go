// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/vcs"
	vcsgit "github.com/alibaba/open-code-review/internal/vcs/git"
)

// ReviewMode represents the legacy review mode used by callers without Source.
type ReviewMode int

const (
	ModeWorkspace ReviewMode = iota
	ModeRange
	ModeCommit
	ModeEmpty
)

func ParseReviewMode(from, to, commit string) ReviewMode {
	if commit != "" {
		return ModeCommit
	}
	if from != "" && to != "" {
		return ModeRange
	}
	return ModeWorkspace
}

func (m ReviewMode) RefValue(toRef, commit string) (string, bool) {
	switch m {
	case ModeRange:
		return toRef, true
	case ModeCommit:
		return commit, true
	default:
		return "", false
	}
}

// FileReader resolves content through the selected repository provider. Content
// is the resolved version; legacy fields remain for callers without Source.
type FileReader struct {
	Source  vcs.FileSource
	Content vcs.ContentRef
	RepoDir string
	Mode    ReviewMode
	Ref     string
	Runner  *gitcmd.Runner
}

func (fr *FileReader) fileSource() (vcs.FileSource, vcs.ContentRef, error) {
	if fr.Source != nil {
		if (fr.Mode == ModeRange || fr.Mode == ModeCommit) && fr.Content.IsWorkspace() {
			return nil, fr.Content, fmt.Errorf("historical file read requires a revision")
		}
		return fr.Source, fr.Content, nil
	}
	content := fr.Content
	if content.IsWorkspace() && (fr.Mode == ModeRange || fr.Mode == ModeCommit) {
		if fr.Ref == "" {
			return nil, content, fmt.Errorf("historical file read requires a revision")
		}
		content.Revision = fr.Ref
	}
	return vcsgit.New(fr.RepoDir, fr.Runner), content, nil
}

// Read returns the full content of a repository-relative file.
func (fr *FileReader) Read(ctx context.Context, path string) (string, error) {
	if fr.Mode == ModeEmpty {
		return "", fmt.Errorf("file %q does not exist in the empty tree", path)
	}
	source, content, err := fr.fileSource()
	if err != nil {
		return "", err
	}
	var data []byte
	err = source.ReadFile(ctx, content, path, func(reader io.Reader) error {
		var readErr error
		data, readErr = io.ReadAll(reader)
		return readErr
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReadLines collects a window of lines while counting the complete file.
func (fr *FileReader) ReadLines(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	if fr.Mode == ModeEmpty {
		return nil, 0, fmt.Errorf("file %q does not exist in the empty tree", path)
	}
	source, content, err := fr.fileSource()
	if err != nil {
		return nil, 0, err
	}
	var lines []string
	var total int
	err = source.ReadFile(ctx, content, path, func(reader io.Reader) error {
		var readErr error
		lines, total, readErr = scanLines(reader, startLine, maxLines)
		return readErr
	})
	if err != nil {
		return nil, 0, err
	}
	return lines, total, nil
}

// scanLines reads from r line by line, collecting at most maxLines lines
// starting from startLine (1-based), while counting the total number of lines.
// The behavior matches strings.Split(content, "\n") for trailing-newline files.
func scanLines(r io.Reader, startLine, maxLines int) ([]string, int, error) {
	br := bufio.NewReader(r)
	var collected []string
	lineNum := 0
	lastHadNewline := false

	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lineNum++
			lastHadNewline = line[len(line)-1] == '\n'
			trimmed := strings.TrimSuffix(line, "\n")
			trimmed = strings.TrimSuffix(trimmed, "\r")
			if lineNum >= startLine && len(collected) < maxLines {
				collected = append(collected, trimmed)
			}
		}
		if err != nil {
			if err != io.EOF {
				return nil, 0, err
			}
			break
		}
	}

	if lastHadNewline {
		lineNum++
		if lineNum >= startLine && len(collected) < maxLines {
			collected = append(collected, "")
		}
	}

	return collected, lineNum, nil
}
