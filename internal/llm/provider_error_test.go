// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	openai "github.com/openai/openai-go/v3"
)

// These tests cover issue #1042. The SDK extracts an error payload with the
// gjson path "error", which does not match when a provider array-wraps its
// error document, so the body is dropped and only the status line survives.

func newErrorFixtureClient(t *testing.T, status int, body string) *OpenAIClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write error body: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return NewOpenAIClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "google/gemini-3.8-flash",
	})
}

func completionError(t *testing.T, client *OpenAIClient) error {
	t.Helper()

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err == nil {
		t.Fatal("expected a provider error, got nil")
	}
	return err
}

func TestOpenAIClient_SurfacesArrayWrappedErrorBody(t *testing.T) {
	const body = `[{"error":{"code":400,"message":"Function call is missing a thought_signature in functionCall parts.","status":"INVALID_ARGUMENT"}}]`
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))
	if !strings.Contains(err.Error(), "missing a thought_signature") {
		t.Fatalf("array-wrapped provider error body not surfaced: %v", err)
	}
}

// TestOpenAIClient_DoesNotDuplicateStandardErrorBody guards the enrichment's
// trigger: when the SDK already extracted a payload, nothing is appended.
func TestOpenAIClient_DoesNotDuplicateStandardErrorBody(t *testing.T) {
	const body = `{"error":{"code":400,"message":"invalid model","type":"invalid_request_error"}}`
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))
	if got := strings.Count(err.Error(), "invalid model"); got != 1 {
		t.Fatalf("provider message appears %d times, want 1: %v", got, err)
	}
}

func TestOpenAIClient_BoundsOversizedErrorBody(t *testing.T) {
	body := `[{"error":{"message":"` + strings.Repeat("x", maxErrorBodyBytes) + `"}}]`
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))
	if !strings.Contains(err.Error(), "... (truncated)") {
		t.Fatalf("oversized provider body was not truncated: %v", err)
	}
	if len(err.Error()) > maxErrorBodyBytes+1024 {
		t.Fatalf("error message = %d bytes, want it bounded near %d", len(err.Error()), maxErrorBodyBytes)
	}
}

// TestOpenAIClient_SurfacesArrayWrappedErrorBodyWhenStreaming covers the
// streaming exit, which returns stream.Err() rather than the request error.
func TestOpenAIClient_SurfacesArrayWrappedErrorBodyWhenStreaming(t *testing.T) {
	const body = `[{"error":{"code":400,"message":"Function call is missing a thought_signature in functionCall parts.","status":"INVALID_ARGUMENT"}}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write error body: %v", err)
		}
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "google/gemini-3.8-flash",
		ExtraBody: map[string]any{"stream": true},
	})
	if !strings.Contains(completionError(t, client).Error(), "missing a thought_signature") {
		t.Fatal("streamed provider error body not surfaced")
	}
}

// TestWithProviderErrorBody_LeavesResponseBodyReadable guards that enrichment
// does not starve later consumers of the response body.
func TestWithProviderErrorBody_LeavesResponseBodyReadable(t *testing.T) {
	const body = `[{"error":{"message":"boom"}}]`
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an *openai.Error, got %T", err)
	}
	rest, readErr := io.ReadAll(apiErr.Response.Body)
	if readErr != nil {
		t.Fatalf("re-read response body: %v", readErr)
	}
	if string(rest) != body {
		t.Fatalf("response body = %q, want it restored to %q", rest, body)
	}
}

// TestOpenAIClient_StripsTerminalControlsFromErrorBody guards against an
// endpoint using the surfaced body to rewrite the terminal.
func TestOpenAIClient_StripsTerminalControlsFromErrorBody(t *testing.T) {
	body := "[{\"error\":{\"message\":\"boom\x1b[31m red \x1b]0;pwned\x07" + string(rune(0x9b)) + "31m" + string(rune(0x90)) + "dcs\"}}]"
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))
	// 0x9b is CSI and 0x90 is DCS in the C1 range: a terminal decoding C1 obeys
	// them exactly as it obeys the ESC-prefixed forms.
	for _, r := range []rune{0x1b, 0x07, 0x9b, 0x90} {
		if strings.ContainsRune(err.Error(), r) {
			t.Fatalf("control rune %U survived into the error message: %q", r, err.Error())
		}
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("sanitizing dropped the diagnostic text: %q", err.Error())
	}
}

// shortReader yields some bytes and then fails, standing in for a body that
// cannot be read to completion.
type shortReader struct {
	data []byte
	done bool
}

func (r *shortReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("connection reset")
	}
	r.done = true
	n := copy(p, r.data)
	return n, nil
}

func (r *shortReader) Close() error { return nil }

// TestWithProviderErrorBody_ShortReadKeepsWhatArrived documents the short-read
// path. The SDK buffers every non-2xx body before building the error, so this
// is unreachable in production; if it ever were reachable, the bytes that did
// arrive are what a reader and DumpResponse can still use.
func TestWithProviderErrorBody_ShortReadKeepsWhatArrived(t *testing.T) {
	apiErr := &openai.Error{
		StatusCode: http.StatusBadRequest,
		Response:   &http.Response{StatusCode: http.StatusBadRequest, Body: &shortReader{data: []byte(`[{"error":`)}},
	}

	got := withProviderErrorBody(apiErr)
	if got != error(apiErr) {
		t.Fatalf("a short read should return the original error unwrapped, got %v", got)
	}

	rest, readErr := io.ReadAll(apiErr.Response.Body)
	if readErr != nil {
		t.Fatalf("re-read restored body: %v", readErr)
	}
	if !bytes.Equal(rest, []byte(`[{"error":`)) {
		t.Errorf("partial bytes = %q, want them preserved", rest)
	}
}

// TestOpenAIClient_SurfacesBodyWhenSDKExtractedNull covers a provider whose
// document has an "error" key holding null: gjson reports the literal "null",
// which is not a payload a user can act on, so the body is still needed.
func TestOpenAIClient_SurfacesBodyWhenSDKExtractedNull(t *testing.T) {
	const body = `{"error":null,"message":"quota exhausted for this project","code":400}`
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))
	if !strings.Contains(err.Error(), "quota exhausted for this project") {
		t.Fatalf("body dropped when the SDK extracted null: %v", err)
	}
}

// TestWithProviderErrorBody_IsIdempotent guards against the payload being
// appended twice. Enriching restores the body it read, so a second pass would
// otherwise find it readable and append again — silently doubling the output if
// the helper ever ends up on two layers of the same call path.
func TestWithProviderErrorBody_IsIdempotent(t *testing.T) {
	const body = `[{"error":{"code":400,"message":"only once please"}}]`
	err := completionError(t, newErrorFixtureClient(t, http.StatusBadRequest, body))

	once := strings.Count(err.Error(), "only once please")
	if once != 1 {
		t.Fatalf("first enrichment produced %d copies, want 1", once)
	}
	for i := 0; i < 3; i++ {
		err = withProviderErrorBody(err)
		if got := strings.Count(err.Error(), "only once please"); got != 1 {
			t.Fatalf("after %d further enrichments the payload appears %d times, want 1", i+1, got)
		}
	}
}

// TestOpenAIResponsesClient_SurfacesArrayWrappedErrorBody covers the Responses
// API exit. It routes its request error through withProviderErrorBody too, but
// no other test drives that client, so this guards the wiring against removal.
func TestOpenAIResponsesClient_SurfacesArrayWrappedErrorBody(t *testing.T) {
	const body = `[{"error":{"code":400,"message":"responses request rejected by gateway","status":"INVALID_ARGUMENT"}}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write error body: %v", err)
		}
	}))
	defer server.Close()

	client := NewOpenAIResponsesClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "custom-model",
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err == nil {
		t.Fatal("expected a provider error, got nil")
	}
	if !strings.Contains(err.Error(), "responses request rejected by gateway") {
		t.Fatalf("Responses API did not surface the array-wrapped body: %v", err)
	}
}

// TestLimitErrorBodyForLog_DropsRuneSplitByTheCap guards the cut at the 64 KiB
// display bound: a multi-byte character split by the cap is dropped rather than
// turned into a replacement character.
func TestLimitErrorBodyForLog_DropsRuneSplitByTheCap(t *testing.T) {
	// The cap falls after the first byte of this 3-byte character.
	body := strings.Repeat("x", maxErrorBodyBytes-1) + "\xe2\x82\xac"
	got := limitErrorBodyForLog([]byte(body))
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("the split character became a replacement character: ...%q", got[len(got)-24:])
	}
	if !strings.HasSuffix(got, "x... (truncated)") {
		t.Fatalf("want the partial character dropped before the truncation marker, got ...%q", got[len(got)-24:])
	}
}
