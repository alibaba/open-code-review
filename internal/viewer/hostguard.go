package viewer

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// EnvAllowedHosts is the environment variable users can set to extend the
// default loopback allowlist with additional hostnames (comma-separated).
const EnvAllowedHosts = "OCR_VIEWER_ALLOWED_HOSTS"

// hostOnly returns the bare host portion of a Host header value, with any
// port stripped and surrounding brackets removed from IPv6 literals. Empty
// or malformed input returns "".
func hostOnly(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	// SplitHostPort handles bracketed IPv6 ([::1]:5483) and host:port forms.
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	// No port. Strip brackets from a bare bracketed IPv6 literal like "[::1]".
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		return strings.ToLower(h[1 : len(h)-1])
	}
	// Reject a bare unbracketed IPv6 with embedded colons (ambiguous with port).
	if strings.Count(h, ":") > 1 {
		return ""
	}
	return strings.ToLower(h)
}

// isLoopbackHost reports whether host is a loopback name or IP literal that
// is always safe to accept regardless of operator configuration.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "0:0:0:0:0:0:0:1":
		return true
	}
	// Any 127.0.0.0/8 IPv4 literal.
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// buildAllowedHosts returns the default-deny allowlist used by the host guard.
// The set always includes the standard loopback names. If bindHost is a
// concrete (non-wildcard) hostname or IP, it is also included so a user who
// runs `ocr viewer --addr 192.168.1.10:5483` can still reach the UI at that
// address. Wildcard binds (empty, 0.0.0.0, ::) are NOT auto-added — operators
// who bind on a public interface must explicitly set OCR_VIEWER_ALLOWED_HOSTS,
// which forces them to acknowledge the exposure.
func buildAllowedHosts(bindHost string, envVal string) map[string]struct{} {
	allowed := map[string]struct{}{
		"localhost": {},
		"127.0.0.1": {},
		"::1":       {},
	}
	bh := strings.ToLower(strings.TrimSpace(bindHost))
	if bh != "" && bh != "0.0.0.0" && bh != "::" && bh != "*" {
		// Strip brackets from a bracketed IPv6 literal if present.
		if strings.HasPrefix(bh, "[") && strings.HasSuffix(bh, "]") {
			bh = bh[1 : len(bh)-1]
		}
		allowed[bh] = struct{}{}
	}
	for _, h := range strings.Split(envVal, ",") {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			allowed[h] = struct{}{}
		}
	}
	return allowed
}

// hostGuard returns a middleware that rejects requests whose Host header is
// not on the allowlist. This blocks DNS-rebinding attacks against the local
// viewer: an attacker page that resolves its own domain to 127.0.0.1 still
// sends the attacker's domain in the Host header, which fails this check.
func hostGuard(allowed map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := hostOnly(r.Host)
		if host == "" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if isLoopbackHost(host) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := allowed[host]; ok {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "forbidden host", http.StatusForbidden)
	})
}

// splitBindHost returns the host portion of a listen address as passed to
// http.Server.Addr. Empty input returns "".
func splitBindHost(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// resolveAllowedHostsFromEnv reads the OCR_VIEWER_ALLOWED_HOSTS environment
// variable and combines it with the bind host to produce the active allowlist.
func resolveAllowedHostsFromEnv(bindAddr string) map[string]struct{} {
	return buildAllowedHosts(splitBindHost(bindAddr), os.Getenv(EnvAllowedHosts))
}
