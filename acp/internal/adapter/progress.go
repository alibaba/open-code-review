// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

const progressInterval = 500 * time.Millisecond

// collectProgress uses one native tool call for the entire OCR process. Log
// snapshots replace prior content at a bounded rate, not once per stderr line.
func (a *Agent) collectProgress(ctx context.Context, cancel context.CancelFunc, sessionID acp.SessionId, request orchestrator.Request) (orchestrator.Outcome, error) {
	id := nextToolCallID("ocr-execution")
	// Generic tools have a collapsible header in Zed. Execute titles instead
	// render as command previews, without the same activity-label interaction.
	operation := "OCR review"
	if len(request.Args) > 0 && request.Args[0] == "scan" {
		operation = "OCR scan"
	}
	started := time.Now()
	activity := ""
	title := progressTitle(operation, activity, 0)
	var details strings.Builder
	details.WriteString(executionCommand(a.binary, request))
	details.WriteString("Working directory:\n\n")
	writeCodeBlock(&details, "", request.CWD)
	executionDetails := acp.ToolContent(acp.TextBlock(details.String()))
	content := func(log *progressLog) []acp.ToolCallContent {
		blocks := []acp.ToolCallContent{executionDetails}
		if log.text != "" || log.truncated {
			blocks = append(blocks, log.content()...)
		}
		return blocks
	}
	if err := a.notify(ctx, sessionID, acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindOther), acp.WithStartStatus(acp.ToolCallStatusInProgress), acp.WithStartContent([]acp.ToolCallContent{executionDetails}))); err != nil {
		return orchestrator.Outcome{}, err
	}
	events, outcomes := a.runner.Run(ctx, request)
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	var log progressLog
	first := true
	var sendErr error
	flush := func() {
		if sendErr != nil || ctx.Err() != nil {
			return
		}
		nextTitle := progressTitle(operation, activity, time.Since(started))
		if !log.dirty && nextTitle == title {
			return
		}
		options := []acp.ToolCallUpdateOpt{acp.WithUpdateTitle(nextTitle)}
		if log.dirty {
			options = append(options, acp.WithUpdateContent(content(&log)))
		}
		sendErr = a.notify(ctx, sessionID, acp.UpdateToolCall(id, options...))
		title = nextTitle
		log.dirty = false
		if sendErr != nil {
			cancel()
		}
	}
	for events != nil {
		select {
		case event, open := <-events:
			if !open {
				events = nil
				continue
			}
			log.append(event.Message)
			log.truncated = log.truncated || event.Truncated
			if event.Kind == orchestrator.EventProgress && strings.TrimSpace(event.Message) != "" {
				activity = event.Message
			}
			if first && log.dirty {
				first = false
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
	flush()
	outcome := <-outcomes
	// The bounded stderr tail is authoritative when best-effort events dropped.
	if outcome.Diagnostics != "" {
		log.text = ""
		log.append(outcome.Diagnostics)
	}
	for _, warning := range outcome.Warnings {
		log.append(warning.Message)
		log.truncated = log.truncated || warning.Truncated
	}
	status := acp.ToolCallStatusCompleted
	kind := outcome.Kind
	if ctx.Err() == context.DeadlineExceeded {
		kind = orchestrator.OutcomeTimedOut
	} else if ctx.Err() != nil {
		kind = orchestrator.OutcomeCancelled
	}
	if outcome.Err != nil && kind == orchestrator.OutcomeCompleted {
		kind = orchestrator.OutcomeFailed
	}
	if sendErr != nil {
		kind = orchestrator.OutcomeFailed
		log.append("OCR progress delivery failed: " + sendErr.Error())
	}
	if kind != orchestrator.OutcomeCompleted || outcome.Err != nil {
		status = acp.ToolCallStatusFailed
	}
	if kind != orchestrator.OutcomeCompleted {
		log.append("OCR execution: " + string(kind))
	}
	if outcome.Err != nil {
		log.append(outcome.Err.Error())
	}
	// After cleanup, make one bounded terminal attempt even if progress failed.
	// A disconnected client may never receive it; preserve the original failure.
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	finalContent := content(&log)
	if stats := executionStats(outcome.Result); stats != "" {
		finalContent = append(finalContent, acp.ToolContent(acp.TextBlock(stats)))
	}
	finalTitle := progressTitle(operation, executionResultLabel(kind, outcome.Result), time.Since(started))
	if err := a.notify(finish, sessionID, acp.UpdateToolCall(id, acp.WithUpdateTitle(finalTitle), acp.WithUpdateStatus(status), acp.WithUpdateContent(finalContent))); sendErr == nil {
		sendErr = err
	}
	return outcome, sendErr
}
