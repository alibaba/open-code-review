// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmresolve

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Trace callbacks can run on transport goroutines, even after Do returns.
// Keep only the latest observed phase, never headers or request contents.
type requestTrace struct {
	mu    sync.Mutex
	stage string
	start time.Time
}

func (t *requestTrace) set(stage string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stage = stage
}

func (t *requestTrace) hooks() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart:          func(httptrace.DNSStartInfo) { t.set("resolving DNS") },
		ConnectStart:      func(string, string) { t.set("connecting to endpoint or proxy") },
		TLSHandshakeStart: func() { t.set("TLS handshake") },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil {
				t.set("acquiring connection")
			}
		},
		GotConn: func(httptrace.GotConnInfo) { t.set("sending request") },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				t.set("waiting for response headers")
			}
		},
		GotFirstResponseByte: func() { t.set("reading response headers") },
	}
}

type requestError struct {
	message string
	cause   error
}

func (e *requestError) Error() string { return e.message }
func (e *requestError) Unwrap() error { return e.cause }

func (c *Client) requestFailure(ctx context.Context, trace *requestTrace, rawURL string, status int, cause error, detail string) error {
	trace.mu.Lock()
	stage := trace.stage
	trace.mu.Unlock()
	reason := "network or transport failure"
	hint := "Check connectivity, proxy settings and the gateway logs."
	var networkError net.Error
	var dnsError *net.DNSError
	var certError *tls.CertificateVerificationError
	switch {
	case stage == "building request":
		reason = "invalid parser endpoint URL"
		hint = "Request was not sent. Check parser-base-url."
	case errors.Is(ctx.Err(), context.Canceled):
		reason = "request cancelled by caller"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		reason = "caller deadline exceeded"
		if !errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			// The owning layer explains custom deadlines. Arbitrary cause text
			// may contain private data and must not enter diagnostics here.
			reason = "request deadline exceeded"
		}
	case errors.As(cause, &networkError) && networkError.Timeout():
		reason = "HTTP client or transport timeout"
	case errors.As(cause, &dnsError):
		reason = "DNS lookup failed"
	case errors.As(cause, &certError):
		reason = "TLS certificate verification failed"
	case errors.Is(cause, syscall.ECONNREFUSED):
		reason = "connection refused"
	case errors.Is(cause, io.ErrUnexpectedEOF):
		reason = "connection closed before response completed"
	case status < 200 && status != 0 || status >= 300:
		reason = fmt.Sprintf("HTTP %d", status)
	}
	if stage == "waiting for response headers" || stage == "reading response headers" {
		hint = "The request was sent, but no complete HTTP response headers arrived. Gateway queueing, provider latency or the network may be responsible; check gateway/provider logs."
	} else if stage == "reading response body" {
		hint = "HTTP headers arrived, but the response body could not be read completely. Check gateway/provider streaming and network logs."
	}
	if detail != "" {
		hint = "Provider response: " + c.diagnosticText(detail)
	}
	endpoint := "<invalid URL>"
	if u, err := url.Parse(rawURL); err == nil {
		u.User, u.RawQuery, u.Fragment = nil, "", ""
		endpoint = u.String()
	}
	message := fmt.Sprintf("Parsing LLM request failed: %s.\nStage: %s; elapsed: %s; HTTP status: %s.\nProtocol: %s; model: %s; endpoint: %s.\n%s",
		reason, stage, time.Since(trace.start).Round(time.Millisecond), responseStatus(status),
		quoteDiagnostic(c.ep.Protocol), quoteDiagnostic(c.ep.Model), quoteDiagnostic(endpoint), hint)
	return &requestError{message: message, cause: cause}
}

func responseStatus(status int) string {
	if status == 0 {
		return "not received"
	}
	return fmt.Sprint(status)
}

// Redact provider-controlled text, where credentials might be echoed. Metadata
// fields are quoted separately; URLs lose userinfo, query and fragment first.
func (c *Client) diagnosticText(text string) string {
	secrets := []string{c.ep.Token}
	for name, value := range c.ep.ExtraHeaders {
		if !publicDiagnosticField(name) {
			secrets = append(secrets, value)
		}
	}
	if u, err := url.Parse(c.ep.URL); err == nil {
		if u.User != nil {
			password, _ := u.User.Password()
			secrets = append(secrets, password)
		}
		for _, pair := range strings.Split(u.RawQuery, "&") {
			rawName, rawValue, hasValue := strings.Cut(pair, "=")
			name, _ := url.QueryUnescape(rawName)
			if !hasValue || publicDiagnosticField(name) {
				continue
			}
			// Preserve original escapes: QueryEscape normalizes spaces and hex
			// case, while providers may echo the URL exactly as configured.
			secrets = append(secrets, rawValue)
			if value, err := url.QueryUnescape(rawValue); err == nil {
				secrets = append(secrets, value, url.QueryEscape(value))
			}
		}
	}
	// Match complete encoded values before shorter decoded prefixes. Replace
	// once so later secrets cannot alter an already inserted redaction marker.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	var replacements []string
	for _, secret := range secrets {
		if secret != "" {
			replacements = append(replacements, secret, "[REDACTED]")
		}
	}
	return quoteDiagnostic(strings.NewReplacer(replacements...).Replace(text))
}

// Exempt only ordinary tracing, content-negotiation and version metadata.
// Unknown fields stay conservative, including custom authentication headers.
// Credentials have no minimum length and remain redacted in provider text.
func publicDiagnosticField(name string) bool {
	switch strings.ToLower(name) {
	case "x-trace", "x-request-id", "traceparent", "tracestate", "content-type", "accept", "user-agent", "api-version", "anthropic-version":
		return true
	default:
		return false
	}
}

func quoteDiagnostic(text string) string {
	runes := []rune(text)
	if len(runes) > maxErrorBody {
		text = string(runes[:maxErrorBody]) + "..."
	}
	// Quote controls so an endpoint cannot inject diagnostic lines.
	return fmt.Sprintf("%q", text)
}
