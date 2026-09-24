// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"

	acp "github.com/coder/acp-go-sdk"
)

// Clients may retain tool cards across reconnects and server restarts. A random
// process namespace keeps new cards separate from those persisted histories.
var toolCallInstance = func() string {
	var instance [16]byte
	if _, err := rand.Read(instance[:]); err != nil {
		panic("generate ACP tool call namespace: " + err.Error())
	}
	return hex.EncodeToString(instance[:])
}()

var toolCallSequence atomic.Uint64

func nextToolCallID(kind string) acp.ToolCallId {
	return acp.ToolCallId(fmt.Sprintf("%s-%s-%d", kind, toolCallInstance, toolCallSequence.Add(1)))
}
