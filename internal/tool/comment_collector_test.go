package tool

import (
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func cm(path, content string) model.LlmComment {
	return model.LlmComment{Path: path, Content: content}
}

func TestCommentCollector_AddAndComments(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "issue 1"))
	c.Add(cm("b.go", "issue 2"))

	got := c.Comments()
	if len(got) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(got))
	}
	if got[0].Path != "a.go" || got[1].Path != "b.go" {
		t.Errorf("unexpected paths: %v", got)
	}
}

func TestCommentCollector_CommentsReturnsDefensiveCopy(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "x"))
	got := c.Comments()
	got[0].Content = "mutated"
	if c.Comments()[0].Content == "mutated" {
		t.Error("Comments() should return a copy")
	}
}

func TestCommentCollector_CommentsForPath(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "1"))
	c.Add(cm("b.go", "2"))
	c.Add(cm("a.go", "3"))

	got := c.CommentsForPath("a.go")
	if len(got) != 2 {
		t.Fatalf("expected 2 comments for a.go, got %d", len(got))
	}
	if got[0].Content != "1" || got[1].Content != "3" {
		t.Errorf("unexpected: %v", got)
	}

	got = c.CommentsForPath("nonexist.go")
	if len(got) != 0 {
		t.Errorf("expected 0 for nonexistent path, got %d", len(got))
	}
}

func TestCommentCollector_SnapshotAndSince(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "old"))
	snap := c.Snapshot()
	if snap != 1 {
		t.Fatalf("snapshot = %d, want 1", snap)
	}

	c.Add(cm("b.go", "new1"))
	c.Add(cm("c.go", "new2"))

	since := c.Since(snap)
	if len(since) != 2 {
		t.Fatalf("Since(%d) len = %d, want 2", snap, len(since))
	}
	if since[0].Path != "b.go" || since[1].Path != "c.go" {
		t.Errorf("unexpected: %v", since)
	}
}

func TestCommentCollector_SinceEdgeCases(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "x"))

	if got := c.Since(-1); len(got) != 1 {
		t.Errorf("Since(-1) should clamp to 0, got len %d", len(got))
	}
	if got := c.Since(100); got != nil {
		t.Errorf("Since(100) should return nil, got %v", got)
	}
}

func TestCommentCollector_ReplaceSince(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "keep"))
	snap := c.Snapshot()
	c.Add(cm("b.go", "raw1"))
	c.Add(cm("c.go", "raw2"))

	c.ReplaceSince(snap, []model.LlmComment{cm("merged.go", "deduped")})

	got := c.Comments()
	if len(got) != 2 {
		t.Fatalf("expected 2 comments after replace, got %d", len(got))
	}
	if got[0].Path != "a.go" {
		t.Errorf("first comment should be kept, got %v", got[0])
	}
	if got[1].Path != "merged.go" {
		t.Errorf("second comment should be replacement, got %v", got[1])
	}
}

func TestCommentCollector_ReplaceSinceOutOfBounds(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "x"))
	c.ReplaceSince(100, []model.LlmComment{cm("z.go", "nope")})
	if len(c.Comments()) != 1 {
		t.Error("ReplaceSince beyond len should be no-op")
	}
}

func TestCommentCollector_RemoveByPathAndIndices(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "a0"))
	c.Add(cm("b.go", "b0"))
	c.Add(cm("a.go", "a1"))
	c.Add(cm("a.go", "a2"))
	c.Add(cm("b.go", "b1"))

	c.RemoveByPathAndIndices("a.go", map[int]struct{}{0: {}, 2: {}})

	got := c.Comments()
	if len(got) != 3 {
		t.Fatalf("expected 3 comments, got %d: %v", len(got), got)
	}
	paths := make([]string, len(got))
	for i, g := range got {
		paths[i] = g.Path + ":" + g.Content
	}
	want := []string{"b.go:b0", "a.go:a1", "b.go:b1"}
	for i, w := range want {
		if paths[i] != w {
			t.Errorf("index %d: got %q, want %q", i, paths[i], w)
		}
	}
}

func TestCommentCollector_RemoveByPathAndIndices_NoMatch(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "x"))
	c.RemoveByPathAndIndices("b.go", map[int]struct{}{0: {}})
	if len(c.Comments()) != 1 {
		t.Error("remove from non-matching path should be no-op")
	}
}

func TestCommentCollector_ReplaceSince_NegativeSnap(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "keep"))
	c.ReplaceSince(-1, []model.LlmComment{cm("new.go", "replaced")})

	got := c.Comments()
	if len(got) != 1 || got[0].Path != "new.go" {
		t.Errorf("ReplaceSince(-1) should replace all, got %v", got)
	}
}

func TestCommentCollector_ReplaceSince_Zero(t *testing.T) {
	c := NewCommentCollector()
	c.Add(cm("a.go", "x"))
	c.Add(cm("b.go", "y"))

	c.ReplaceSince(0, []model.LlmComment{cm("only.go", "z")})

	got := c.Comments()
	if len(got) != 1 || got[0].Path != "only.go" {
		t.Errorf("ReplaceSince(0) should replace all, got %v", got)
	}
}
