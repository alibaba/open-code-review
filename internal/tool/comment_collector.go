// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"sync"

	"github.com/alibaba/open-code-review/internal/location"
	"github.com/alibaba/open-code-review/internal/model"
)

// CommentCollector is a thread-safe, per-Agent comment store.
// Each Agent instance owns its own collector so reviews across different repos do not interfere.
type CommentCollector struct {
	mu       sync.Mutex
	comments []model.LlmComment
	sides    []location.Side
}

// NewCommentCollector creates an empty collector.
func NewCommentCollector() *CommentCollector {
	return &CommentCollector{}
}

// Add appends a comment to the collector.
func (c *CommentCollector) Add(cm model.LlmComment) {
	c.AddWithSide(cm, location.SideUnknown)
}

// AddWithSide appends a comment and the file side that supplied its location.
func (c *CommentCollector) AddWithSide(cm model.LlmComment, side location.Side) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.comments = append(c.comments, cm)
	c.sides = append(c.sides, side)
}

// Comments returns all collected comments.
func (c *CommentCollector) Comments() []model.LlmComment {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]model.LlmComment, len(c.comments))
	copy(out, c.comments)
	return out
}

// Sides returns the location provenance aligned with Comments.
func (c *CommentCollector) Sides() []location.Side {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]location.Side, len(c.sides))
	copy(out, c.sides)
	return out
}

// CommentsForPath returns a copy of comments whose Path matches the given path.
func (c *CommentCollector) CommentsForPath(path string) []model.LlmComment {
	comments, _ := c.CommentsAndSidesForPath(path)
	return comments
}

// CommentsAndSidesForPath returns comments for path and their aligned source
// sides from one locked snapshot.
func (c *CommentCollector) CommentsAndSidesForPath(path string) ([]model.LlmComment, []location.Side) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var comments []model.LlmComment
	var sides []location.Side
	for i, cm := range c.comments {
		if cm.Path == path {
			comments = append(comments, cm)
			sides = append(sides, sideAt(c.sides, i))
		}
	}
	return comments, sides
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
	c.comments = append(c.comments[:start:start], replacements...)
	c.sides = append(c.sides[:start:start], make([]location.Side, len(replacements))...)
}

// RemoveByPathAndIndices removes comments for a given path whose per-path index
// (0-based position among all comments with that path) is in the indices set.
func (c *CommentCollector) RemoveByPathAndIndices(path string, indices map[int]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	keptComments := c.comments[:0]
	keptSides := c.sides[:0]
	pathIdx := 0
	for i, cm := range c.comments {
		if cm.Path == path {
			if _, remove := indices[pathIdx]; remove {
				pathIdx++
				continue
			}
			pathIdx++
		}
		keptComments = append(keptComments, cm)
		keptSides = append(keptSides, sideAt(c.sides, i))
	}
	tail := c.comments[len(keptComments):]
	for i := range tail {
		tail[i] = model.LlmComment{}
	}
	clear(c.sides[len(keptSides):])
	c.comments = keptComments
	c.sides = keptSides
}

func sideAt(sides []location.Side, index int) location.Side {
	if index < 0 || index >= len(sides) {
		return location.SideUnknown
	}
	return sides[index]
}
