// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import "net/http"

// contentSecurityPolicy locks the viewer down to first-party resources only.
// The viewer loads no third-party scripts, styles, fonts, or frames, so a
// strict same-origin policy holds without any 'unsafe-inline' relaxation
// (the previously-inline session script now lives in static/session.js).
// This mitigates injection of active content should any user- or LLM-supplied
// value ever escape HTML escaping in a template.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"object-src 'none'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'; " +
	"form-action 'none'"

// securityHeaders wraps a handler and sets defense-in-depth response headers on
// every reply. These harden the local viewer's browser-facing surface (the
// session JSONL exposed here contains reviewed source code and the LLM's
// analysis of it). HSTS is intentionally omitted: the viewer serves plain HTTP
// on loopback, where HSTS is meaningless and would wrongly pin localhost.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}
