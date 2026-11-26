// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"net/url"
	"testing"
)

func TestNormalizeEndpointURL(t *testing.T) {
	tests := []struct {
		name        string
		rawURL      string
		suffix      string
		wantBase    string
		wantFull    string
		wantQuery   url.Values
		wantQueryOK bool
	}{
		{
			name:        "gateway query is split from the path",
			rawURL:      "https://host/gateway?tenant=acme",
			suffix:      "/responses",
			wantBase:    "https://host/gateway/responses",
			wantFull:    "https://host/gateway/responses?tenant=acme",
			wantQuery:   url.Values{"tenant": {"acme"}},
			wantQueryOK: true,
		},
		{
			name:        "suffix already present with query",
			rawURL:      "https://host/v1/responses?tenant=acme&api-version=2024-01",
			suffix:      "/responses",
			wantBase:    "https://host/v1/responses",
			wantFull:    "https://host/v1/responses?tenant=acme&api-version=2024-01",
			wantQuery:   url.Values{"tenant": {"acme"}, "api-version": {"2024-01"}},
			wantQueryOK: true,
		},
		{
			name:        "repeated query keys are preserved",
			rawURL:      "https://host/v1?tag=a&tag=b",
			suffix:      "/responses",
			wantBase:    "https://host/v1/responses",
			wantFull:    "https://host/v1/responses?tag=a&tag=b",
			wantQuery:   url.Values{"tag": {"a", "b"}},
			wantQueryOK: true,
		},
		{
			name:        "trailing slash before query",
			rawURL:      "https://host/gateway/?tenant=acme",
			suffix:      "/responses",
			wantBase:    "https://host/gateway/responses",
			wantFull:    "https://host/gateway/responses?tenant=acme",
			wantQuery:   url.Values{"tenant": {"acme"}},
			wantQueryOK: true,
		},
		{
			name:        "no query falls through unchanged",
			rawURL:      "https://host/v1",
			suffix:      "/responses",
			wantBase:    "https://host/v1",
			wantFull:    "https://host/v1",
			wantQueryOK: false,
		},
		{
			name:        "empty query falls through unchanged",
			rawURL:      "https://host/v1?",
			suffix:      "/responses",
			wantBase:    "https://host/v1?",
			wantFull:    "https://host/v1?",
			wantQueryOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, query, full := normalizeEndpointURL(tt.rawURL, tt.suffix)
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if full != tt.wantFull {
				t.Errorf("full = %q, want %q", full, tt.wantFull)
			}
			if tt.wantQueryOK {
				if !equalQuery(query, tt.wantQuery) {
					t.Errorf("query = %v, want %v", query, tt.wantQuery)
				}
			} else if query != nil {
				t.Errorf("query = %v, want nil", query)
			}
		})
	}
}

func equalQuery(a, b url.Values) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
	}
	return true
}
