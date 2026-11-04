// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package transport

import (
	"context"
	"encoding/json"
	"errors"

	acp "github.com/coder/acp-go-sdk"
)

func dispatch(ctx context.Context, a acp.Agent, method string, params json.RawMessage) (any, *acp.RequestError) {
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
