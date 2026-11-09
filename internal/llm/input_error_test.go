// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

func TestIsNonRetryableInputError(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "http://example.invalid/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []struct {
		name      string
		makeError func(int, *http.Response) error
	}{
		{"openai", func(status int, response *http.Response) error {
			return &openai.Error{StatusCode: status, Request: request, Response: response}
		}},
		{"anthropic", func(status int, response *http.Response) error {
			return &anthropic.Error{StatusCode: status, Request: request, Response: response}
		}},
	} {
		for _, status := range []int{200, 400, 401, 403, 408, 409, 413, 429, 500, 503} {
			for _, directive := range []string{"", "true", "false"} {
				t.Run(fmt.Sprintf("%s/%d/retry=%s", provider.name, status, directive), func(t *testing.T) {
					response := &http.Response{StatusCode: status, Header: make(http.Header)}
					response.Header.Set("x-should-retry", directive)
					apiErr := provider.makeError(status, response)
					want := (status == 400 || status == 413) && directive != "true"
					for _, input := range []error{apiErr, fmt.Errorf("wrapped: %w", apiErr)} {
						if got := IsNonRetryableInputError(input); got != want {
							t.Fatalf("IsNonRetryableInputError = %v, want %v", got, want)
						}
					}
				})
			}
		}
		t.Run(provider.name+"/missing_response", func(t *testing.T) {
			if !IsNonRetryableInputError(provider.makeError(400, nil)) {
				t.Fatal("typed HTTP 400 should remain recognized without response metadata")
			}
		})
		for _, header := range []string{"Retry-After", "retry-after-ms"} {
			t.Run(provider.name+"/"+header, func(t *testing.T) {
				for _, status := range []int{400, 413} {
					response := &http.Response{StatusCode: status, Header: make(http.Header)}
					response.Header.Set(header, "10")
					if IsNonRetryableInputError(provider.makeError(status, response)) {
						t.Fatal("a server recovery hint must not be cached as permanent rejection")
					}
				}
			})
		}
	}
	var nilOpenAI *openai.Error
	var nilAnthropic *anthropic.Error
	for _, input := range []error{nil, nilOpenAI, nilAnthropic, context.Canceled, context.DeadlineExceeded,
		errors.New("400 context_length_exceeded"), errors.New("413 request too large")} {
		if IsNonRetryableInputError(input) {
			t.Fatal("unrecognized error must not be treated as a permanent input rejection")
		}
	}
}
