package viewer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersSetsAllHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	securityHeaders(inner).ServeHTTP(rec, req)

	want := map[string]string{
		"Content-Security-Policy": contentSecurityPolicy,
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Permissions-Policy":      "geolocation=(), camera=(), microphone=()",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %q = %q, want %q", k, got, v)
		}
	}
}

// The viewer serves plain HTTP on loopback; HSTS would wrongly pin localhost.
func TestSecurityHeadersOmitsHSTS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	securityHeaders(inner).ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should not be set on the loopback viewer, got %q", got)
	}
}

// The CSP must stay strict: no 'unsafe-inline'/'unsafe-eval' relaxation, since
// the formerly-inline session script now lives in static/session.js.
func TestContentSecurityPolicyIsStrict(t *testing.T) {
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "*"} {
		if strings.Contains(contentSecurityPolicy, bad) {
			t.Errorf("CSP unexpectedly contains %q: %s", bad, contentSecurityPolicy)
		}
	}
}
