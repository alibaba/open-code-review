// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeS3Server is an in-memory S3-compatible endpoint that verifies the SigV4
// authorization header of every request before answering it. It stands in for
// MinIO/Ceph in tests: the client code under test is the same code a real
// deployment runs, including request building, signing and response parsing.
type fakeS3Server struct {
	t       *testing.T
	objects map[string][]byte
	mu      sync.Mutex
	// requireAuth fails every request whose Authorization header is absent or
	// does not carry the expected credential scope.
	requireAuth bool
	// pageSize makes List truncate at this many keys, which exercises
	// continuation-token handling.
	pageSize int
	// requests records the method and key of every request received.
	requests []string
	// paths records the request path of every request received.
	paths []string
}

func newFakeS3Server(t *testing.T) *fakeS3Server {
	t.Helper()
	return &fakeS3Server{t: t, objects: map[string][]byte{}, requireAuth: true}
}

func (f *fakeS3Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.requireAuth {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, algorithm+" Credential=test-access-key/") ||
			!strings.Contains(auth, "SignedHeaders=") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<Error><Code>AccessDenied</Code></Error>"))
			return
		}
		if r.Header.Get("x-amz-date") == "" || r.Header.Get("x-amz-content-sha256") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
	f.requests = append(f.requests, r.Method+" "+key)
	f.paths = append(f.paths, r.URL.Path)

	switch r.Method {
	case http.MethodPut:
		body := make([]byte, 0, 64)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		f.objects[key] = body
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if _, ok := r.URL.Query()["list-type"]; ok {
			f.writeList(w, r)
			return
		}
		data, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<Error><Code>NoSuchKey</Code></Error>"))
			return
		}
		_, _ = w.Write(data)
	case http.MethodDelete:
		if _, ok := f.objects[key]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3Server) writeList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	var keys []string
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	start := 0
	if token := r.URL.Query().Get("continuation-token"); token != "" {
		parsed, err := strconv.Atoi(token)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		start = parsed
	}
	end := len(keys)
	truncated := false
	if f.pageSize > 0 && start+f.pageSize < len(keys) {
		end = start + f.pageSize
		truncated = true
	}

	var b strings.Builder
	b.WriteString("<ListBucketResult>")
	for _, key := range keys[start:end] {
		b.WriteString("<Contents><Key>" + key + "</Key></Contents>")
	}
	if truncated {
		b.WriteString("<IsTruncated>true</IsTruncated>")
		b.WriteString("<NextContinuationToken>" + strconv.Itoa(end) + "</NextContinuationToken>")
	} else {
		b.WriteString("<IsTruncated>false</IsTruncated>")
	}
	b.WriteString("</ListBucketResult>")
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(b.String()))
}

// directClient returns a client that ignores proxy environment variables, which
// a test host reachable only from this process requires. Production uses the
// default transport so a configured proxy still applies.
func directClient() *http.Client {
	return &http.Client{
		Timeout:   s3RequestTimeout,
		Transport: &http.Transport{Proxy: nil},
	}
}

// newTestBackend wires an S3 backend to a fake server.
func newTestBackend(t *testing.T, configure func(*fakeS3Server)) (*s3Backend, *fakeS3Server) {
	t.Helper()
	fake := newFakeS3Server(t)
	if configure != nil {
		configure(fake)
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	backend, err := newS3Backend(S3Config{
		Endpoint:        server.URL,
		Bucket:          "test-bucket",
		Region:          "us-west-2",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	return backend, fake
}

func TestS3RoundTrip(t *testing.T) {
	backend, fake := newTestBackend(t, nil)
	ctx := context.Background()

	if err := backend.Put(ctx, "sessions/repo/id-1.jsonl", []byte("{\"type\":\"session_start\"}\n")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, err := backend.Get(ctx, "sessions/repo/id-1.jsonl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "{\"type\":\"session_start\"}\n" {
		t.Errorf("Get returned %q", data)
	}
	if got := len(fake.objects); got != 1 {
		t.Errorf("server holds %d objects, want 1", got)
	}

	if err := backend.Delete(ctx, "sessions/repo/id-1.jsonl"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := backend.Get(ctx, "sessions/repo/id-1.jsonl"); !errors.Is(err, ErrNotExist) {
		t.Errorf("Get after Delete: err = %v, want ErrNotExist", err)
	}
	// Deleting an absent object is not an error: a retried delete must succeed.
	if err := backend.Delete(ctx, "sessions/repo/id-1.jsonl"); err != nil {
		t.Errorf("second Delete: %v", err)
	}
}

func TestS3RequestPaths(t *testing.T) {
	// The path of each verb is part of the SigV4 canonical request, so a wrong
	// one is a wrong signature: a bucket-level listing must address the bucket
	// itself, without an extra slash that would read as an object named "/".
	backend, fake := newTestBackend(t, nil)
	ctx := context.Background()

	if err := backend.Put(ctx, "sessions/repo/id.jsonl", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := backend.List(ctx, "sessions/"); err != nil {
		t.Fatalf("List: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	paths := append([]string(nil), fake.paths...)
	if len(paths) != 2 {
		t.Fatalf("server saw %d requests, want 2: %v", len(paths), paths)
	}
	if paths[0] != "/test-bucket/sessions/repo/id.jsonl" {
		t.Errorf("object request path = %q", paths[0])
	}
	if paths[1] != "/test-bucket" {
		t.Errorf("bucket listing path = %q, want /test-bucket with no trailing slash", paths[1])
	}
	for _, path := range paths {
		if strings.Contains(path, "//") {
			t.Errorf("request path %q contains a doubled slash", path)
		}
	}
}

func TestS3ObjectVerbsRejectEmptyKey(t *testing.T) {
	backend, _ := newTestBackend(t, nil)
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"Put":    func() error { return backend.Put(ctx, "/", []byte("x")) },
		"Get":    func() error { _, err := backend.Get(ctx, ""); return err },
		"Delete": func() error { return backend.Delete(ctx, "") },
	} {
		if err := call(); err == nil {
			t.Errorf("%s accepted an empty key", name)
		}
	}
}

func TestS3PutReplacesAndQuotesKey(t *testing.T) {
	backend, fake := newTestBackend(t, nil)
	ctx := context.Background()

	// The captured repository path in a real key is percent-encoded, and a key
	// with a space and a plus proves the signing path escapes rather than
	// truncating at the first unusual byte.
	key := "sessions/C:-Users-dev-My Repo+1/session id.jsonl"
	if err := backend.Put(ctx, key, []byte("first")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := backend.Put(ctx, key, []byte("second")); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if got := string(fake.objects["sessions/C:-Users-dev-My Repo+1/session id.jsonl"]); got != "second" {
		t.Errorf("stored %q, want the replacement bytes", got)
	}
	if err := backend.Put(ctx, "another.jsonl", []byte("x")); err != nil {
		t.Fatalf("Put another: %v", err)
	}
	if _, ok := fake.objects["another.jsonl"]; !ok {
		t.Errorf("keys escaped into a different object: %v", fake.objects)
	}
}

func TestS3ListPaginates(t *testing.T) {
	backend, _ := newTestBackend(t, func(f *fakeS3Server) { f.pageSize = 2 })
	ctx := context.Background()

	for _, key := range []string{
		"sessions/repo/id-1.jsonl",
		"sessions/repo/id-2.jsonl",
		"sessions/repo/id-3.jsonl",
		"sessions/other/id-4.jsonl",
	} {
		if err := backend.Put(ctx, key, []byte("x")); err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	keys, err := backend.List(ctx, "sessions/repo/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"sessions/repo/id-1.jsonl", "sessions/repo/id-2.jsonl", "sessions/repo/id-3.jsonl"}
	if len(keys) != len(want) {
		t.Fatalf("List returned %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("List returned %v, want %v", keys, want)
		}
	}
}

func TestS3Errors(t *testing.T) {
	ctx := context.Background()

	// A 403 is reported with the server's own explanation, which is where an
	// S3-compatible server states that the signature was rejected.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<Error><Code>SignatureDoesNotMatch</Code></Error>"))
	}))
	defer server.Close()
	denied, err := newS3Backend(S3Config{
		Endpoint: server.URL, Bucket: "test-bucket", Region: "us-west-2",
		AccessKeyID: "a", SecretAccessKey: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	if err := denied.Put(ctx, "sessions/repo/id.jsonl", []byte("x")); err == nil {
		t.Fatal("Put against a rejecting server returned no error")
	} else if !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Errorf("error lost the server's reason: %v", err)
	}

	// A malformed list body is reported rather than silently yielding no keys.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not xml"))
	}))
	defer broken.Close()
	badList, err := newS3Backend(S3Config{
		Endpoint: broken.URL, Bucket: "test-bucket", Region: "us-west-2",
		AccessKeyID: "a", SecretAccessKey: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	if _, err := badList.List(ctx, "sessions/"); err == nil {
		t.Error("List with an undecodable body returned no error")
	}
}

func TestS3UnreachableEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	backend, err := newS3Backend(S3Config{
		Endpoint: url, Bucket: "test-bucket", Region: "us-west-2",
		AccessKeyID: "a", SecretAccessKey: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	if err := backend.Put(context.Background(), "sessions/repo/id.jsonl", []byte("x")); err == nil {
		t.Error("Put to a closed endpoint returned no error")
	}
}

func TestS3VirtualHostAddressing(t *testing.T) {
	var gotHost, gotPath string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHost, gotPath = r.Host, r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	listenHost, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}

	// The bucket becomes a subdomain of the endpoint, which no test DNS resolves:
	// dial the test server regardless of the host in the request line, so the
	// handler still sees the virtual-host request it has to answer.
	client := &http.Client{
		Timeout: s3RequestTimeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, listenHost.Host)
			},
		},
	}
	backend, err := newS3BackendWithClient(S3Config{
		Endpoint: server.URL, Bucket: "test-bucket", Region: "us-west-2",
		AccessKeyID: "a", SecretAccessKey: "b", PathStyle: false,
	}, client)
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	if err := backend.Put(context.Background(), "sessions/repo/id.jsonl", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(gotHost, "test-bucket.") {
		t.Errorf("host = %q, want the bucket as a subdomain", gotHost)
	}
	if gotPath != "/sessions/repo/id.jsonl" {
		t.Errorf("path = %q, want the key without the bucket", gotPath)
	}
}

func TestS3SessionTokenSigned(t *testing.T) {
	var token string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("x-amz-security-token")
		if !strings.Contains(r.Header.Get("Authorization"), "x-amz-security-token") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	backend, err := newS3Backend(S3Config{
		Endpoint: server.URL, Bucket: "test-bucket", Region: "us-east-1",
		AccessKeyID: "a", SecretAccessKey: "b", SessionToken: "session-token",
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	if err := backend.Put(context.Background(), "k.jsonl", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if token != "session-token" {
		t.Errorf("x-amz-security-token = %q", token)
	}
}

func TestS3RequestTimeoutConfigured(t *testing.T) {
	backend, err := newS3Backend(S3Config{
		Endpoint: "https://s3.example.com", Bucket: "b", Region: "us-east-1",
		AccessKeyID: "a", SecretAccessKey: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}
	if backend.client.Timeout != s3RequestTimeout {
		t.Errorf("client timeout = %v, want %v", backend.client.Timeout, s3RequestTimeout)
	}
	if backend.client.Timeout <= 0 {
		t.Error("an unbounded client timeout would hang a mirror write forever")
	}
}
