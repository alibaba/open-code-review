// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestValidateRemoteURL(t *testing.T) {
	for _, endpoint := range []string{
		"https://mcp.example.com/mcp", "https://localhost:8443/mcp",
		"http://localhost:8000/mcp", "http://LOCALHOST/mcp", "http://127.0.0.1/mcp",
		"http://127.1.2.3/mcp", "http://[::1]:8000/mcp", "http://[::ffff:127.0.0.1]/mcp",
	} {
		if err := ValidateRemoteURL(endpoint); err != nil {
			t.Errorf("ValidateRemoteURL(%q): %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"", "/mcp", "https:///mcp", "https://:443/mcp", "https://%zz/mcp",
		"https://user:password@example.com/mcp", "https://example.com/mcp#fragment",
		"ftp://localhost/mcp", "http://example.com/mcp", "http://192.168.1.1/mcp",
		"http://10.0.0.1/mcp", "http://0.0.0.0/mcp", "http://[::]/mcp",
		"http://localhost.example.com/mcp", "http://127.0.0.1.example.com/mcp",
		"http://127.1/mcp", "http://2130706433/mcp", "http://[::ffff:192.168.1.1]/mcp",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if err := ValidateRemoteURL(endpoint); err == nil {
				t.Fatal("invalid remote endpoint accepted")
			}
			// Manual configurations bypass manifest validation but use the same client.
			_, err := NewRemoteClient(context.Background(), "test", endpoint, nil, "test")
			if err == nil || !strings.Contains(err.Error(), "remote MCP requires") {
				t.Fatalf("client did not reject the endpoint before connecting: %v", err)
			}
		})
	}
}

func TestRemoteRedirectOrigin(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		allowed  bool
	}{
		{"https://example.com/mcp", "https://EXAMPLE.com:443/tools", true},
		{"https://example.com/mcp", "https://example.com/tools#fragment", true},
		{"http://localhost/mcp", "http://LOCALHOST:80/tools", true},
		{"https://example.com/mcp", "https://example.com:8443/tools", false},
		{"https://example.com/mcp", "https://other.example.com/mcp", false},
		{"https://example.com/mcp", "http://example.com/mcp", false},
		{"https://localhost/mcp", "http://localhost/mcp", false},
		{"https://example.com/mcp", "https://user@example.com/mcp", false},
	} {
		first, _ := http.NewRequest(http.MethodGet, tc.from, nil)
		next, _ := http.NewRequest(http.MethodGet, tc.to, nil)
		if err := remoteRedirect(next, []*http.Request{first}); (err == nil) != tc.allowed {
			t.Errorf("redirect %s -> %s: %v", tc.from, tc.to, err)
		}
	}
	request, _ := http.NewRequest(http.MethodGet, "https://example.com/mcp", nil)
	if err := remoteRedirect(request, make([]*http.Request, 10)); err == nil {
		t.Fatal("redirect limit was removed")
	}
}

func TestRemoteClientRejectsRedirectBeforeSendingHeaders(t *testing.T) {
	for _, mode := range []string{"https-to-http", "https-cross-origin", "http-cross-origin"} {
		t.Run(mode, func(t *testing.T) {
			var destinationRequests atomic.Int64
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				destinationRequests.Add(1)
				w.WriteHeader(http.StatusNoContent)
			})
			var destination *httptest.Server
			if mode == "https-cross-origin" {
				destination = httptest.NewTLSServer(handler)
			} else {
				destination = httptest.NewServer(handler)
			}
			defer destination.Close()
			var originRequests atomic.Int64
			redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originRequests.Add(1)
				if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-Api-Key") != "test-key" {
					t.Error("configured headers did not reach the original server")
				}
				http.Redirect(w, r, destination.URL+"/mcp", http.StatusTemporaryRedirect)
			})
			var origin *httptest.Server
			if mode == "http-cross-origin" {
				origin = httptest.NewServer(redirect)
			} else {
				origin = httptest.NewTLSServer(redirect)
			}
			defer origin.Close()
			previous := http.DefaultTransport
			http.DefaultTransport = origin.Client().Transport
			defer func() { http.DefaultTransport = previous }()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := NewRemoteClient(ctx, "redirect", origin.URL+"/mcp", map[string]string{
				"Authorization": "Bearer test-token", "X-Api-Key": "test-key",
			}, "test")
			if err == nil || !strings.Contains(err.Error(), "redirects must keep") {
				t.Fatalf("expected redirect rejection, got %v", err)
			}
			if originRequests.Load() == 0 || destinationRequests.Load() != 0 {
				t.Fatalf("origin requests=%d, destination requests=%d", originRequests.Load(), destinationRequests.Load())
			}
		})
	}
}

func TestRemoteClientTLSAndLocalHTTP(t *testing.T) {
	for _, tls := range []bool{false, true} {
		t.Run(map[bool]string{false: "loopback-http", true: "https"}[tls], func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
			server.AddTool(&mcp.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}},
				func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil
				})
			mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true})
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer local-test" {
					t.Error("missing authentication header after same-origin redirect")
				}
				if r.URL.Path == "/redirect" {
					http.Redirect(w, r, "/mcp#local-fragment", http.StatusTemporaryRedirect)
					return
				}
				mcpHandler.ServeHTTP(w, r)
			})
			var ts *httptest.Server
			if tls {
				ts = httptest.NewTLSServer(handler)
			} else {
				ts = httptest.NewServer(handler)
			}
			defer ts.Close()
			previous := http.DefaultTransport
			http.DefaultTransport = ts.Client().Transport
			defer func() { http.DefaultTransport = previous }()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, err := NewRemoteClient(ctx, "test", ts.URL+"/redirect", map[string]string{"Authorization": "Bearer local-test"}, "test")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if len(client.Tools()) != 1 || client.Tools()[0].Name != "ping" {
				t.Fatalf("tools = %v", client.Tools())
			}
			if got, err := client.CallTool(ctx, "ping", nil); err != nil || got != "pong" {
				t.Fatalf("CallTool = %q, %v", got, err)
			}
		})
	}
}
