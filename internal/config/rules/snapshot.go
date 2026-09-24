// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// loadProjectRuleAtRef never falls back to disk, even when the project rule is
// absent from the captured tree. Referenced documents are root-relative, just
// as they are for the working-tree project layer, and must be regular blobs.
func loadProjectRuleAtRef(repoDir, ref string, runner *gitcmd.Runner) (*ProjectRule, error) {
	if runner == nil {
		runner = gitcmd.New(1)
	}
	read := func(name string) ([]byte, bool, error) {
		// Bound each file independently; a long rule list must not consume the
		// deadline available to documents later in the list.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return readSnapshotRule(ctx, repoDir, ref, name, runner)
	}
	data, exists, err := read(".opencodereview/rule.json")
	if err != nil || !exists {
		return nil, err
	}
	var pr ProjectRule
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot project rule: %w", err)
	}
	for i := range pr.Rules {
		e := &pr.Rules[i]
		if strings.TrimSpace(e.Rule) == "" || !looksLikeFilePath(e.Rule) {
			continue
		}
		content, exists, err := read(e.Rule)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("snapshot rule document %q does not exist in the snapshot tree", e.Rule)
		}
		e.Rule = strings.TrimRight(string(content), "\n")
	}
	return &pr, nil
}

func readSnapshotRule(ctx context.Context, repoDir, ref, name string, runner *gitcmd.Runner) ([]byte, bool, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") ||
		strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return nil, false, fmt.Errorf("snapshot rule path %q must be repository-relative", name)
	}
	run := func(args ...string) ([]byte, error) {
		out, stderr, err := runner.RunSplit(ctx, repoDir, args...)
		if detail := strings.TrimSpace(stderr); err != nil && detail != "" {
			err = fmt.Errorf("%w: %s", err, detail)
		}
		return []byte(out), err
	}
	out, err := run("ls-tree", "-l", "-z", "--full-tree", "--end-of-options", ref, "--", ":(literal)"+name)
	if err != nil {
		return nil, false, fmt.Errorf("locate snapshot rule %q: %w", name, err)
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	header, entryPath, ok := bytes.Cut(bytes.TrimSuffix(out, []byte{0}), []byte{'\t'})
	fields := strings.Fields(string(header))
	if !ok || string(entryPath) != name || len(fields) != 4 ||
		(fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" {
		return nil, false, fmt.Errorf("snapshot rule %q must be a regular file", name)
	}
	const maxSize = 512 * 1024
	size, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || size < 0 || size > maxSize {
		return nil, false, fmt.Errorf("snapshot rule %q exceeds the %d-byte limit", name, maxSize)
	}
	data, err := run("cat-file", "blob", fields[2])
	if err != nil {
		return nil, false, fmt.Errorf("read snapshot rule %q: %w", name, err)
	}
	return data, true, nil
}
