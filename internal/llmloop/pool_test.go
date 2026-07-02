package llmloop

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestNewCommentWorkerPool_Default(t *testing.T) {
	p := NewCommentWorkerPool(0)
	if cap(p.semaphore) != 8 {
		t.Errorf("default capacity = %d, want 8", cap(p.semaphore))
	}
}

func TestNewCommentWorkerPool_Custom(t *testing.T) {
	p := NewCommentWorkerPool(4)
	if cap(p.semaphore) != 4 {
		t.Errorf("capacity = %d, want 4", cap(p.semaphore))
	}
}

func TestCommentWorkerPool_SubmitAndAwait(t *testing.T) {
	p := NewCommentWorkerPool(2)

	p.Submit(func() ([]model.LlmComment, error) {
		return []model.LlmComment{{Path: "a.go", Content: "issue 1"}}, nil
	})
	p.Submit(func() ([]model.LlmComment, error) {
		return []model.LlmComment{{Path: "b.go", Content: "issue 2"}, {Path: "b.go", Content: "issue 3"}}, nil
	})

	results := p.Await()
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	paths := map[string]bool{}
	for _, r := range results {
		paths[r.Path] = true
	}
	if !paths["a.go"] || !paths["b.go"] {
		t.Errorf("unexpected paths: %v", results)
	}
}

func TestCommentWorkerPool_ErrorDoesNotBlock(t *testing.T) {
	p := NewCommentWorkerPool(2)

	p.Submit(func() ([]model.LlmComment, error) {
		return nil, errors.New("oops")
	})
	p.Submit(func() ([]model.LlmComment, error) {
		return []model.LlmComment{{Path: "ok.go", Content: "fine"}}, nil
	})

	results := p.Await()
	if len(results) != 1 {
		t.Fatalf("expected 1 result after error, got %d", len(results))
	}
	if results[0].Path != "ok.go" {
		t.Errorf("Path = %q", results[0].Path)
	}
}

func TestCommentWorkerPool_Concurrency(t *testing.T) {
	p := NewCommentWorkerPool(3)
	var running atomic.Int32
	var maxRunning atomic.Int32

	for i := 0; i < 10; i++ {
		p.Submit(func() ([]model.LlmComment, error) {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			running.Add(-1)
			return nil, nil
		})
	}

	p.Await()
	if maxRunning.Load() > 3 {
		t.Errorf("max concurrent = %d, expected <= 3", maxRunning.Load())
	}
}

func TestCommentWorkerPool_AwaitEmpty(t *testing.T) {
	p := NewCommentWorkerPool(2)
	results := p.Await()
	if results != nil {
		t.Errorf("expected nil for no submissions, got %v", results)
	}
}

func TestCommentWorkerPool_PanicIsIsolated(t *testing.T) {
	p := NewCommentWorkerPool(2)

	p.Submit(func() ([]model.LlmComment, error) {
		panic("boom in submitted work")
	})
	p.Submit(func() ([]model.LlmComment, error) {
		return []model.LlmComment{{Path: "healthy.go", Content: "fine"}}, nil
	})

	// Await must not crash: the recovered panic contributes no comments, and the
	// healthy task's result is still collected.
	results := p.Await()
	if len(results) != 1 {
		t.Fatalf("expected 1 result after a panicking task, got %d", len(results))
	}
	if results[0].Path != "healthy.go" {
		t.Errorf("Path = %q, want healthy.go", results[0].Path)
	}
}
