// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

type session struct {
	commandsSent bool
	closing      bool
	cwd          string
	state        *intent.State
	cancel       context.CancelFunc
	busy         bool
	done         chan struct{}
}
type Agent struct {
	// TurnTimeout bounds parsing and OCR execution together. Zero disables it.
	TurnTimeout time.Duration
	parser      *intent.Parser
	binary      string
	runner      orchestrator.Runner
	conn        interface {
		SessionUpdate(context.Context, acp.SessionNotification) error
	}
	mu       sync.Mutex
	sessions map[acp.SessionId]*session
	nextID   uint64
	closed   bool
}

func NewAgent(binary string, runner orchestrator.Runner, parsers ...*intent.Parser) *Agent {
	parser := intent.NewParser(nil, nil)
	if len(parsers) > 0 && parsers[0] != nil {
		parser = parsers[0]
	}
	return &Agent{binary: binary, parser: parser, runner: runner, sessions: map[acp.SessionId]*session{}}
}
func (a *Agent) SetAgentConnection(c interface {
	SessionUpdate(context.Context, acp.SessionNotification) error
}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.conn = c
}
func (a *Agent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{ProtocolVersion: acp.ProtocolVersionNumber, AgentInfo: &acp.Implementation{Name: "ocr-acp", Version: "dev"}, AgentCapabilities: acp.AgentCapabilities{PromptCapabilities: acp.PromptCapabilities{}, SessionCapabilities: acp.SessionCapabilities{Close: &acp.SessionCloseCapabilities{}}}}, nil
}
func (a *Agent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}
func (a *Agent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}
func (a *Agent) NewSession(_ context.Context, p acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if !filepath.IsAbs(p.Cwd) {
		return acp.NewSessionResponse{}, fmt.Errorf("cwd must be absolute")
	}
	st, e := os.Stat(p.Cwd)
	if e != nil || !st.IsDir() {
		return acp.NewSessionResponse{}, fmt.Errorf("invalid cwd")
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return acp.NewSessionResponse{}, fmt.Errorf("server_closed")
	}
	a.nextID++
	id := acp.SessionId(fmt.Sprintf("s-%d", a.nextID))
	a.sessions[id] = &session{cwd: p.Cwd, state: &intent.State{}}
	a.mu.Unlock()
	return acp.NewSessionResponse{SessionId: id}, nil
}
func (a *Agent) Cancel(_ context.Context, p acp.CancelNotification) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.sessions[p.SessionId]; s != nil {
		s.state.Clear()
		if s.cancel != nil {
			s.cancel()
		}
	}
	return nil
}
func (a *Agent) CloseSession(ctx context.Context, p acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	a.mu.Lock()
	s := a.sessions[p.SessionId]
	var done <-chan struct{}
	if s != nil {
		s.closing = true
		s.state.Clear()
		if s.cancel != nil {
			s.cancel()
		}
		done = s.done
	}
	a.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return acp.CloseSessionResponse{}, ctx.Err()
		}
	}
	a.mu.Lock()
	delete(a.sessions, p.SessionId)
	a.mu.Unlock()
	return acp.CloseSessionResponse{}, nil
}

// Shutdown rejects new work, cancels active turns and waits for reader cleanup.
func (a *Agent) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	a.closed = true
	var pending []<-chan struct{}
	for _, s := range a.sessions {
		s.state.Clear()
		if s.cancel != nil {
			s.cancel()
		}
		if s.done != nil {
			pending = append(pending, s.done)
		}
	}
	a.mu.Unlock()
	for _, done := range pending {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (a *Agent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return acp.PromptResponse{}, fmt.Errorf("server_closed")
	}
	s := a.sessions[p.SessionId]
	if s == nil {
		a.mu.Unlock()
		return acp.PromptResponse{}, fmt.Errorf("unknown session")
	}
	if s.closing {
		a.mu.Unlock()
		return acp.PromptResponse{}, fmt.Errorf("session_closed")
	}
	if s.busy {
		a.mu.Unlock()
		return acp.PromptResponse{}, fmt.Errorf("session_busy")
	}
	s.busy = true
	s.done = make(chan struct{})
	task, cancel := context.WithCancel(ctx)
	if a.TurnTimeout > 0 {
		cancel()
		task, cancel = context.WithTimeout(ctx, a.TurnTimeout)
	}
	s.cancel = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if task.Err() != nil {
			s.state.Clear()
		}
		cancel()
		s.busy = false
		s.cancel = nil
		close(s.done)
		if s.closing {
			delete(a.sessions, p.SessionId)
		}
		a.mu.Unlock()
	}()
	text, paths, e := promptContent(s.cwd, p.Prompt)
	if e != nil {
		s.state.Clear()
		return a.sendRejection(task, p.SessionId, e.Error())
	}
	if len(paths) > 0 && strings.TrimSpace(text) == "" {
		s.state.Clear()
		return a.sendTerminal(task, p.SessionId, "Send /scan with these resources to scan the selected files.", acp.StopReasonEndTurn)
	}
	r, e := a.parser.WithRepo(intent.GitRepo{Dir: s.cwd}).Parse(task, text, s.state)
	if task.Err() != nil {
		if errors.Is(task.Err(), context.DeadlineExceeded) {
			return a.reportTimeout(ctx, p.SessionId)
		}
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if e != nil {
		return a.sendRejection(task, p.SessionId, "Invalid scan or review options: "+e.Error())
	}
	if r.Kind == intent.KindClarify {
		return a.sendTerminal(task, p.SessionId, r.Clarify.Question, acp.StopReasonEndTurn)
	}
	if r.Kind == intent.KindReject {
		return a.sendRejection(task, p.SessionId, r.Reject.Reason+" "+r.Reject.Hint)
	}
	if len(paths) > 0 {
		if r.Scan == nil || len(r.Scan.Paths) > 0 {
			return a.sendRejection(task, p.SessionId, "Resource links select files only with /scan without --path. Remove the links for repository review or explicit --path scans.")
		}
		r.Scan.Paths = paths
	}
	var args []string
	if r.Review != nil {
		args, e = contract.BuildReviewArgs(r.Review)
	} else {
		args, e = contract.BuildScanArgs(r.Scan)
	}
	if e != nil {
		return a.sendRejection(task, p.SessionId, "Invalid scan or review options: "+e.Error())
	}
	root, e := resultRoot(task, s.cwd, args)
	if e != nil {
		if task.Err() != nil {
			return a.finishSend(task, p.SessionId, task.Err(), acp.StopReasonEndTurn)
		}
		return a.sendRejection(task, p.SessionId, "Cannot determine the OCR target directory. Review requires a Git repository; check the session directory or use /scan for a non-Git directory.")
	}
	// Output failures stop OCR without marking the whole turn as user-cancelled.
	runTask, stopRunner := context.WithCancel(task)
	defer stopRunner()
	o, progressErr := a.collectProgress(runTask, stopRunner, p.SessionId, orchestrator.Request{CWD: s.cwd, Args: args})
	if o.Kind == orchestrator.OutcomeTimedOut || errors.Is(task.Err(), context.DeadlineExceeded) {
		return a.reportTimeout(ctx, p.SessionId)
	}
	if progressErr != nil {
		return a.finishSend(task, p.SessionId, progressErr, acp.StopReasonEndTurn)
	}
	if o.Kind == orchestrator.OutcomeCancelled || task.Err() != nil {
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if o.Result != nil {
		text := formatResultAtRoot(o.Result, root)
		if o.Err != nil {
			text += "\nOCR failed: " + o.Err.Error()
		}
		resp, sendErr := a.sendTerminal(task, p.SessionId, text, acp.StopReasonEndTurn)
		if sendErr != nil {
			return resp, sendErr
		}
		if o.Err != nil {
			return acp.PromptResponse{}, o.Err
		}
		return resp, nil
	}
	if o.Err != nil {
		return acp.PromptResponse{}, o.Err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

// Application-level rejections are completed guidance, not model refusals.
// Clients such as Zed discard the turn's messages on the refusal stop reason.
func (a *Agent) sendRejection(ctx context.Context, id acp.SessionId, text string) (acp.PromptResponse, error) {
	response, err := a.sendTerminal(ctx, id, text, acp.StopReasonEndTurn)
	if err == nil && response.StopReason == acp.StopReasonEndTurn && response.Meta == nil && ctx.Err() == nil {
		response.Meta = map[string]any{"ocr": map[string]any{"kind": "rejected"}}
	}
	return response, err
}

func (a *Agent) sendTerminal(ctx context.Context, id acp.SessionId, text string, reason acp.StopReason) (acp.PromptResponse, error) {
	err := a.update(ctx, id, text)
	return a.finishSend(ctx, id, err, reason)
}

func (a *Agent) finishSend(ctx context.Context, id acp.SessionId, err error, reason acp.StopReason) (acp.PromptResponse, error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return a.reportTimeout(ctx, id)
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: reason}, nil
}

// ACP v1 has no timeout stop reason. Preserve the machine-readable cause in
// metadata instead of using max_turn_requests, which means a different limit.
func timeoutResponse() acp.PromptResponse {
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn, Meta: map[string]any{"ocr": map[string]any{"kind": "timed_out", "retryable": true}}}
}

func (a *Agent) reportTimeout(ctx context.Context, id acp.SessionId) (acp.PromptResponse, error) {
	// The task deadline has expired, but its connection may still be usable.
	notice, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	// A best-effort notice must not replace the machine-readable turn outcome.
	// A broken transport still closes normally and may prevent response delivery.
	_ = a.update(notice, id, "OCR timed out before this turn completed. Retry or increase --turn-timeout.")
	return timeoutResponse(), nil
}

func (a *Agent) update(ctx context.Context, id acp.SessionId, msg string) error {
	return a.notify(ctx, id, acp.UpdateAgentMessageText(msg))
}

func (a *Agent) notify(ctx context.Context, id acp.SessionId, update acp.SessionUpdate) error {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.SessionUpdate(ctx, acp.SessionNotification{SessionId: id, Update: update})
}
func (a *Agent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}
func (a *Agent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}
func (a *Agent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionResume)
}
func (a *Agent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionList)
}

var _ acp.Agent = (*Agent)(nil)
