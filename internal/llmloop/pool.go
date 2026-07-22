// Package llmloop carries the per-file LLM tool-use loop shared by `ocr
// review` (diff-based) and `ocr scan` (full-file). It owns the chat
// completion conversation state, three-zone memory compression, tool-call
// dispatch (including async comment post-processing), and aggregate token /
// warning bookkeeping. Callers above this package render the initial
// messages (review uses MAIN_TASK, scan uses FULL_SCAN_TASK) and hand them
// in via Runner.RunPerFile.
package llmloop

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/stdout"
)

// AgentWarning describes a non-fatal warning recorded during a per-file
// review/scan. The name is kept for backwards compatibility with the
// previous internal/agent package.
type AgentWarning struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// CommentWorkerPool manages a fixed-size pool of workers dedicated to
// processing code-review comment post-steps (line-range tracking,
// re-tracking, reflection, suggestion validation) asynchronously.
//
// Offloading them to a worker pool keeps the main LLM tool-use loop
// unblocked, reducing overall latency — mirroring the Java side's dedicated
// subtaskExecutor for the CODE_COMMENT tool.
type CommentWorkerPool struct {
	semaphore chan struct{}
	wg        sync.WaitGroup
	resultsMu sync.Mutex
	results   []model.LlmComment

	// keys tracks per-key WaitGroups so callers can drain only the units
	// submitted under one key (e.g. one reviewed file) without waiting for
	// — or racing — submissions made under other keys.
	keysMu sync.Mutex
	keys   map[string]*sync.WaitGroup
}

// NewCommentWorkerPool creates a pool with the given concurrency limit.
// workerCount <= 0 defaults to 8.
func NewCommentWorkerPool(workerCount int) *CommentWorkerPool {
	if workerCount <= 0 {
		workerCount = 8
	}
	return &CommentWorkerPool{
		semaphore: make(chan struct{}, workerCount),
	}
}

// Submit runs f in a background goroutine bounded by the semaphore.
// When f completes its return value is collected internally.
func (p *CommentWorkerPool) Submit(f func() ([]model.LlmComment, error)) {
	p.submit(f, nil)
}

// SubmitFor is Submit plus registration under key, so AwaitKey can wait for
// exactly the units submitted under that key instead of the whole pool.
//
// Callers must guarantee that all SubmitFor calls for a given key
// happen-before the matching AwaitKey call for that key — the same contract
// as Await, but scoped to one key. Per-file review satisfies this because a
// file's tool-use loop finishes submitting before its AwaitKey runs.
func (p *CommentWorkerPool) SubmitFor(key string, f func() ([]model.LlmComment, error)) {
	p.keysMu.Lock()
	if p.keys == nil {
		p.keys = make(map[string]*sync.WaitGroup)
	}
	kwg := p.keys[key]
	if kwg == nil {
		kwg = &sync.WaitGroup{}
		p.keys[key] = kwg
	}
	kwg.Add(1)
	p.keysMu.Unlock()
	p.submit(f, kwg)
}

func (p *CommentWorkerPool) submit(f func() ([]model.LlmComment, error), kwg *sync.WaitGroup) {
	p.wg.Go(func() {
		if kwg != nil {
			defer kwg.Done()
		}
		p.semaphore <- struct{}{}
		defer func() { <-p.semaphore }()
		// Contain a panic in the submitted work so one bad unit of work cannot
		// crash the whole process. The work that panics contributes no comments;
		// the semaphore is still released via the defer above.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(stdout.Writer(), "[ocr] CommentWorkerPool panic: %v\n%s\n", r, debug.Stack())
			}
		}()

		comments, err := f()
		if err != nil {
			fmt.Fprintf(stdout.Writer(), "[ocr] CommentWorkerPool error: %v\n", err)
		}
		p.resultsMu.Lock()
		p.results = append(p.results, comments...)
		p.resultsMu.Unlock()
	})
}

// Await blocks until all submitted work has completed and returns
// aggregated results from every Submit call so far.
//
// A panic in submitted work is recovered and logged inside Submit (see the
// recover defer there) but is not surfaced here as an error or reflected in
// the returned count — a unit that panics contributes no comments and is
// indistinguishable from one that produced zero.
//
// Concurrency contract: Await must not run concurrently with Submit. Submit
// calls wg.Go (which does wg.Add(1) synchronously), so a Submit racing Await
// would risk sync.WaitGroup's "Add called concurrently with Wait" panic.
// Callers must ensure every Submit has returned before calling Await.
// Callers that need to drain while other submissions are still in flight
// must use SubmitFor/AwaitKey instead.
func (p *CommentWorkerPool) Await() []model.LlmComment {
	p.wg.Wait()
	return p.results
}

// AwaitKey blocks until every unit submitted under key so far has completed.
// It never touches the pool-wide WaitGroup, so it is safe to call while other
// keys still have SubmitFor calls in flight — unlike Await.
//
// The caller must ensure no SubmitFor with the same key runs concurrently
// with AwaitKey (see SubmitFor's contract). Waiting on an unknown key
// returns immediately.
func (p *CommentWorkerPool) AwaitKey(key string) {
	p.keysMu.Lock()
	kwg := p.keys[key]
	p.keysMu.Unlock()
	if kwg == nil {
		return
	}
	kwg.Wait()
}
