// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"sync"

	"github.com/alibaba/open-code-review/internal/model"
)

// CommentCollector is a thread-safe, per-Agent comment store.
// Each Agent instance owns its own collector so reviews across different repos do not interfere.
type CommentCollector struct {
	mu       sync.Mutex
	comments []model.LlmComment
	byPath   map[string][]model.LlmComment
}

// NewCommentCollector creates an empty collector.
func NewCommentCollector() *CommentCollector {
	return &CommentCollector{}
}

// Add appends a comment to the collector.
func (c *CommentCollector) Add(cm model.LlmComment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byPath == nil {
		c.byPath = make(map[string][]model.LlmComment)
	}
	c.byPath[cm.Path] = append(c.byPath[cm.Path], cm)
	c.comments = append(c.comments, cm)
}

// Comments returns all collected comments.
func (c *CommentCollector) Comments() []model.LlmComment {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]model.LlmComment, len(c.comments))
	copy(out, c.comments)
	return out
}

// CommentsForPath returns a copy of comments whose Path matches the given path.
func (c *CommentCollector) CommentsForPath(path string) []model.LlmComment {
	c.mu.Lock()
	defer c.mu.Unlock()
	comments := c.byPath[path]
	if len(comments) == 0 {
		return nil
	}
	out := make([]model.LlmComment, len(comments))
	copy(out, comments)
	return out
}

// Snapshot returns the current count of stored comments. Pair with Since /
// ReplaceSince to operate on the comments added between two points in time
// (e.g. before / after a scan batch).
func (c *CommentCollector) Snapshot() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.comments)
}

// Since returns a copy of all comments stored at index ≥ start. Returns nil
// when no new comments have been added since the snapshot.
func (c *CommentCollector) Since(start int) []model.LlmComment {
	c.mu.Lock()
	defer c.mu.Unlock()
	if start < 0 {
		start = 0
	}
	if start >= len(c.comments) {
		return nil
	}
	out := make([]model.LlmComment, len(c.comments)-start)
	copy(out, c.comments[start:])
	return out
}

// ReplaceSince replaces comments[start:] with the given replacements.
// Useful for batch-level dedup: take a Snapshot, run a batch, dedup the
// new comments, then apply the deduped list back. Indices ≥ len(comments)
// are ignored (no-op).
func (c *CommentCollector) ReplaceSince(start int, replacements []model.LlmComment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if start < 0 {
		start = 0
	}
	if start > len(c.comments) {
		return
	}
	for i := len(c.comments) - 1; i >= start; i-- {
		path := c.comments[i].Path
		comments := c.byPath[path]
		comments[len(comments)-1] = model.LlmComment{}
		if len(comments) == 1 {
			delete(c.byPath, path)
		} else {
			c.byPath[path] = comments[:len(comments)-1]
		}
	}
	c.comments = append(c.comments[:start:start], replacements...)
	if len(replacements) > 0 && c.byPath == nil {
		c.byPath = make(map[string][]model.LlmComment)
	}
	for _, cm := range replacements {
		c.byPath[cm.Path] = append(c.byPath[cm.Path], cm)
	}
}

// RemoveByPathAndIndices removes comments for a given path whose per-path index
// (0-based position among all comments with that path) is in the indices set.
func (c *CommentCollector) RemoveByPathAndIndices(path string, indices map[int]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.comments[:0]
	pathIdx := 0
	for _, cm := range c.comments {
		if cm.Path == path {
			if _, remove := indices[pathIdx]; remove {
				pathIdx++
				continue
			}
			pathIdx++
		}
		kept = append(kept, cm)
	}
	tail := c.comments[len(kept):]
	for i := range tail {
		tail[i] = model.LlmComment{}
	}
	c.comments = kept
	if len(tail) > 0 {
		comments := c.byPath[path]
		keptPath := comments[:0]
		for i, cm := range comments {
			if _, remove := indices[i]; !remove {
				keptPath = append(keptPath, cm)
			}
		}
		clear(comments[len(keptPath):])
		if len(keptPath) == 0 {
			delete(c.byPath, path)
		} else {
			c.byPath[path] = keptPath
		}
	}
}
