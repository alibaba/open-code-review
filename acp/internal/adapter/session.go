// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alibaba/open-code-review/acp/internal/intent"
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

// beginTurn marks the session busy and installs its cancellation context under
// the agent lock. Every successful call must be paired with endTurn.
func (a *Agent) beginTurn(ctx context.Context, id acp.SessionId) (*session, context.Context, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, nil, fmt.Errorf("server_closed")
	}
	s := a.sessions[id]
	if s == nil {
		return nil, nil, fmt.Errorf("unknown session")
	}
	if s.closing {
		return nil, nil, fmt.Errorf("session_closed")
	}
	if s.busy {
		return nil, nil, fmt.Errorf("session_busy")
	}
	s.busy = true
	s.done = make(chan struct{})
	task, cancel := context.WithCancel(ctx)
	if a.TurnTimeout > 0 {
		cancel()
		task, cancel = context.WithTimeout(ctx, a.TurnTimeout)
	}
	s.cancel = cancel
	return s, task, nil
}

// endTurn releases the session only after its prompt and runner cleanup finish.
// The busy flag keeps s.cancel owned by this turn until it is cleared here.
func (a *Agent) endTurn(ctx context.Context, id acp.SessionId, s *session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() != nil {
		s.state.Clear()
	}
	s.cancel()
	s.busy = false
	s.cancel = nil
	close(s.done)
	if s.closing {
		delete(a.sessions, id)
	}
}
