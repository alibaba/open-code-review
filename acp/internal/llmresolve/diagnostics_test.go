// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmresolve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
)

func TestRequestTimeoutReportsObservedPhase(t *testing.T) {
	for _, body := range []bool{false, true} {
		name := "headers"
		if body {
			name = "body"
		}
		t.Run(name, func(t *testing.T) {
			received := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if body {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				close(received)
				<-r.Context().Done()
			}))
			defer srv.Close()
			client, _ := NewClient(Endpoint{URL: srv.URL, Token: "test-secret", Model: "test-model", Protocol: ProtocolAnthropic})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := client.CallTool(ctx, testRequest())
			select {
			case <-received:
			default:
				t.Fatal("request never reached server")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected deadline error, got %v", err)
			}
			stage, status := "waiting for response headers", "not received"
			if body {
				stage, status = "reading response body", "200"
			}
			for _, want := range []string{"caller deadline exceeded", "Stage: " + stage, "elapsed:", "HTTP status: " + status, "test-model", "anthropic"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("missing %q in %v", want, err)
				}
			}
		})
	}
}

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRequestFailureClassificationAndRedaction(t *testing.T) {
	for _, test := range []struct {
		name  string
		trace func(*httptrace.ClientTrace)
		err   error
		want  string
	}{
		{"dns", func(tr *httptrace.ClientTrace) { tr.DNSStart(httptrace.DNSStartInfo{}) }, &net.DNSError{Err: "not found", Name: "secret-host"}, "DNS lookup failed"},
		{"connect", func(tr *httptrace.ClientTrace) { tr.ConnectStart("tcp", "secret-host") }, errors.New("fake-token-secret"), "connecting to endpoint or proxy"},
		{"refused", func(tr *httptrace.ClientTrace) { tr.ConnectStart("tcp", "secret-host") }, syscall.ECONNREFUSED, "connection refused"},
		{"tls", func(tr *httptrace.ClientTrace) { tr.TLSHandshakeStart() }, &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "TLS certificate verification failed"},
		{"tls failure", func(tr *httptrace.ClientTrace) {
			tr.TLSHandshakeStart()
			tr.TLSHandshakeDone(tls.ConnectionState{}, io.EOF)
		}, io.EOF, "TLS handshake"},
		{"timeout", func(tr *httptrace.ClientTrace) { tr.GotConn(httptrace.GotConnInfo{}) }, &net.DNSError{IsTimeout: true}, "HTTP client or transport timeout"},
		{"write", func(tr *httptrace.ClientTrace) {
			tr.GotConn(httptrace.GotConnInfo{})
			tr.WroteRequest(httptrace.WroteRequestInfo{Err: io.ErrUnexpectedEOF})
		}, io.ErrUnexpectedEOF, "sending request"},
		{"partial headers", func(tr *httptrace.ClientTrace) {
			tr.TLSHandshakeDone(tls.ConnectionState{}, nil)
			tr.WroteRequest(httptrace.WroteRequestInfo{})
			tr.GotFirstResponseByte()
		}, io.ErrUnexpectedEOF, "reading response headers"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, _ := NewClient(Endpoint{URL: "http://example.invalid", Token: "fake-token-secret", Model: "test-model", Protocol: ProtocolOpenAI})
			client.http.Transport = diagnosticTransport(func(r *http.Request) (*http.Response, error) {
				trace := httptrace.ContextClientTrace(r.Context())
				if trace == nil {
					t.Fatal("HTTP request has no diagnostic trace")
				}
				test.trace(trace)
				return nil, &url.Error{Op: "Post", URL: "http://user:password@example.invalid?key=fake-token-secret", Err: test.err}
			})
			_, err := client.CallTool(context.Background(), testRequest())
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, test.err) {
				t.Fatalf("got %v, want %s and original cause", err, test.want)
			}
			for _, secret := range []string{"fake-token-secret", "password", "secret-host"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("credential or raw transport details exposed: %v", err)
				}
			}
		})
	}
}

func TestRequestCancellationIsNotReportedAsTimeout(t *testing.T) {
	client, _ := NewClient(Endpoint{URL: "http://example.invalid", Token: "test-secret", Model: "test-model", Protocol: ProtocolAnthropic})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.http.Transport = diagnosticTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	_, err := client.CallTool(ctx, testRequest())
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "cancelled by caller") || strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("cancellation diagnostic = %v", err)
	}
}

func TestParserRejectionPreservesRequestDiagnostics(t *testing.T) {
	client, _ := NewClient(Endpoint{URL: "http://example.invalid", Token: "test-secret", Model: "test-model", Protocol: ProtocolAnthropic})
	client.http.Transport = diagnosticTransport(func(r *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(r.Context())
		trace.GotConn(httptrace.GotConnInfo{})
		trace.WroteRequest(httptrace.WroteRequestInfo{})
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	parser := intent.NewParser(client, nil).WithTimeout(20 * time.Millisecond)
	result, err := parser.Parse(context.Background(), "review current changes", intent.NewState())
	if err != nil || result.Kind != intent.KindReject {
		t.Fatalf("parse result=%+v error=%v", result, err)
	}
	for _, want := range []string{"parser timeout of 20ms", "request deadline exceeded", "OCR has not started", "waiting for response headers", "test-model", "no complete HTTP response headers"} {
		if !strings.Contains(result.Reject.Reason, want) {
			t.Errorf("missing %q in %s", want, result.Reject.Reason)
		}
	}
	if strings.Contains(result.Reject.Reason, "caller deadline exceeded") {
		t.Errorf("parser timeout attributed to caller: %s", result.Reject.Reason)
	}
}

func TestCustomDeadlineCauseIsNotExposed(t *testing.T) {
	client, err := NewClient(Endpoint{URL: "http://example.invalid", Token: "test-secret", Model: "test-model", Protocol: ProtocolAnthropic})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), errors.New("private-custom-cause"))
	defer cancel()
	client.http.Transport = diagnosticTransport(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	})
	_, err = client.CallTool(ctx, testRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected original deadline error, got %v", err)
	}
	if !strings.Contains(err.Error(), "request deadline exceeded") || strings.Contains(err.Error(), "caller deadline exceeded") || strings.Contains(err.Error(), "private-custom-cause") {
		t.Fatalf("unsafe or misattributed deadline diagnostic: %v", err)
	}
}

func TestHTTPFailureRedactsEncodedQueryCredentials(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		echo  string
	}{
		{"plus as space", "key=a+b", "a+b|a b"},
		{"percent space", "key=a%20b", "a%20b|a b|a+b"},
		{"literal plus", "key=a%2Bb", "a%2Bb|a+b"},
		{"lowercase slash", "key=a%2fb", "a%2fb|a/b|a%2Fb"},
		{"equals", "key=a%3Db", "a%3Db|a=b"},
		{"overlapping percent", "key=a%25", "a%25|a%"},
		{"short percent", "key=%25", "%25|%"},
		{"repeated encoded key", "%6bey=a%20b&key=c%2fd", "a%20b|a b|a+b|c%2fd|c/d|c%2Fd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RawQuery != test.query {
					t.Errorf("request query changed: %q", r.URL.RawQuery)
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "Echo: "+test.echo)
			}))
			defer srv.Close()
			client, err := NewClient(Endpoint{URL: srv.URL + "?" + test.query, Token: "test-secret", Model: "test-model", Protocol: ProtocolAnthropic})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.post(context.Background(), client.ep.URL, map[string]string{}, nil)
			if err == nil {
				t.Fatal("expected HTTP error")
			}
			redacted := strings.TrimSuffix(strings.Repeat("[REDACTED]|", strings.Count(test.echo, "|")+1), "|")
			if !strings.Contains(err.Error(), `Provider response: "Echo: `+redacted+`"`) {
				t.Fatalf("encoded credential not fully redacted: %v", err)
			}
		})
	}
}

func TestDiagnosticsBoundProviderOutput(t *testing.T) {
	client := &Client{ep: Endpoint{Token: "test-secret"}}
	value := client.diagnosticText(strings.Repeat("x", 1000) + "test-secret")
	if len(value) > maxErrorBody+5 || !strings.HasSuffix(value, "...\"") || strings.Contains(value, "test-secret") {
		t.Fatalf("unbounded diagnostic: %d bytes", len(value))
	}
}

func TestHTTPFailureIncludesSafeProviderMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"No provider channel; fake-token-secret header-secret query-secret password-secret\nforged line"}}`)
	}))
	defer srv.Close()
	client, _ := NewClient(Endpoint{URL: srv.URL, Token: "fake-token-secret", Model: "test-model", Protocol: ProtocolAnthropic,
		ExtraHeaders: map[string]string{"X-Access-Token": "header-secret"}})
	// Exercise diagnostic redaction independently of the provider URL builder.
	u, _ := url.Parse(srv.URL)
	u.User = url.UserPassword("fake-user", "password-secret")
	u.RawQuery = "key=query-secret"
	client.ep.URL = u.String()
	_, err := client.post(context.Background(), u.String(), map[string]string{"prompt": "private-prompt"}, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") || !strings.Contains(err.Error(), "No provider channel") {
		t.Fatalf("missing provider diagnostic: %v", err)
	}
	for _, secret := range []string{"fake-token-secret", "header-secret", "query-secret", "password-secret", "fake-user", "private-prompt", "\nforged"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("secret/control exposed: %v", err)
		}
	}
}

func TestDiagnosticMetadataSurvivesShortConfigurationValues(t *testing.T) {
	client, _ := NewClient(Endpoint{
		URL:   "http://api:p5@10.0.0.1:8080/v1/messages?api-version=2024-01-01&key=q7#private",
		Token: "q7", Model: "gpt-4.1", Protocol: ProtocolAnthropic,
		ExtraHeaders: map[string]string{"X-Trace": "1", "Content-Type": "json", "X-Access-Token": "z9"},
	})
	client.http.Transport = diagnosticTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("api version 2024-01-01, trace 1, json; credentials: q7 p5 z9")), Header: make(http.Header)}, nil
	})
	_, err := client.post(context.Background(), client.ep.URL, map[string]string{}, nil)
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	for _, want := range []string{`Protocol: "anthropic"`, `model: "gpt-4.1"`, `endpoint: "http://10.0.0.1:8080/v1/messages"`, "api version 2024-01-01, trace 1, json", "HTTP 503", "[REDACTED]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	for _, secret := range []string{"q7", "p5", "z9", "api:p5@", "?api-version", "#private"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("credential/URL decoration exposed: %v", err)
		}
	}
}

func TestInvalidURLFailsBeforeTransport(t *testing.T) {
	for _, rawURL := range []string{"http://example.invalid/%zz", "http://example.invalid/\nprivate"} {
		client := &Client{http: &http.Client{Transport: diagnosticTransport(func(*http.Request) (*http.Response, error) {
			t.Error("invalid URL reached transport")
			return nil, errors.New("unexpected transport call")
		})}}
		_, err := client.post(context.Background(), rawURL, map[string]string{}, nil)
		if err == nil {
			t.Fatal("invalid URL accepted")
		}
		for _, want := range []string{"invalid parser endpoint URL", "Stage: building request", "Request was not sent", "HTTP status: not received"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("missing %q in %v", want, err)
			}
		}
		if strings.Contains(err.Error(), "network or transport failure") || strings.Contains(err.Error(), "acquiring connection") {
			t.Errorf("local build failure misclassified: %v", err)
		}
	}
}
