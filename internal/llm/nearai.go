// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	nearai "github.com/nearai/inference-sdk/go"
	"github.com/openai/openai-go/v3"
)

// NearAISettings is captured when a client is created; in-flight calls never
// inherit a later configuration change.
type NearAISettings struct {
	E2EE              bool   `json:"e2ee"`
	VerificationLevel string `json:"verification_level"`
}

func ResolveNearAISettings(e2ee *bool, level string) (NearAISettings, error) {
	s := NearAISettings{E2EE: true, VerificationLevel: "model-tee"}
	if e2ee != nil {
		s.E2EE = *e2ee
	}
	if level != "" {
		s.VerificationLevel = level
	}
	if s.VerificationLevel != "model-tee" && s.VerificationLevel != "gateway" {
		return s, fmt.Errorf("NEAR AI verification_level must be model-tee or gateway")
	}
	return s, nil
}

func validateNearAIEndpoint(endpoint, protocol string, body map[string]any, retries []int) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("NEAR AI url must be an HTTPS API endpoint without credentials, query or fragment")
	}
	if protocol != ProtocolOpenAIChatCompletions {
		return fmt.Errorf("NEAR AI requires the openai Chat Completions protocol")
	}
	if _, ok := body["model"]; ok {
		return fmt.Errorf("NEAR AI model must be configured through model, not extra_body.model")
	}
	if len(retries) != 0 {
		return fmt.Errorf("NEAR AI does not support retry_codes: inference requests are never automatically replayed")
	}
	return nil
}

// VerificationRecord contains diagnostics, not a portable cryptographic proof.
// Request and response bodies and credentials must never be added here.
type VerificationRecord struct {
	FilePath       string    `json:"file_path,omitempty"`
	TaskType       string    `json:"task_type,omitempty"`
	RequestNo      int       `json:"request_no,omitempty"`
	CompletionID   string    `json:"completion_id,omitempty"`
	Model          string    `json:"model"`
	Status         string    `json:"status"`
	Stage          string    `json:"stage"`
	SignatureKind  string    `json:"signature_kind,omitempty"`
	SigningAddress string    `json:"signing_address,omitempty"`
	Code           string    `json:"code,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
	NearAISettings
}

type VerificationReport struct {
	Status    string `json:"status"`
	Verified  int    `json:"verified"`
	Failed    int    `json:"failed"`
	Cancelled int    `json:"cancelled"`
	Pending   int    `json:"pending"`
	NearAISettings
	Records []VerificationRecord `json:"records"`
}

type nearAITransport interface {
	HTTPClient() *http.Client
	BaseURL() string
	Verify(context.Context, string) (nearai.AttestationVerificationResult, error)
	VerifyResponse(context.Context, string) (nearai.VerifiedCompletionResult, error)
	Close() error
}

type nearAIClient struct {
	inner    *OpenAIClient
	sdk      nearAITransport
	cfg      ClientConfig
	settings NearAISettings
	initErr  error
	mu       sync.Mutex
	records  []VerificationRecord
	failure  error
}

type nearAIFailure struct{ code, stage string }

func (e *nearAIFailure) Error() string {
	if e.code == "policy.model_attestation_required" || e.code == "e2ee.model_public_key_required" {
		return "NEAR AI model cannot satisfy the configured policy (" + e.code + "). Choose a model with model attestation and E2EE support, or explicitly set providers.nearai.e2ee=false and providers.nearai.verification_level=gateway for Gateway-only verification"
	}
	return "NEAR AI " + e.stage + " failed (" + e.code + ")"
}

func newNearAIClient(cfg ClientConfig, settings NearAISettings) *nearAIClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Minute
	}
	c := &nearAIClient{cfg: cfg, settings: settings}
	resolved, err := ResolveNearAISettings(&settings.E2EE, settings.VerificationLevel)
	if err != nil {
		c.initErr = &nearAIFailure{code: "configuration.invalid", stage: "configuration"}
		return c
	}
	c.settings = resolved
	if err := validateNearAIEndpoint(cfg.URL, ProtocolOpenAIChatCompletions, cfg.ExtraBody, cfg.RetryCodes); err != nil {
		c.initErr = &nearAIFailure{code: "configuration.invalid", stage: "configuration"}
		return c
	}
	sdk, err := nearai.NewInferenceClient(nearAIOptions(cfg, resolved))
	if err != nil {
		c.initErr = nearAIError(err, "configuration")
		return c
	}
	c.attach(sdk)
	return c
}

func nearAIOptions(cfg ClientConfig, settings NearAISettings) nearai.InferenceOptions {
	baseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/chat/completions")
	opts := nearai.InferenceOptions{
		ClientOptions: nearai.ClientOptions{APIKey: cfg.APIKey, BaseURL: baseURL, HTTPClient: httpClientWithHeaderTimeout(cfg.Timeout)},
		E2EE:          settings.E2EE,
	}
	if settings.VerificationLevel == "model-tee" {
		// Explicit SDK-default statuses also require model evidence when E2EE
		// is off, including after an attestation cache refresh.
		opts.ModelVerification.Policy.AcceptedTCBStatuses = []string{"UpToDate", "OutOfDate"}
	}
	return opts
}

func (c *nearAIClient) attach(sdk nearAITransport) {
	c.sdk = sdk
	cfg := c.cfg
	cfg.URL = sdk.BaseURL()
	cfg.httpClient = sdk.HTTPClient()
	cfg.disableRetries = true
	cfg.deferFinalize = true
	c.inner = NewOpenAIClient(cfg)
}

func nearAIError(err error, stage string) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var failure *nearAIFailure
	if errors.As(err, &failure) {
		return failure
	}
	code := "request.failed"
	var sdkError *nearai.Error
	if errors.As(err, &sdkError) {
		code = sdkError.Code
	} else {
		var apiError *openai.Error
		if errors.As(err, &apiError) {
			code = fmt.Sprintf("http.%d", apiError.StatusCode)
		}
	}
	return &nearAIFailure{code: code, stage: stage}
}

func (c *nearAIClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (resp *ChatResponse, err error) {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}
	record := VerificationRecord{Model: model, Status: "pending", Stage: "preflight", NearAISettings: c.settings}
	if meta, ok := RequestMetaFromContext(ctx); ok {
		record.FilePath, record.TaskType, record.RequestNo = meta.FilePath, meta.TaskType, meta.RequestNo
	}
	c.mu.Lock()
	index := len(c.records)
	c.records = append(c.records, record)
	priorFailure := c.failure
	c.mu.Unlock()
	defer func() {
		panicked := recover()
		if panicked != nil {
			err = &nearAIFailure{code: "request.panicked", stage: record.Stage}
		}
		if err != nil {
			if class, phase, ok := classifyBoundaryError(err); ok {
				reviseAttempt(ctx, c.cfg.retryCollector, class, phase)
			}
			err = nearAIError(err, record.Stage)
			resp = nil
			record.Status = "failed"
			record.Code = "request.failed"
			var failure *nearAIFailure
			if errors.As(err, &failure) {
				record.Code = failure.code
				record.Stage = failure.stage
			}
			if errors.Is(err, context.Canceled) {
				record.Status, record.Code = "cancelled", "request.cancelled"
			}
			if errors.Is(err, context.DeadlineExceeded) {
				record.Code = "request.timeout"
			}
			err = describeTimeout(parent, err)
		}
		record.CheckedAt = time.Now().UTC()
		c.mu.Lock()
		c.records[index] = record
		if record.Status == "failed" && c.failure == nil {
			c.failure = err
		}
		c.mu.Unlock()
		if resp != nil {
			verified := record
			resp.Verification = &verified
		}
		finalizeRequest(ctx, c.cfg.retryCollector, err)
		if panicked != nil {
			panic(panicked)
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if priorFailure != nil {
		return nil, &nearAIFailure{code: "request.blocked_after_failure", stage: "preflight"}
	}
	if c.initErr != nil {
		return nil, c.initErr
	}
	evidence, err := c.sdk.Verify(ctx, model)
	if err != nil {
		return nil, err
	}
	if c.settings.VerificationLevel == "model-tee" && len(evidence.Models) == 0 {
		return nil, &nearAIFailure{code: "policy.model_attestation_required", stage: "preflight"}
	}
	record.Stage = "inference"
	resp, err = c.inner.CompletionsWithCtx(ctx, req)
	if err != nil {
		return nil, err
	}
	record.CompletionID = resp.ID
	record.Stage = "response verification"
	verified, err := c.verifyResponse(ctx, resp.ID)
	if err != nil {
		return nil, err
	}
	record.SignatureKind = verified.SignatureKind
	if verified.Attestation != nil {
		record.SigningAddress = verified.Attestation.Signer.SigningAddress
	}
	if c.settings.VerificationLevel == "model-tee" && verified.SignatureKind != "provider_tee" {
		return nil, &nearAIFailure{code: "policy.model_tee_required", stage: record.Stage}
	}
	if verified.SignatureKind != "provider_tee" && verified.SignatureKind != "gateway" {
		return nil, &nearAIFailure{code: "signature.kind_mismatch", stage: record.Stage}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	record.Status = "verified"
	return resp, nil
}

func (c *nearAIClient) verifyResponse(ctx context.Context, id string) (nearai.VerifiedCompletionResult, error) {
	for attempt := 0; ; attempt++ {
		result, err := c.sdk.VerifyResponse(ctx, id)
		var sdkError *nearai.Error
		if err == nil || attempt == 2 || !errors.As(err, &sdkError) || !sdkError.Retryable {
			return result, err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nearai.VerifiedCompletionResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *nearAIClient) VerificationError() error { c.mu.Lock(); defer c.mu.Unlock(); return c.failure }

func (c *nearAIClient) VerificationReport() *VerificationReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := &VerificationReport{Status: "not_requested", NearAISettings: c.settings, Records: append([]VerificationRecord{}, c.records...)}
	for _, record := range r.Records {
		switch record.Status {
		case "verified":
			r.Verified++
		case "failed":
			r.Failed++
		case "cancelled":
			r.Cancelled++
		default:
			r.Pending++
		}
	}
	switch {
	case r.Failed > 0:
		r.Status = "failed"
	case r.Pending > 0:
		r.Status = "pending"
	case r.Verified > 0:
		r.Status = "verified"
	case r.Cancelled > 0:
		r.Status = "cancelled"
	}
	return r
}

func (c *nearAIClient) Close() error {
	if c.sdk != nil {
		return c.sdk.Close()
	}
	return nil
}

func ClientVerificationError(client LLMClient) error {
	if c, ok := client.(interface{ VerificationError() error }); ok {
		return c.VerificationError()
	}
	return nil
}

func ClientVerificationReport(client LLMClient) *VerificationReport {
	if c, ok := client.(interface{ VerificationReport() *VerificationReport }); ok {
		return c.VerificationReport()
	}
	return nil
}

func CloseClient(client LLMClient) {
	if c, ok := client.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}
