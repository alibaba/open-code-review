// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// Connection uses the SDK framing and schema validation while leaving prompt
// cancellation to Agent. AgentSideConnection v0.13.5 cancels an existing prompt
// before the agent can reject a new one as busy.
type Connection struct{ *acp.Connection }

// timedWriter closes a stalled transport, including the read side, so the SDK
// releases its write lock and reports connection loss to the shutdown owner.
type timedWriter struct {
	output  io.Writer
	input   io.Reader
	timeout time.Duration
	once    sync.Once
}

func (w *timedWriter) abort() {
	w.once.Do(func() {
		if c, ok := w.output.(io.Closer); ok {
			_ = c.Close()
		}
		if c, ok := w.input.(io.Closer); ok {
			_ = c.Close()
		}
	})
}

func (w *timedWriter) Write(p []byte) (int, error) {
	if file, ok := w.output.(*os.File); ok {
		// Reject non-pollable files before entering a potentially blocking write.
		if err := file.SetWriteDeadline(time.Now().Add(w.timeout)); err != nil {
			w.abort()
			return 0, err
		}
		n, err := file.Write(p)
		if err != nil {
			w.abort()
		}
		return n, err
	}
	if _, ok := w.output.(io.Closer); !ok {
		return 0, fmt.Errorf("ACP output must support Close to interrupt blocked writes")
	}
	timer := time.AfterFunc(w.timeout, w.abort)
	defer timer.Stop()
	return w.output.Write(p)
}

func NewConnection(agent *Agent, output io.Writer, input io.Reader) *Connection {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	w := &timedWriter{output: output, input: input, timeout: 2 * time.Second}
	c := &Connection{acp.NewConnection(agent.dispatch, &discoveryWriter{output: w, agent: agent}, input)}
	agent.conn = c
	return c
}

func (c *Connection) SessionUpdate(ctx context.Context, p acp.SessionNotification) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return c.SendNotification(ctx, acp.ClientMethodSessionUpdate, p)
}

func (a *Agent) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
	switch method {
	case acp.AgentMethodInitialize:
		return dispatchRequest(ctx, params, a.Initialize)
	case acp.AgentMethodAuthenticate:
		return dispatchRequest(ctx, params, a.Authenticate)
	case acp.AgentMethodLogout:
		return dispatchRequest(ctx, params, a.Logout)
	case acp.AgentMethodSessionNew:
		return dispatchRequest(ctx, params, a.NewSession)
	case acp.AgentMethodSessionPrompt:
		return dispatchRequest(ctx, params, a.Prompt)
	case acp.AgentMethodSessionCancel:
		return dispatchRequest(ctx, params, func(ctx context.Context, p acp.CancelNotification) (any, error) {
			return nil, a.Cancel(ctx, p)
		})
	case acp.AgentMethodSessionClose:
		return dispatchRequest(ctx, params, a.CloseSession)
	case acp.AgentMethodSessionSetMode:
		return dispatchRequest(ctx, params, a.SetSessionMode)
	case acp.AgentMethodSessionSetConfigOption:
		return dispatchRequest(ctx, params, a.SetSessionConfigOption)
	case acp.AgentMethodSessionResume:
		return dispatchRequest(ctx, params, a.ResumeSession)
	case acp.AgentMethodSessionList:
		return dispatchRequest(ctx, params, a.ListSessions)
	default:
		return nil, acp.NewMethodNotFound(method)
	}
}

func dispatchRequest[P any, R any, V interface {
	*P
	Validate() error
}](ctx context.Context, raw json.RawMessage, call func(context.Context, P) (R, error)) (any, *acp.RequestError) {
	var p P
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
	}
	if err := V(&p).Validate(); err != nil {
		return nil, acp.NewInvalidParams(map[string]any{"error": err.Error()})
	}
	r, err := call(ctx, p)
	if err == nil {
		return r, nil
	}
	var rpcErr *acp.RequestError
	if errors.As(err, &rpcErr) {
		return nil, rpcErr
	}
	if errors.Is(err, context.Canceled) {
		return nil, acp.NewRequestCancelled(nil)
	}
	return nil, acp.NewInternalError(map[string]any{"error": err.Error()})
}
