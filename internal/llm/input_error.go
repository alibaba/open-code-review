// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// IsNonRetryableInputError identifies HTTP 400/413 input rejections without a
// provider recovery directive. Unrecognized errors are not classified by their
// message text.
func IsNonRetryableInputError(err error) bool {
	var openaiErr *openai.Error
	if errors.As(err, &openaiErr) && openaiErr != nil {
		return nonRetryableInputStatus(openaiErr.StatusCode, openaiErr.Response)
	}
	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) && anthropicErr != nil {
		return nonRetryableInputStatus(anthropicErr.StatusCode, anthropicErr.Response)
	}
	return false
}

func nonRetryableInputStatus(status int, response *http.Response) bool {
	if status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
		return false
	}
	return response == nil || (response.Header.Get("x-should-retry") != "true" &&
		response.Header.Get("Retry-After") == "" && response.Header.Get("retry-after-ms") == "")
}
