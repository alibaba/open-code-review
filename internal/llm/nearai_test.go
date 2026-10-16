// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/edwards25519"
	nearai "github.com/nearai/inference-sdk/go"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// Only the hardware quote verifier and TLS binding are substituted in these
// local fixtures. The released SDK verifies the nonce, measurements, exact
// request/response bytes and real Ed25519 signatures, and performs E2EE.
type nearAIGateway struct {
	t                           *testing.T
	key                         ed25519.PrivateKey
	tee, e2ee, tamper, truncate bool
	chatStatus                  int
	signatureKind               string
	signatureDelay              int32
	signatureEntered            chan struct{}
	signatureRelease            chan struct{}
	chats, signatures           atomic.Int32
	mu                          sync.Mutex
	receipts                    map[string]map[string]any
}

func newNearAIGateway(t *testing.T) *nearAIGateway {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &nearAIGateway{t: t, key: key, tee: true, receipts: make(map[string]map[string]any)}
}

func (g *nearAIGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer test" {
		g.t.Error("incorrect authorization")
		w.WriteHeader(401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasPrefix(r.URL.Path, "/v1/model/"):
		provider := "proxy"
		if g.tee {
			provider = "vllm"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"providerType": provider, "attestationSupported": g.tee}})
	case r.URL.Path == "/v1/attestation/report":
		nonce := r.URL.Query().Get("nonce")
		nonceBytes, _ := hex.DecodeString(nonce)
		pub := g.key.Public().(ed25519.PublicKey)
		report := append(append([]byte{}, pub...), nonceBytes...)
		compose := `{"docker_compose_file":"services: {}"}`
		composeHash := sha256.Sum256([]byte(compose))
		measurement := make([]byte, 48)
		measurement[0] = 1
		copy(measurement[1:], composeHash[:])
		digest := sha512.Sum384([]byte("test event"))
		rtmr := sha512.Sum384(append(make([]byte, 48), digest[:]...))
		quote, _ := json.Marshal(nearai.QuoteVerificationResult{TCBStatus: "UpToDate", ReportData: report, MRConfigID: measurement, RTMR3: rtmr[:]})
		attestation := map[string]any{
			"request_nonce": nonce, "signing_algo": "ed25519", "signing_address": hex.EncodeToString(pub),
			"signing_public_key": hex.EncodeToString(pub), "intel_quote": string(quote),
			"report_data": hex.EncodeToString(report),
			"event_log":   []any{map[string]any{"imr": 3, "digest": hex.EncodeToString(digest[:])}},
			"info":        map[string]any{"tcb_info": map[string]any{"app_compose": compose}},
		}
		result := map[string]any{"gateway_attestation": attestation}
		if r.URL.Query().Get("model") != "" {
			result = map[string]any{"model_attestations": []any{attestation}}
		}
		_ = json.NewEncoder(w).Encode(result)
	case r.URL.Path == "/v1/chat/completions":
		id := fmt.Sprintf("chat-%d", g.chats.Add(1))
		if g.chatStatus != 0 {
			w.WriteHeader(g.chatStatus)
			_, _ = io.WriteString(w, `{"error":{"message":"sensitive provider body"}}`)
			return
		}
		if r.Header.Get("X-No-Aliasing") != "true" || r.Header.Get("Accept-Encoding") != "identity" {
			g.t.Error("missing byte-verification headers")
		}
		body, _ := io.ReadAll(r.Body)
		var input struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &input); err != nil {
			g.t.Error(err)
			return
		}
		if input.Model != "canonical/model" {
			g.t.Errorf("model = %q", input.Model)
		}
		transform := func(s string) string { return s }
		if g.e2ee {
			if r.Header.Get("X-Signing-Algo") != "ed25519" || r.Header.Get("X-Model-Pub-Key") == "" {
				g.t.Error("missing E2EE headers")
			}
			for _, plain := range []string{"private review", "task_done", "private tool result", "verified answer", "verified reasoning"} {
				if strings.Contains(string(body), plain) {
					g.t.Errorf("plaintext field escaped E2EE: %s", plain)
				}
			}
			plain, err := nearAIDecrypt(input.Messages[0].Content, g.key)
			if err != nil || plain != "private review" {
				g.t.Errorf("decrypt request: %q %v", plain, err)
			}
			transform = func(s string) string {
				out, err := nearAIEncrypt(s, r.Header.Get("X-Client-Pub-Key"))
				if err != nil {
					g.t.Error(err)
				}
				return out
			}
		} else if r.Header.Get("X-Client-Pub-Key") != "" {
			g.t.Error("E2EE unexpectedly enabled")
		}
		message := map[string]any{"role": "assistant", "content": transform("verified answer"), "reasoning_content": transform("verified reasoning"),
			"tool_calls": []any{map[string]any{"index": 0, "id": "call-1", "type": "function", "function": map[string]any{"name": transform("task_done"), "arguments": transform(`{}`)}}}}
		choice := map[string]any{"index": 0, "finish_reason": "tool_calls", "message": message}
		if input.Stream {
			delete(choice, "message")
			choice["delta"] = message
		}
		reply, _ := json.Marshal(map[string]any{"id": id, "model": input.Model, "choices": []any{choice}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			reply = []byte("data: " + string(reply) + "\r\n\r\ndata: [DONE]\r\n\r\n")
		}
		requestHash, responseHash := sha256.Sum256(body), sha256.Sum256(reply)
		text := fmt.Sprintf("%x:%x", requestHash, responseHash)
		kind := "gateway"
		if g.tee {
			text = input.Model + ":" + text
			kind = "provider_tee"
		}
		if g.signatureKind == "gateway" {
			kind = "gateway"
			text = fmt.Sprintf("%x:%x", requestHash, responseHash)
		}
		signature := ed25519.Sign(g.key, []byte(text))
		if g.tamper {
			signature[0] ^= 1
		}
		g.mu.Lock()
		g.receipts[id] = map[string]any{"signature_kind": kind, "text": text, "signature": hex.EncodeToString(signature), "signing_algo": "ed25519", "signing_address": hex.EncodeToString(g.key.Public().(ed25519.PublicKey))}
		g.mu.Unlock()
		if g.truncate {
			w.Header().Set("Content-Length", fmt.Sprint(len(reply)+100))
			reply = reply[:len(reply)/2]
		}
		_, _ = w.Write(reply)
	case strings.HasPrefix(r.URL.Path, "/v1/signature/"):
		attempt := g.signatures.Add(1)
		if g.signatureEntered != nil {
			select {
			case g.signatureEntered <- struct{}{}:
			default:
			}
			select {
			case <-g.signatureRelease:
			case <-r.Context().Done():
				return
			}
		}
		if attempt <= g.signatureDelay {
			w.WriteHeader(http.StatusTooEarly)
			return
		}
		g.mu.Lock()
		receipt := g.receipts[strings.TrimPrefix(r.URL.Path, "/v1/signature/")]
		g.mu.Unlock()
		_ = json.NewEncoder(w).Encode(receipt)
	default:
		http.NotFound(w, r)
	}
}

func nearAISharedKey(secret *ecdh.PrivateKey, public []byte) (cipher.AEAD, error) {
	pub, err := ecdh.X25519().NewPublicKey(public)
	if err != nil {
		return nil, err
	}
	shared, err := secret.ECDH(pub)
	if err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err = io.ReadFull(hkdf.New(sha256.New, shared, nil, []byte("ed25519_encryption")), key); err != nil {
		return nil, err
	}
	return chacha20poly1305.NewX(key)
}

func nearAIEncrypt(plain, public string) (string, error) {
	pub, err := hex.DecodeString(public)
	if err != nil {
		return "", err
	}
	point, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil {
		return "", err
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	aead, err := nearAISharedKey(ephemeral, point.BytesMontgomery())
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	result := append(ephemeral.PublicKey().Bytes(), nonce...)
	return hex.EncodeToString(append(result, aead.Seal(nil, nonce, []byte(plain), nil)...)), nil
}

func nearAIDecrypt(encrypted string, key ed25519.PrivateKey) (string, error) {
	data, err := hex.DecodeString(encrypted)
	if err != nil || len(data) < 72 {
		return "", fmt.Errorf("invalid ciphertext")
	}
	hash := sha512.Sum512(key.Seed())
	hash[0] &= 248
	hash[31] &= 127
	hash[31] |= 64
	secret, err := ecdh.X25519().NewPrivateKey(hash[:32])
	if err != nil {
		return "", err
	}
	aead, err := nearAISharedKey(secret, data[:32])
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, data[32:56], data[56:], nil)
	return string(plain), err
}

func nearAITestClient(t *testing.T, gateway *nearAIGateway, settings NearAISettings, stream bool) *nearAIClient {
	t.Helper()
	server := httptest.NewServer(gateway)
	t.Cleanup(server.Close)
	cfg := ClientConfig{APIKey: "test", URL: server.URL + "/v1", Model: "canonical/model", Timeout: 5 * time.Second, ExtraBody: map[string]any{"stream": stream}, retryCollector: NewRetryCollector()}
	opts := nearAIOptions(cfg, settings)
	opts.DisableTLSBinding = true
	quote := func(_ context.Context, raw string) (nearai.QuoteVerificationResult, error) {
		var result nearai.QuoteVerificationResult
		err := json.Unmarshal([]byte(raw), &result)
		return result, err
	}
	opts.GatewayVerification.QuoteVerifier = quote
	opts.ModelVerification.QuoteVerifier = quote
	sdk, err := nearai.NewInferenceClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	client := &nearAIClient{cfg: cfg, settings: settings}
	client.attach(sdk)
	t.Cleanup(func() { CloseClient(client) })
	return client
}

func nearAIRequest() ChatRequest {
	return ChatRequest{Messages: []Message{NewTextMessage("user", "private review")}, Tools: []ToolDef{{Type: "function", Function: FunctionDef{Name: "task_done", Description: "Finish review", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}}}
}

func TestNearAIEncryptedToolRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			g := newNearAIGateway(t)
			g.e2ee = true
			c := nearAITestClient(t, g, NearAISettings{true, "model-tee"}, stream)
			req := nearAIRequest()
			first, err := c.CompletionsWithCtx(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			req.Messages = append(req.Messages, NewToolCallMessage(first.VisibleContent(), first.ToolCalls(), first.Native(), first.ReasoningContent()), NewToolResultMessage("call-1", "private tool result"))
			second, err := c.CompletionsWithCtx(context.Background(), req)
			if err != nil || second.Content() != "verified answer" || c.VerificationReport().Verified != 2 {
				t.Fatalf("tool round trip: %+v %v", second, err)
			}
		})
	}
}

func TestNearAIRealSDKCompletions(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, e2ee := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/e2ee=%t", stream, e2ee), func(t *testing.T) {
				gateway := newNearAIGateway(t)
				gateway.e2ee = e2ee
				client := nearAITestClient(t, gateway, NearAISettings{e2ee, "model-tee"}, stream)
				response, err := client.CompletionsWithCtx(metaCtx(testMeta()), nearAIRequest())
				if err != nil {
					t.Fatal(err)
				}
				if response.Content() != "verified answer" || response.Choices[0].Message.ReasoningContent != "verified reasoning" {
					t.Fatalf("unexpected response: %+v", response)
				}
				calls := response.Choices[0].Message.ToolCalls
				if len(calls) != 1 || calls[0].Function.Name != "task_done" || calls[0].Function.Arguments != `{}` {
					t.Fatalf("tool calls: %+v", calls)
				}
				if response.Verification == nil || response.Verification.Status != "verified" || response.Verification.SignatureKind != "provider_tee" {
					t.Fatalf("verification: %+v", response.Verification)
				}
				if response.Usage == nil || response.Usage.TotalTokens != 15 {
					t.Fatalf("usage: %+v", response.Usage)
				}
				if report, err := client.cfg.retryCollector.Freeze("test-run"); err != nil || report != nil {
					t.Fatalf("successful request should not emit retry diagnostics: %+v %v", report, err)
				}
				report := ClientVerificationReport(client)
				if report.Status != "verified" || report.Verified != 1 || ClientVerificationError(client) != nil {
					t.Fatalf("report: %+v", report)
				}
				report.Records[0].Status = "mutated"
				if client.VerificationReport().Records[0].Status != "verified" {
					t.Fatal("mutable report alias")
				}
			})
		}
	}
}

func TestNearAIFailsClosed(t *testing.T) {
	for _, scenario := range []struct {
		name                                string
		e2ee, tee, tamper, truncate, stream bool
		chatStatus                          int
		signatureKind                       string
		chats                               int32
	}{
		{name: "unsupported_e2ee", e2ee: true},
		{name: "unsupported_model_tee"},
		{name: "tampered_json", tee: true, tamper: true, chats: 1},
		{name: "tampered_sse", tee: true, tamper: true, stream: true, chats: 1},
		{name: "truncated_json_no_replay", tee: true, truncate: true, chats: 1},
		{name: "truncated_sse_no_replay", tee: true, truncate: true, stream: true, chats: 1},
		{name: "http_503_no_replay", tee: true, chatStatus: 503, chats: 1},
		{name: "valid_gateway_signature_cannot_satisfy_model_tee", tee: true, signatureKind: "gateway", chats: 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			g := newNearAIGateway(t)
			g.tee, g.e2ee, g.tamper, g.truncate, g.chatStatus = scenario.tee, scenario.e2ee, scenario.tamper, scenario.truncate, scenario.chatStatus
			g.signatureKind = scenario.signatureKind
			c := nearAITestClient(t, g, NearAISettings{scenario.e2ee, "model-tee"}, scenario.stream)
			resp, err := c.CompletionsWithCtx(metaCtx(testMeta()), nearAIRequest())
			if err == nil || resp != nil {
				t.Fatalf("unverified reply escaped: %+v %v", resp, err)
			}
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatal("provider body leaked in error")
			}
			if g.chats.Load() != scenario.chats {
				t.Fatalf("chats = %d, want %d", g.chats.Load(), scenario.chats)
			}
			if scenario.chats == 0 {
				if report, err := c.cfg.retryCollector.Freeze("test-run"); err != nil || report != nil {
					t.Fatalf("preflight invented an HTTP attempt: %+v %v", report, err)
				}
			} else if req := freezeOne(t, c.cfg.retryCollector); req.Outcome != OutcomeFailed {
				t.Fatalf("retry report: %+v", req)
			} else if scenario.tamper && req.Attempts[0].FailurePhase != FailurePhaseVerification {
				t.Fatalf("verification recorded as HTTP success: %+v", req)
			}
			if c.VerificationReport().Failed != 1 || c.VerificationError() == nil {
				t.Fatal("verification failure not retained")
			}
			_, _ = c.CompletionsWithCtx(context.Background(), nearAIRequest())
			if g.chats.Load() != scenario.chats {
				t.Fatal("failed run sent another inference")
			}
		})
	}
}

func TestNearAIGatewayPolicyAndSignatureRetry(t *testing.T) {
	for _, delay := range []int32{0, 1, 3} {
		t.Run(fmt.Sprint(delay), func(t *testing.T) {
			g := newNearAIGateway(t)
			g.tee = false
			g.signatureDelay = delay
			c := nearAITestClient(t, g, NearAISettings{false, "gateway"}, false)
			resp, err := c.CompletionsWithCtx(context.Background(), nearAIRequest())
			if delay < 3 {
				if err != nil || resp.Verification.SignatureKind != "gateway" {
					t.Fatalf("gateway response: %+v %v", resp, err)
				}
			} else if err == nil || resp != nil {
				t.Fatal("unavailable signature accepted")
			}
			if g.chats.Load() != 1 || g.signatures.Load() != min(delay+1, 3) {
				t.Fatalf("chats=%d signatures=%d", g.chats.Load(), g.signatures.Load())
			}
		})
	}
}

func TestNearAIWaitsForVerificationAndCancellation(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRequest), func(t *testing.T) {
			g := newNearAIGateway(t)
			g.signatureEntered = make(chan struct{}, 1)
			g.signatureRelease = make(chan struct{})
			c := nearAITestClient(t, g, NearAISettings{false, "model-tee"}, true)
			defer close(g.signatureRelease)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			var resp *ChatResponse
			var err error
			go func() { defer close(done); resp, err = c.CompletionsWithCtx(ctx, nearAIRequest()) }()
			select {
			case <-g.signatureEntered:
			case <-time.After(5 * time.Second):
				t.Fatal("signature request never arrived")
			}
			select {
			case <-done:
				t.Fatal("tool call released before verification")
			default:
			}
			if c.VerificationReport().Status != "pending" {
				t.Fatal("in-flight response reported verified")
			}
			if cancelRequest {
				cancel()
			} else {
				g.signatureRelease <- struct{}{}
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("client did not return")
			}
			if cancelRequest {
				if !errors.Is(err, context.Canceled) || resp != nil || c.VerificationReport().Cancelled != 1 || c.VerificationError() != nil {
					t.Fatalf("cancelled: %+v %v", resp, err)
				}
			} else if err != nil || resp == nil {
				t.Fatalf("verified: %+v %v", resp, err)
			}
		})
	}
}

func TestNearAIConcurrentCompletions(t *testing.T) {
	g := newNearAIGateway(t)
	g.e2ee = true
	c := nearAITestClient(t, g, NearAISettings{true, "model-tee"}, true)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := c.CompletionsWithCtx(context.Background(), nearAIRequest()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	r := c.VerificationReport()
	if r.Verified != 8 || r.Pending != 0 || r.Failed != 0 {
		t.Fatalf("report: %+v", r)
	}
}

func TestNearAIProviderConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, provider, entry string
		wantErr               bool
		e2ee                  bool
		level                 string
	}{
		{"default", "nearai", `{}`, false, true, "model-tee"},
		{"plaintext_model_tee", "nearai", `{"e2ee":false}`, false, false, "model-tee"},
		{"gateway", "nearai", `{"e2ee":false,"verification_level":"gateway"}`, false, false, "gateway"},
		{"invalid_level", "nearai", `{"verification_level":"none"}`, true, false, ""},
		{"wrong_type", "nearai", `{"e2ee":"false"}`, true, false, ""},
		{"http", "nearai", `{"url":"http://example.com/v1"}`, true, false, ""},
		{"userinfo", "nearai", `{"url":"https://user:secret@example.com/v1"}`, true, false, ""},
		{"query", "nearai", `{"url":"https://example.com/v1?key=secret"}`, true, false, ""},
		{"protocol", "nearai", `{"protocol":"openai-responses"}`, true, false, ""},
		{"body_model", "nearai", `{"extra_body":{"model":"other"}}`, true, false, ""},
		{"retries", "nearai", `{"retry_codes":[429]}`, true, false, ""},
		{"wrong_provider", "openai", `{"e2ee":false}`, true, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NEARAI_API_KEY", "test")
			t.Setenv("OPENAI_API_KEY", "test")
			path := filepath.Join(t.TempDir(), "config.json")
			data := fmt.Sprintf(`{"provider":%q,"providers":{%q:%s}}`, tc.provider, tc.provider, tc.entry)
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			ep, err := ResolveEndpointWithOptions(path, ResolveOptions{Provider: tc.provider, Model: "canonical/model"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("endpoint: %+v %v", ep, err)
			}
			if !tc.wantErr {
				if ep.NearAI == nil || ep.NearAI.E2EE != tc.e2ee || ep.NearAI.VerificationLevel != tc.level {
					t.Fatalf("settings: %+v", ep.NearAI)
				}
				client := NewLLMClient(ep, nil, nil)
				defer CloseClient(client)
				c, ok := client.(*nearAIClient)
				if !ok || c.initErr != nil {
					t.Fatalf("client: %+v", client)
				}
				if c.VerificationReport().Status != "not_requested" {
					t.Fatal("unused client reported verified")
				}
			}
		})
	}
}

func TestNearAITimeoutIncludesVerification(t *testing.T) {
	for _, callerDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint(callerDeadline), func(t *testing.T) {
			g := newNearAIGateway(t)
			g.signatureDelay = 10
			c := nearAITestClient(t, g, NearAISettings{false, "model-tee"}, false)
			ctx := context.Background()
			if callerDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			} else {
				c.cfg.Timeout = 100 * time.Millisecond
			}
			resp, err := c.CompletionsWithCtx(ctx, nearAIRequest())
			if resp != nil || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrRequestTimeout) == callerDeadline {
				t.Fatalf("timeout classification: %+v %v", resp, err)
			}
			if g.chats.Load() != 1 || g.signatures.Load() != 1 || c.VerificationReport().Failed != 1 {
				t.Fatal("deadline allowed a replay or lost failure")
			}
		})
	}
}

func TestNearAIProductionOptionsDoNotBypassVerification(t *testing.T) {
	s, _ := ResolveNearAISettings(nil, "")
	opts := nearAIOptions(ClientConfig{URL: "https://cloud-api.near.ai/v1/chat/completions"}, s)
	if !opts.E2EE || opts.DisableTLSBinding || opts.GatewayVerification.QuoteVerifier != nil || opts.ModelVerification.QuoteVerifier != nil || len(opts.ModelVerification.Policy.AcceptedTCBStatuses) != 2 {
		t.Fatal("production verification defaults changed")
	}
	if opts.BaseURL != "https://cloud-api.near.ai/v1" {
		t.Fatalf("base URL: %q", opts.BaseURL)
	}
	c := NewLLMClient(ResolvedEndpoint{Provider: "nearai", URL: "https://cloud-api.near.ai/v1", Protocol: "openai-responses"}, nil, nil)
	defer CloseClient(c)
	if resp, err := c.CompletionsWithCtx(context.Background(), nearAIRequest()); err == nil || resp != nil {
		t.Fatal("factory bypassed protocol validation")
	}
	for _, cfg := range []ClientConfig{{URL: "http://localhost"}, {URL: "https://cloud-api.near.ai/v1", RetryCodes: []int{429}}} {
		c := newNearAIClient(cfg, s)
		if resp, err := c.CompletionsWithCtx(context.Background(), nearAIRequest()); err == nil || resp != nil || c.VerificationReport().Records[0].Stage != "configuration" {
			t.Fatal("invalid constructor configuration accepted")
		}
		CloseClient(c)
	}
	invalid := newNearAIClient(ClientConfig{}, NearAISettings{VerificationLevel: "none"})
	if invalid.initErr == nil {
		t.Fatal("invalid policy accepted")
	}
	other := NewOpenAIClient(ClientConfig{})
	if ClientVerificationReport(other) != nil || ClientVerificationError(other) != nil {
		t.Fatal("other provider acquired a verification claim")
	}
	CloseClient(other)
}
