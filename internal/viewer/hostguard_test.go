package viewer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostOnly(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"localhost", "localhost"},
		{"localhost:5483", "localhost"},
		{"127.0.0.1:5483", "127.0.0.1"},
		{"[::1]:5483", "::1"},
		{"[::1]", "::1"},
		{"LOCALHOST", "localhost"},
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"::1", ""}, // bare unbracketed IPv6 is ambiguous; reject
		{"a:b:c", ""},
	}
	for _, c := range cases {
		got := hostOnly(c.in)
		if got != c.want {
			t.Errorf("hostOnly(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	loopback := []string{"localhost", "127.0.0.1", "127.0.0.2", "::1"}
	for _, h := range loopback {
		if !isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = false, want true", h)
		}
	}
	notLoopback := []string{"example.com", "192.168.1.10", "10.0.0.1", "8.8.8.8", ""}
	for _, h := range notLoopback {
		if isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = true, want false", h)
		}
	}
}

func TestBuildAllowedHosts(t *testing.T) {
	// Default: loopback only
	a := buildAllowedHosts("", "")
	for _, h := range []string{"localhost", "127.0.0.1", "::1"} {
		if _, ok := a[h]; !ok {
			t.Errorf("default allowlist missing %q", h)
		}
	}
	if len(a) != 3 {
		t.Errorf("default allowlist size = %d, want 3, got %v", len(a), a)
	}

	// Concrete bind host is auto-added
	a = buildAllowedHosts("192.168.1.10", "")
	if _, ok := a["192.168.1.10"]; !ok {
		t.Errorf("concrete bind host not added: %v", a)
	}

	// Wildcard bind host is NOT auto-added (forces operator to set env var)
	for _, bh := range []string{"0.0.0.0", "::", ""} {
		a = buildAllowedHosts(bh, "")
		if len(a) != 3 {
			t.Errorf("wildcard bind %q: allowlist size = %d, want 3 (loopback only)", bh, len(a))
		}
	}

	// Env extension parsed correctly
	a = buildAllowedHosts("", "foo.example,bar.example , BAZ.example")
	for _, h := range []string{"foo.example", "bar.example", "baz.example"} {
		if _, ok := a[h]; !ok {
			t.Errorf("env-allowlisted host %q missing: %v", h, a)
		}
	}
}

func TestSplitBindHost(t *testing.T) {
	cases := map[string]string{
		"":                "",
		":5483":           "",
		"localhost:5483":  "localhost",
		"127.0.0.1:5483":  "127.0.0.1",
		"[::1]:5483":      "::1",
		"0.0.0.0:5483":    "0.0.0.0",
		"192.168.1.10":    "192.168.1.10",
	}
	for in, want := range cases {
		if got := splitBindHost(in); got != want {
			t.Errorf("splitBindHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostGuard(t *testing.T) {
	body := []byte("session data leak")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	})

	cases := []struct {
		name      string
		bindAddr  string
		envVal    string
		hostHdr   string
		wantCode  int
		wantLeak  bool
	}{
		// Loopback always allowed
		{"loopback-localhost", "localhost:5483", "", "localhost:5483", 200, true},
		{"loopback-127", "localhost:5483", "", "127.0.0.1:5483", 200, true},
		{"loopback-127-anyport", "localhost:5483", "", "127.0.0.1:1234", 200, true},
		{"loopback-ipv6", "localhost:5483", "", "[::1]:5483", 200, true},

		// Default bind => attacker host rejected (DNS rebinding case)
		{"rebind-attacker", "localhost:5483", "", "attacker.example", 403, false},
		{"rebind-with-port", "localhost:5483", "", "evil.com:5483", 403, false},
		{"link-local-metadata", "localhost:5483", "", "169.254.169.254", 403, false},
		{"public-ip-rebind", "localhost:5483", "", "8.8.8.8:5483", 403, false},

		// Missing / empty Host header rejected
		{"empty-host", "localhost:5483", "", "", 403, false},

		// Concrete LAN bind: bind host is allowed, other names still rejected
		{"lan-bind-self", "192.168.1.10:5483", "", "192.168.1.10:5483", 200, true},
		{"lan-bind-attacker-rejected", "192.168.1.10:5483", "", "attacker.example", 403, false},

		// Wildcard bind: only loopback allowed without env override
		{"wildcard-bind-loopback-ok", "0.0.0.0:5483", "", "127.0.0.1:5483", 200, true},
		{"wildcard-bind-attacker-rejected", "0.0.0.0:5483", "", "evil.example", 403, false},

		// Env extension allows additional hosts
		{"env-allowed", "0.0.0.0:5483", "ocr.lan,review.internal", "review.internal:5483", 200, true},
		{"env-not-listed-rejected", "0.0.0.0:5483", "ocr.lan", "review.internal:5483", 403, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allowed := buildAllowedHosts(splitBindHost(c.bindAddr), c.envVal)
			h := hostGuard(allowed, inner)

			req := httptest.NewRequest("GET", "http://test/", nil)
			req.Host = c.hostHdr
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != c.wantCode {
				t.Errorf("status = %d, want %d", rr.Code, c.wantCode)
			}
			leaked := rr.Body.String() == string(body)
			if leaked != c.wantLeak {
				t.Errorf("leaked = %v, want %v (body=%q)", leaked, c.wantLeak, rr.Body.String())
			}
		})
	}
}
