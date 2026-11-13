// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"net/url"
	"strings"
)

// normalizeEndpointURL normalizes a configured endpoint URL that carries query
// parameters, preserving those parameters on the final request.
//
// The three protocol clients (OpenAI chat completions, OpenAI Responses and
// Anthropic messages) each normalize cfg.URL by appending a protocol terminal
// suffix (/chat/completions, /responses, /v1/messages) and then stripping that
// suffix to derive the SDK base URL. That bookkeeping used plain string
// manipulation, so a URL that already carried query parameters (for example a
// gateway route like https://host/gateway?tenant=acme) had the suffix appended
// *after* the query string — https://host/gateway?tenant=acme/responses — and
// the query was then silently dropped when the SDK resolved the request path
// against the base URL (url.URL.ResolveReference does not inherit a base URL's
// query when the relative reference has none).
//
// normalizeEndpointURL parses the URL so the path and the query are handled
// independently:
//
//   - baseURL is the query-free URL with the terminal suffix already appended;
//     the client strips the suffix to derive the SDK base URL (the SDK then
//     appends its own trailing slash and request path).
//   - query holds the parsed query values for the caller to re-apply with
//     option.WithQueryAdd on each outgoing request.
//   - fullURL is the normalized URL kept on cfg.URL, with the terminal suffix
//     and the original query string preserved.
//
// When the URL has no query (or cannot be parsed as an absolute URL) it returns
// the input unchanged and a nil query, so the existing string-based
// normalization stays authoritative for the common, query-free case.
func normalizeEndpointURL(rawURL, suffix string) (baseURL string, query url.Values, fullURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.RawQuery == "" {
		return rawURL, nil, rawURL
	}
	q := u.Query()
	if len(q) == 0 {
		return rawURL, nil, rawURL
	}

	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, suffix) {
		path += suffix
	}
	u.Path = path

	originalQuery := u.RawQuery
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	base := u.String()

	u.RawQuery = originalQuery
	full := u.String()

	return base, q, full
}
