// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

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
