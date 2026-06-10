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
	p.wg.Go(func() {
		p.semaphore <- struct{}{}
		defer func() { <-p.semaphore }()

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
func (p *CommentWorkerPool) Await() []model.LlmComment {
	p.wg.Wait()
	return p.results
}
