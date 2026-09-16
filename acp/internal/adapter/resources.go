// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	acp "github.com/coder/acp-go-sdk"
)

// Resources select local scan targets; they are never fetched or interpreted
// as instructions, and never silently narrow a repository-wide review.
func promptContent(cwd string, blocks []acp.ContentBlock) (string, []string, error) {
	if len(blocks) > 128 {
		return "", nil, fmt.Errorf("too many prompt blocks (maximum 128)")
	}
	var texts, paths []string
	size := 0
	for _, block := range blocks {
		switch {
		case block.Text != nil:
			size += len(block.Text.Text)
			texts = append(texts, block.Text.Text)
		case block.ResourceLink != nil:
			uri := block.ResourceLink.Uri
			size += len(uri)
			if len(uri) > 8192 {
				return "", nil, fmt.Errorf("resource URI is too long")
			}
			u, err := url.Parse(uri)
			if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !filepath.IsAbs(u.Path) {
				return "", nil, fmt.Errorf("resources must be local absolute file URIs without query or fragment")
			}
			location := findingLocation(cwd, contract.Comment{Path: u.Path})
			if location == nil {
				return "", nil, fmt.Errorf("resource must be an existing file inside the session directory")
			}
			root, err := filepath.EvalSymlinks(cwd)
			if err != nil {
				return "", nil, fmt.Errorf("session directory is unavailable")
			}
			path, err := filepath.Rel(root, location.Path)
			if err != nil {
				return "", nil, fmt.Errorf("resource cannot be resolved relative to the session directory")
			}
			// OCR normalizes leading/trailing whitespace in --path values.
			// Reject such names instead of allowing the CLI to select a
			// different file than the client requested.
			if path != strings.TrimSpace(path) {
				return "", nil, fmt.Errorf("resource path has leading or trailing whitespace")
			}
			paths = append(paths, path)
		default:
			return "", nil, fmt.Errorf("unsupported content: send text or a local file resource link")
		}
		if size > 64*1024 {
			return "", nil, fmt.Errorf("prompt exceeds 64 KiB")
		}
	}
	return strings.Join(texts, "\n"), paths, nil
}
