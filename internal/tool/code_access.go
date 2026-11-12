// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
)

type CodeAccess struct {
	newReader FileReader
	oldReader FileReader
}

func NewComparisonCodeAccess(current, previous FileReader) (*CodeAccess, error) {
	if current.Ref == "" || (current.Mode != ModeRange && current.Mode != ModeCommit) {
		return nil, fmt.Errorf("comparison code access requires a pinned new version")
	}
	if previous.Mode != ModeEmpty && (previous.Ref == "" || (previous.Mode != ModeRange && previous.Mode != ModeCommit)) {
		return nil, fmt.Errorf("comparison code access requires a pinned old version or an empty tree")
	}
	return &CodeAccess{newReader: current, oldReader: previous}, nil
}

func (access *CodeAccess) execute(ctx context.Context, args map[string]any, current *FileReader, run func(*FileReader) (string, error)) (string, error) {
	rawSide, supplied := args["side"]
	if access == nil {
		if supplied {
			return "", fmt.Errorf("side is not supported for this task")
		}
		return run(current)
	}
	side := "new"
	if supplied {
		var valid bool
		side, valid = rawSide.(string)
		if !valid || (side != "new" && side != "old") {
			return "", fmt.Errorf("side must be new or old")
		}
	}
	if side == "new" {
		return run(&access.newReader)
	}
	prefix := "SIDE: old\nREVISION: " + access.oldReader.Ref + "\n"
	if access.oldReader.Mode == ModeEmpty {
		prefix = "SIDE: old\nSOURCE: empty-tree\n"
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%s%w", prefix, err)
	}
	content, err := run(&access.oldReader)
	if err != nil {
		return "", fmt.Errorf("%s%w", prefix, err)
	}
	return prefix + content, nil
}
