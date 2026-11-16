// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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
	raw          *RawHolder
	base         http.RoundTripper
}

func (t *copilotTransport) begin(ctx context.Context, limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ctx, t.limit, t.responded = ctx, limit, false
	t.responseDone = nil
}

func copilotOutputLimitKey(path string) string {
	switch {
	case strings.HasSuffix(path, "/responses"):
		return "max_output_tokens"
	case strings.HasSuffix(path, "/messages"):
		return "max_tokens"
	case strings.HasSuffix(path, "/chat/completions"):
		return "max_completion_tokens"
	default:
		return ""
	}
}

func (t *copilotTransport) prepare(data []byte, path string) ([]byte, context.Context, bool, error) {
	key := copilotOutputLimitKey(path)
	if key == "" {
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
	limit := t.limit
	var upstreamMax int
	_ = json.Unmarshal(body[key], &upstreamMax)
	for _, field := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		var existing int
		if json.Unmarshal(body[field], &existing) == nil && existing > 0 && existing < limit {
			limit = existing
		}
		delete(body, field)
	}
	body[key], _ = json.Marshal(limit)
	if key == "max_tokens" && (upstreamMax == 0 || limit < upstreamMax) {
		capCopilotThinking(body, limit)
	}
	encoded, err := json.Marshal(body)
	return encoded, t.ctx, true, err
}

func capCopilotThinking(body map[string]json.RawMessage, limit int) {
	var thinking map[string]json.RawMessage
	if json.Unmarshal(body["thinking"], &thinking) != nil {
		return
	}
	var mode string
	var budget int
	if json.Unmarshal(thinking["type"], &mode) != nil || mode != "enabled" ||
		json.Unmarshal(thinking["budget_tokens"], &budget) != nil || budget < limit {
		return
	}
	// Manual thinking requires at least 1024 tokens plus room for the answer.
	if limit <= 1024 {
		delete(body, "thinking")
		return
	}
	thinking["budget_tokens"], _ = json.Marshal(limit - 1)
	body["thinking"], _ = json.Marshal(thinking)
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
	var response *http.Response
	if inference && t.raw != nil {
		response, err = t.raw.captureHTTP(round, request, base.RoundTrip)
	} else {
		response, err = base.RoundTrip(request)
	}
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
	rawMu     sync.Mutex
	rawRound  context.Context // Last inference sent on this socket, even if another OCR round has begun.
}

func (t *copilotTransport) openWebSocket(ctx *copilot.CopilotRequestContext) (copilot.CopilotWebSocketHandler, error) {
	endpoint, err := url.Parse(ctx.URL)
	if err != nil {
		return nil, err
	}
	forwarder := copilot.NewCopilotWebSocketForwarder(ctx.URL, ctx.Headers)
	socket := &copilotWebSocket{CopilotWebSocketForwarder: forwarder, transport: t, path: endpoint.Path}
	if t.raw != nil && copilotOutputLimitKey(endpoint.Path) != "" {
		socket.rawRound = context.Background()
		forwarder.OnSendRequestMessage = socket.rawFrameObserver(ctx.RequestID, "request")
		forwarder.OnSendResponseMessage = socket.rawFrameObserver(ctx.RequestID, "response")
	}
	return socket, nil
}

// Observe frames at the forwarding boundary after any output-limit rewrite.
// Emit each frame immediately, including on long-lived or interrupted streams.
func (w *copilotWebSocket) rawFrameObserver(id, direction string) func(copilot.CopilotWebSocketMessage) *copilot.CopilotWebSocketMessage {
	return func(msg copilot.CopilotWebSocketMessage) *copilot.CopilotWebSocketMessage {
		writer := w.transport.raw.get()
		if writer == nil {
			return &msg
		}
		w.rawMu.Lock()
		round := w.rawRound
		w.rawMu.Unlock()
		var body json.RawMessage
		var text string
		switch {
		case msg.Binary:
			text = base64.StdEncoding.EncodeToString(msg.Data)
		case json.Valid(msg.Data):
			body = msg.Data
		default:
			text = string(msg.Data)
		}
		rec := newRawRecord(round, id, time.Now(), nil, body)
		rec.WebSocketDirection = direction
		rec.WebSocketBinary = msg.Binary
		if direction == "request" {
			rec.RequestBody, rec.RequestBodyText = body, text
		} else {
			rec.ResponseBody, rec.ResponseBodyText = body, text
		}
		writer.Write(rec)
		return &msg
	}
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
	if inference && w.transport.raw != nil {
		w.rawMu.Lock()
		w.rawRound = round
		w.rawMu.Unlock()
	}
	if err := w.CopilotWebSocketForwarder.SendRequestMessage(ctx, message); err != nil {
		return err
	}
	if inference {
		w.transport.markResponse()
	}
	return nil
}
