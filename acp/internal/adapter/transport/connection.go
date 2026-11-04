// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package transport implements ACP dispatch and ordered notifications over the SDK connection.
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type writeDeadliner interface {
	io.Writer
	SetWriteDeadline(time.Time) error
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
	if file, ok := w.output.(writeDeadliner); ok {
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

// NewConnection serves the SDK protocol without changing Agent cancellation ownership.
// claimDiscovery atomically claims command discovery for a live session.
func NewConnection(agent acp.Agent, output io.Writer, input io.Reader, claimDiscovery func(acp.SessionId) bool) *Connection {
	w := &timedWriter{output: output, input: input, timeout: 2 * time.Second}
	handler := func(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
		return dispatch(ctx, agent, method, params)
	}
	return &Connection{acp.NewConnection(handler, &discoveryWriter{output: w, claim: claimDiscovery}, input)}
}

func (c *Connection) SessionUpdate(ctx context.Context, p acp.SessionNotification) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return c.SendNotification(ctx, acp.ClientMethodSessionUpdate, p)
}
