// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testZIP(t *testing.T, id, version string) []byte {
	t.Helper()
	manifest, err := json.Marshal(testManifest(id, version))
	if err != nil {
		t.Fatal(err)
	}
	return zipEntries(t, map[string]string{ManifestFilename: string(manifest), "rules.json": `{"default_rule":"Check correctness"}`, "docs/review.md": "Review instructions"})
}

func zipEntries(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func zipDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestHTTPSRedirectsAndDownloadBounds(t *testing.T) {
	ctx := context.Background()
	plain := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		t.Error("HTTPS download followed HTTP downgrade")
		writer.Write([]byte("insecure"))
	}))
	defer plain.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/downgrade":
			http.Redirect(writer, request, plain.URL, http.StatusFound)
		case "/redirect":
			http.Redirect(writer, request, "/redirect", http.StatusFound)
		case "/large":
			writer.Header().Set("Content-Length", "100")
			writer.Write([]byte(strings.Repeat("x", 100)))
		case "/chunked":
			writer.(http.Flusher).Flush()
			writer.Write([]byte(strings.Repeat("x", 100)))
		case "/ok":
			writer.Write([]byte("ok"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	store := NewStore(t.TempDir())
	store.client = server.Client()
	for _, test := range []struct{ path, want string }{{"/downgrade", "HTTPS"}, {"/redirect", "too many redirects"}, {"/large", "byte limit"}, {"/chunked", "byte limit"}, {"/missing", "HTTP 404"}} {
		_, _, err := store.download(ctx, server.URL+test.path, 10)
		requireError(t, err, test.want)
	}
	data, _, err := store.download(ctx, server.URL+"/ok", 10)
	if err != nil || string(data) != "ok" {
		t.Fatalf("download = %s, %v", data, err)
	}
	_, _, err = store.download(ctx, plain.URL, 10)
	requireError(t, err, "HTTPS")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, _, err = store.download(cancelled, server.URL+"/ok", 10)
	requireError(t, err, "context canceled")
}

func TestDisplaySource(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"https://user:password@example.com/catalog.json?token=secret#private", "https://example.com/catalog.json"},
		{"https://example.com/catalog.json?", "https://example.com/catalog.json"},
		{"HTTPS://example.com/catalog.json?token=secret", "https://example.com/catalog.json"},
		{"http://example.com/plugin.zip?token=secret", "http://example.com/plugin.zip"},
		{`C:\plugins\example`, `C:\plugins\example`},
		{"/home/user/plugins/example", "/home/user/plugins/example"},
		{"./example@local", "./example@local"},
		{"example@market", "example@market"},
	} {
		if got := DisplaySource(test.source); got != test.want {
			t.Errorf("DisplaySource(%q) = %q, want %q", test.source, got, test.want)
		}
	}
}

func TestDownloadErrorsRedactQueries(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/redirect":
			http.Redirect(writer, request, "/redirect?redirect_token=redirect-secret", http.StatusFound)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	store := NewStore(t.TempDir())
	store.client = server.Client()
	for _, path := range []string{"/missing", "/redirect"} {
		_, _, err := store.download(context.Background(), server.URL+path+"?token=request-secret", 100)
		if err == nil {
			t.Fatal("expected download failure")
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token=") || strings.Contains(err.Error(), "?") {
			t.Fatalf("download error exposed query credentials: %v", err)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := store.download(cancelled, server.URL+"/catalog?token=request-secret", 100)
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "request-secret") {
		t.Fatalf("redacted cancellation error = %v", err)
	}
	// Inner url.Error URLs may refer to redirects rather than the source URL.
	original := &url.Error{Op: "Get", URL: "https://user:password@example.com/redirect?token=redirect-secret", Err: context.DeadlineExceeded}
	redacted := redactDownloadError(original, "https://example.com/catalog?token=request-secret")
	if !errors.Is(redacted, context.DeadlineExceeded) || strings.Contains(redacted.Error(), "secret") || strings.Contains(redacted.Error(), "password") {
		t.Fatalf("nested URL error exposed credentials: %v", redacted)
	}
}
