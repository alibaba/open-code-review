// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import "net/http"

type resolverRoundTripFunc func(*http.Request) (*http.Response, error)

func (f resolverRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
