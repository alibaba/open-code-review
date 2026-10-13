// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"io"

	"github.com/alibaba/open-code-review/acp/internal/adapter/transport"
	acp "github.com/coder/acp-go-sdk"
)

// Connection preserves the adapter entry point while transport owns the wire protocol.
type Connection = transport.Connection

// NewConnection binds notifications before any request can acquire the agent lock.
func NewConnection(agent *Agent, output io.Writer, input io.Reader) *Connection {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	conn := transport.NewConnection(agent, output, input, agent.claimCommandDiscovery)
	agent.conn = conn
	return conn
}

// claimCommandDiscovery keeps session lifecycle and discovery state under one lock.
func (a *Agent) claimCommandDiscovery(id acp.SessionId) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessions[id]
	if s == nil || s.closing || a.closed || s.commandsSent {
		return false
	}
	s.commandsSent = true
	return true
}
