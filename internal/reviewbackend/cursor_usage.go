package reviewbackend

import (
	"sync"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/remdev/cursor-go-sdk/cursor"
)

// cursorUsageAccumulator collects token usage from Cursor stream deltas.
type cursorUsageAccumulator struct {
	mu              sync.Mutex
	prompt          int64
	completion      int64
	cacheRead       int64
	cacheWrite      int64
	deltaCompletion int64
	hasTurnUsage    bool
	reportedTotal   int64
}

func (a *cursorUsageAccumulator) observe(update cursor.InteractionUpdate) {
	switch update.Type {
	case "turn-ended":
		if ui := llm.UsageFromMap(update.Usage); ui != nil {
			a.mu.Lock()
			if !a.hasTurnUsage {
				a.prompt = 0
				a.completion = 0
				a.cacheRead = 0
				a.cacheWrite = 0
				a.hasTurnUsage = true
			}
			a.prompt += ui.PromptTokens
			a.completion += ui.CompletionTokens
			a.cacheRead += ui.CacheReadTokens
			a.cacheWrite += ui.CacheWriteTokens
			if ui.TotalTokens > 0 {
				a.reportedTotal += ui.TotalTokens
			}
			a.mu.Unlock()
		}
	case "token-delta":
		if update.Tokens > 0 {
			a.mu.Lock()
			if !a.hasTurnUsage {
				a.deltaCompletion += int64(update.Tokens)
			}
			a.mu.Unlock()
		}
	}
}

func (a *cursorUsageAccumulator) usage() *llm.UsageInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	prompt := a.prompt
	completion := a.completion
	cacheRead := a.cacheRead
	cacheWrite := a.cacheWrite
	reportedTotal := a.reportedTotal

	if !a.hasTurnUsage && a.deltaCompletion > 0 {
		completion = a.deltaCompletion
	}

	componentTotal := prompt + completion + cacheRead + cacheWrite
	if componentTotal == 0 && reportedTotal == 0 {
		return nil
	}
	total := componentTotal
	if total == 0 {
		total = reportedTotal
	} else if reportedTotal > total {
		total = reportedTotal
	}
	return &llm.UsageInfo{
		TotalTokens:      total,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
	}
}

func (a *cursorUsageAccumulator) callback() func(cursor.InteractionUpdate) {
	return a.observe
}
