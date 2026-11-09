// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alibaba/open-code-review/internal/stdout"
)

type requestProgress struct {
	identity     string
	started      time.Time
	currentStage string
	stageStarted time.Time
}

func startRequestProgress(ctx context.Context, model, defaultModel string) *requestProgress {
	if model == "" {
		model = defaultModel
	}
	identity := fmt.Sprintf("model=%q", model)
	if meta, ok := RequestMetaFromContext(ctx); ok {
		identity += fmt.Sprintf(" provider=%q file=%q task=%q request_no=%d",
			meta.Provider, meta.FilePath, meta.TaskType, meta.RequestNo)
	}
	p := &requestProgress{identity: identity, started: time.Now()}
	fmt.Fprintf(stdout.Writer(), "[ocr] LLM request started %s\n", p.identity)
	return p
}

func (p *requestProgress) stage(name string) {
	p.finishStage("ok")
	p.currentStage, p.stageStarted = name, time.Now()
	fmt.Fprintf(stdout.Writer(), "[ocr] LLM stage started %s stage=%q\n", p.identity, name)
}

func (p *requestProgress) finishStage(status string) {
	if p.currentStage == "" {
		return
	}
	fmt.Fprintf(stdout.Writer(), "[ocr] LLM stage finished %s stage=%q status=%s elapsed=%s\n",
		p.identity, p.currentStage, status, time.Since(p.stageStarted).Round(time.Millisecond))
	p.currentStage = ""
}

func (p *requestProgress) finish(err error) {
	status := "ok"
	switch {
	case errors.Is(err, errRequestPanicked):
		status = "panicked"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrRequestTimeout):
		status = "timeout"
	case errors.Is(err, context.Canceled):
		status = "cancelled"
	case err != nil:
		status = "failed"
	}
	p.finishStage(status)
	fmt.Fprintf(stdout.Writer(), "[ocr] LLM request finished %s status=%s elapsed=%s\n",
		p.identity, status, time.Since(p.started).Round(time.Millisecond))
}
