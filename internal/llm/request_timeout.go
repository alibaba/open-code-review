// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
)

var ErrRequestTimeout = errors.New("LLM request timeout (configure OCR_LLM_TIMEOUT or provider timeout_sec)")

func describeTimeout(ctx context.Context, err error) error {
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("caller deadline exceeded: %w", err)
	}
	return fmt.Errorf("%w: %w", ErrRequestTimeout, err)
}
