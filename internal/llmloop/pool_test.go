package llmloop

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// TestCommentWorkerPool_AwaitKeyWaitsForOwnKey verifies that AwaitKey blocks
// until the units submitted under its key complete, without waiting for units
// registered under other keys.
func TestCommentWorkerPool_AwaitKeyWaitsForOwnKey(t *testing.T) {
	p := NewCommentWorkerPool(2)

	release := make(chan struct{})
	ownDone := make(chan struct{})
	p.SubmitFor("own.go", func() ([]model.LlmComment, error) {
		close(ownDone)
		<-release
		return []model.LlmComment{{Path: "own.go", Content: "mine"}}, nil
	})
	// A slow unit under a different key must not be required for AwaitKey("own.go").
	p.SubmitFor("other.go", func() ([]model.LlmComment, error) {
		<-release
		return []model.LlmComment{{Path: "other.go", Content: "theirs"}}, nil
	})

	awaitReturned := make(chan struct{})
	go func() {
		p.AwaitKey("own.go")
		close(awaitReturned)
	}()

	<-ownDone
	close(release)
	select {
	case <-awaitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitKey did not return after its own key's work completed")
	}

	// The whole pool is drained by Await; both keyed units' results are present.
	results := p.Await()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

// TestCommentWorkerPool_AwaitKeyConcurrentSubmitOtherKey is the regression
// test for the review-path race: one file's goroutine draining its async
// comment work while other files' loops keep submitting. A pool-wide Await in
// this pattern misuses sync.WaitGroup ("Add called concurrently with Wait");
// the keyed API must stay panic-free.
func TestCommentWorkerPool_AwaitKeyConcurrentSubmitOtherKey(t *testing.T) {
	p := NewCommentWorkerPool(4)

	stop := make(chan struct{})
	var submits atomic.Int64
	var producerWg sync.WaitGroup
	for i := 0; i < 4; i++ {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					p.SubmitFor("producer.go", func() ([]model.LlmComment, error) {
						return nil, nil
					})
					submits.Add(1)
				}
			}
		}()
	}

	// Concurrently drain per-goroutine keys that occasionally have real work.
	// Each key is used by exactly one goroutine (submit-then-await in program
	// order), matching the per-file usage in the review path.
	var drained atomic.Int64
	var drainerWg sync.WaitGroup
	for i := 0; i < 4; i++ {
		key := fmt.Sprintf("drainer-%d.go", i)
		drainerWg.Add(1)
		go func() {
			defer drainerWg.Done()
			for j := 0; j < 200; j++ {
				p.SubmitFor(key, func() ([]model.LlmComment, error) {
					return []model.LlmComment{{Path: key}}, nil
				})
				p.AwaitKey(key)
				drained.Add(1)
			}
		}()
	}
	drainerWg.Wait()
	close(stop)
	producerWg.Wait()

	if drained.Load() != 800 {
		t.Errorf("drained = %d, want 800", drained.Load())
	}
	if submits.Load() == 0 {
		t.Error("producers never submitted")
	}
	p.Await()
}

// TestCommentWorkerPool_AwaitKeyUnknown verifies waiting on a key with no
// submissions returns immediately.
func TestCommentWorkerPool_AwaitKeyUnknown(t *testing.T) {
	p := NewCommentWorkerPool(2)
	done := make(chan struct{})
	go func() {
		p.AwaitKey("never-submitted.go")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitKey on unknown key blocked")
	}
}
