// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	copilot "github.com/github/copilot-sdk/go"
)

// The CLI can retry a truncated response internally. OCR owns the next round,
// so a successful inference must not trigger another unbudgeted model call.
type copilotTransport struct {
	mu           sync.Mutex
	ctx          context.Context
	limit        int
	responded    bool
	responseDone chan struct{}
	base         http.RoundTripper
}

func (t *copilotTransport) begin(ctx context.Context, limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ctx, t.limit, t.responded = ctx, limit, false
	t.responseDone = nil
}

func (t *copilotTransport) prepare(data []byte, path string) ([]byte, context.Context, bool, error) {
	key := ""
	switch {
	case strings.HasSuffix(path, "/responses"):
		key = "max_output_tokens"
	case strings.HasSuffix(path, "/messages"):
		key = "max_tokens"
	case strings.HasSuffix(path, "/chat/completions"):
		key = "max_completion_tokens"
	default:
		return data, nil, false, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ctx == nil {
		return nil, nil, false, errors.New("Copilot inference has no active OCR request")
	}
	if err := t.ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	if t.responded {
		return nil, nil, false, errors.New("Copilot attempted another model response within one OCR request")
	}
	if t.limit <= 0 {
		return data, t.ctx, true, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, nil, false, fmt.Errorf("decode Copilot inference request: %w", err)
	}
	if body == nil {
		return nil, nil, false, errors.New("Copilot inference request is not an object")
	}
	// Preserve a smaller upstream cap, including any reasoning budget it sets.
	limit := t.limit
	for _, field := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		var existing int
		if json.Unmarshal(body[field], &existing) == nil && existing > 0 && existing < limit {
			limit = existing
		}
		delete(body, field)
	}
	body[key], _ = json.Marshal(limit)
	encoded, err := json.Marshal(body)
	return encoded, t.ctx, true, err
}

func (t *copilotTransport) markResponse() {
	t.mu.Lock()
	t.responded = true
	t.mu.Unlock()
}

func (t *copilotTransport) waitResponse(ctx context.Context) error {
	t.mu.Lock()
	done := t.responseDone
	t.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *copilotTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if request.Method != http.MethodPost || request.Body == nil {
		return base.RoundTrip(request)
	}
	data, err := io.ReadAll(request.Body)
	_ = request.Body.Close()
	if err != nil {
		return nil, err
	}
	data, round, inference, err := t.prepare(data, request.URL.Path)
	if err != nil {
		return nil, err
	}
	request = request.Clone(request.Context())
	request.Body = io.NopCloser(bytes.NewReader(data))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
	request.ContentLength = int64(len(data))
	var cancel context.CancelFunc
	var stop func() bool
	if round != nil {
		ctx, done := context.WithCancel(request.Context())
		cancel = done
		stop = context.AfterFunc(round, done)
		request = request.WithContext(ctx)
	}
	response, err := base.RoundTrip(request)
	if err != nil {
		if cancel != nil {
			stop()
			cancel()
		}
		return nil, err
	}
	var completed func()
	if inference && response.StatusCode >= 200 && response.StatusCode < 300 {
		t.markResponse()
		done := make(chan struct{})
		t.mu.Lock()
		t.responseDone = done
		t.mu.Unlock()
		completed = sync.OnceFunc(func() { close(done) })
	}
	if cancel != nil {
		response.Body = &copilotResponseBody{ReadCloser: response.Body, cancel: cancel, stop: stop, completed: completed}
	}
	return response, nil
}

type copilotResponseBody struct {
	io.ReadCloser
	cancel    context.CancelFunc
	stop      func() bool
	completed func()
}

func (b *copilotResponseBody) Close() error {
	b.stop()
	b.cancel()
	err := b.ReadCloser.Close()
	if b.completed != nil {
		b.completed()
	}
	return err
}

type copilotWebSocket struct {
	*copilot.CopilotWebSocketForwarder
	transport *copilotTransport
	path      string
}

func (w *copilotWebSocket) SendRequestMessage(ctx context.Context, message copilot.CopilotWebSocketMessage) error {
	var frame struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(message.Data, &frame) == nil && frame.Type != "" && frame.Type != "response.create" {
		return w.CopilotWebSocketForwarder.SendRequestMessage(ctx, message)
	}
	data, round, inference, err := w.transport.prepare(message.Data, w.path)
	if err != nil {
		return err
	}
	if round != nil {
		if err := round.Err(); err != nil {
			return err
		}
	}
	message.Data = data
	if err := w.CopilotWebSocketForwarder.SendRequestMessage(ctx, message); err != nil {
		return err
	}
	if inference {
		w.transport.markResponse()
	}
	return nil
}
