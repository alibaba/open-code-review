// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func gatedRequest(ctx context.Context) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")).WithContext(ctx)
}

// fakeBody is a ReadCloser whose reads drain a fixed payload and whose Close
// is counted.
type fakeBody struct {
	payload string
	closed  int
}

func (b *fakeBody) Read(p []byte) (int, error) {
	if len(b.payload) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.payload)
	b.payload = b.payload[n:]
	return n, nil
}

func (b *fakeBody) Close() error {
	b.closed++
	return nil
}

func TestAdmissionMiddleware_PassthroughWithoutGateOrScope(t *testing.T) {
	called := false
	next := func(r *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}

	// No gate at all (disabled config).
	if _, err := newAdmissionMiddleware(nil)(gatedRequest(context.Background()), next); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("next not called without gate")
	}

	// Gate present but the request carries no review scope (scan / llm test).
	called = false
	gate := newAdmissionGate(1)
	if _, err := newAdmissionMiddleware(gate)(gatedRequest(context.Background()), next); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("next not called without admission scope")
	}
	if gate.inFlight() != 0 {
		t.Fatalf("inFlight = %d, want 0 for ungated request", gate.inFlight())
	}

	// Scope present but gate disabled.
	called = false
	if _, err := newAdmissionMiddleware(nil)(gatedRequest(ContextWithAdmissionScope(context.Background())), next); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("next not called with nil gate despite scope")
	}
}

func TestAdmissionMiddleware_CancelledWhileQueued(t *testing.T) {
	gate := newAdmissionGate(1)
	if err := gate.acquire(context.Background()); err != nil {
		t.Fatal(err) // occupy the only permit
	}
	t.Cleanup(gate.release)

	ctx, cancel := context.WithCancel(ContextWithAdmissionScope(context.Background()))
	nextCalled := false
	next := func(r *http.Request) (*http.Response, error) {
		nextCalled = true
		return nil, errors.New("must not happen")
	}

	cancel()
	res, err := newAdmissionMiddleware(gate)(gatedRequest(ctx), next)
	if res != nil {
		t.Fatalf("response = %v, want nil", res)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if nextCalled {
		t.Error("transport called despite cancelled admission")
	}
	if gate.inFlight() != 1 {
		t.Fatalf("inFlight = %d, want 1 (the pre-held permit only, no waiter residue)", gate.inFlight())
	}
}

func TestAdmissionMiddleware_ReleaseOnFailure(t *testing.T) {
	gate := newAdmissionGate(1)
	ctx := ContextWithAdmissionScope(context.Background())

	// Transport error.
	_, err := newAdmissionMiddleware(gate)(gatedRequest(ctx), func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if gate.inFlight() != 0 {
		t.Fatalf("transport error leaked a permit: %d", gate.inFlight())
	}

	// Nil response.
	_, _ = newAdmissionMiddleware(gate)(gatedRequest(ctx), func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	if gate.inFlight() != 0 {
		t.Fatalf("nil response leaked a permit: %d", gate.inFlight())
	}

	// Body-less response.
	_, _ = newAdmissionMiddleware(gate)(gatedRequest(ctx), func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})
	if gate.inFlight() != 0 {
		t.Fatalf("body-less response leaked a permit: %d", gate.inFlight())
	}
}

func TestAdmissionMiddleware_BodyReleaseExactlyOnce(t *testing.T) {
	cases := []struct {
		name string
		drv  func(t *testing.T, body io.ReadCloser, gate *admissionGate)
	}{
		{"EOF read", func(t *testing.T, body io.ReadCloser, gate *admissionGate) {
			if _, err := body.Read(make([]byte, 128)); err != io.EOF {
				t.Fatalf("read err = %v, want EOF", err)
			}
			if gate.inFlight() != 0 {
				t.Fatalf("EOF did not release: %d", gate.inFlight())
			}
		}},
		{"Close", func(t *testing.T, body io.ReadCloser, gate *admissionGate) {
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			if gate.inFlight() != 0 {
				t.Fatalf("Close did not release: %d", gate.inFlight())
			}
		}},
		{"EOF then Close", func(t *testing.T, body io.ReadCloser, gate *admissionGate) {
			body.Read(make([]byte, 128)) // EOF
			body.Close()
			if gate.inFlight() != 0 {
				t.Fatalf("EOF+Close released more than state allows: %d", gate.inFlight())
			}
		}},
		{"Close twice", func(t *testing.T, body io.ReadCloser, gate *admissionGate) {
			body.Close()
			body.Close()
			if gate.inFlight() != 0 {
				t.Fatalf("double Close over-released: %d (would panic on channel underflow later)", gate.inFlight())
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := newAdmissionGate(2)
			ctx := ContextWithAdmissionScope(context.Background())
			fb := &fakeBody{payload: ""}
			res, err := newAdmissionMiddleware(gate)(gatedRequest(ctx), func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: fb}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if gate.inFlight() != 1 {
				t.Fatalf("held permit after response: %d, want 1", gate.inFlight())
			}
			tc.drv(t, res.Body, gate)
		})
	}
}

func TestAdmissionGate_LimitEnforcement(t *testing.T) {
	gate := newAdmissionGate(1)
	ctx := ContextWithAdmissionScope(context.Background())
	next := func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &fakeBody{}}, nil
	}

	// First request holds the permit via its open body.
	res1, err := newAdmissionMiddleware(gate)(gatedRequest(ctx), next)
	if err != nil {
		t.Fatal(err)
	}
	if gate.inFlight() != 1 {
		t.Fatalf("inFlight = %d, want 1", gate.inFlight())
	}

	// The second blocks until the first releases.
	done := make(chan error, 1)
	go func() {
		res2, err := newAdmissionMiddleware(gate)(gatedRequest(ctx), next)
		if err == nil && res2 != nil {
			res2.Body.Close()
		}
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("second admission acquired while the permit was held")
	case <-time.After(50 * time.Millisecond):
	}

	res1.Body.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second admission never acquired after release")
	}
	if gate.inFlight() != 0 {
		t.Fatalf("inFlight = %d, want 0 at drain", gate.inFlight())
	}
}

func TestAdmissionScope_SurvivesWithoutCancel(t *testing.T) {
	inner := ContextWithAdmissionScope(context.Background())
	outer := context.WithoutCancel(inner)
	if !hasAdmissionScope(outer) {
		t.Fatal("admission scope lost across context.WithoutCancel")
	}
	plain := context.Background()
	if hasAdmissionScope(plain) {
		t.Fatal("plain context carries admission scope")
	}
}

func TestAdmissionMiddleware_PanicInTransportReleasesPermit(t *testing.T) {
	gate := newAdmissionGate(1)
	ctx := ContextWithAdmissionScope(context.Background())

	next := func(r *http.Request) (*http.Response, error) {
		panic("transport exploded")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("middleware swallowed the panic")
			}
		}()
		_, _ = newAdmissionMiddleware(gate)(gatedRequest(ctx), next)
	}()

	if gate.inFlight() != 0 {
		t.Fatalf("panic leaked a permit: %d in flight", gate.inFlight())
	}
}

func TestAdmissionMiddleware_ReadErrorReleasesPermit(t *testing.T) {
	gate := newAdmissionGate(1)
	ctx := ContextWithAdmissionScope(context.Background())

	res, err := newAdmissionMiddleware(gate)(gatedRequest(ctx), func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &errBody{}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gate.inFlight() != 1 {
		t.Fatalf("inFlight = %d, want 1 while body open", gate.inFlight())
	}
	if _, rerr := res.Body.Read(make([]byte, 8)); rerr == nil {
		t.Fatal("expected read error")
	}
	if gate.inFlight() != 0 {
		t.Fatalf("read error did not release: %d in flight", gate.inFlight())
	}
	res.Body.Close() // once-guarded; must not over-release
	if gate.inFlight() != 0 {
		t.Fatalf("Close after read error over-released: %d", gate.inFlight())
	}
}

type errBody struct{}

func (b *errBody) Read(p []byte) (int, error) { return 0, errors.New("connection reset") }
func (b *errBody) Close() error               { return nil }
