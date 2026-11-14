// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package lore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/vcs"
)

func validPath(name string) bool {
	if name == "." || !fs.ValidPath(name) || !filepath.IsLocal(filepath.FromSlash(name)) || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for component := range strings.SplitSeq(name, "/") {
		if strings.EqualFold(component, ".lore") {
			return false
		}
	}
	return true
}

func (r *Repository) ReadFile(ctx context.Context, ref vcs.ContentRef, name string, consume func(io.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validPath(name) || (r.scope != "" && !validPath(r.scope)) {
		return fmt.Errorf("invalid Lore file path %q", name)
	}
	name = path.Join(r.scope, name)
	if ref.IsWorkspace() {
		root, err := os.OpenRoot(r.root)
		if err != nil {
			return err
		}
		defer root.Close()
		// Refuse links, including parent components, to keep administrative
		// files and non-regular entries out of the review.
		for p := name; p != "."; p = path.Dir(p) {
			info, err := root.Lstat(filepath.FromSlash(p))
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || (p == name && !info.Mode().IsRegular()) {
				return fmt.Errorf("not a regular Lore file: %s", name)
			}
		}
		file, err := root.Open(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		defer file.Close()
		return consume(file)
	}
	// lore file write accepts an output filename, not a stdout stream.
	dir, err := os.MkdirTemp("", "ocr-lore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "content")
	if err := r.events(ctx, nil, "file", "write", "--revision", ref.Revision, "--path", "./"+name, "--output", output); err != nil {
		return err
	}
	f, err := os.Open(output)
	if err != nil {
		return err
	}
	defer f.Close()
	return consume(f)
}

func (r *Repository) ListFiles(ctx context.Context, ref vcs.ContentRef) ([]string, error) {
	revision := ref.Revision
	if ref.IsWorkspace() {
		rev, err := r.revision(ctx, "")
		if err != nil {
			return nil, err
		}
		revision = rev.Revision
	}
	names := map[string]bool{}
	scoped := func(name string) bool {
		return validPath(name) && (r.scope == "" || strings.HasPrefix(name, r.scope+"/"))
	}
	if revision != zeroRevision {
		err := r.events(ctx, func(tag string, data json.RawMessage) error {
			if tag != "repositoryStateDumpNode" {
				return nil
			}
			var node struct {
				Name  string
				Flags uint32
			}
			if err := json.Unmarshal(data, &node); err != nil {
				return err
			}
			if node.Flags&1 != 0 && scoped(node.Name) {
				names[node.Name] = true
			}
			return nil
		}, "repository", "dump", "--revision", revision)
		if err != nil {
			return nil, err
		}
	}
	if ref.IsWorkspace() {
		err := r.events(ctx, func(tag string, data json.RawMessage) error {
			if tag != "repositoryStatusFile" {
				return nil
			}
			var change struct{ Path, Type string }
			if err := json.Unmarshal(data, &change); err != nil {
				return err
			}
			if change.Type == "file" && scoped(change.Path) {
				names[change.Path] = true
			}
			return nil
		}, "--dry-run", "status", "--scan")
		if err != nil {
			return nil, err
		}
	}
	files := make([]string, 0, len(names))
	for name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ref.IsWorkspace() {
			info, err := os.Lstat(filepath.Join(r.root, filepath.FromSlash(name)))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
		}
		if r.scope != "" {
			name = strings.TrimPrefix(name, r.scope+"/")
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
}
