// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"

	"github.com/alibaba/open-code-review/internal/llm"
)

type commentFallbackPathKey struct{}

// RunMainTaskWithFallbackPath gives concurrent conversations distinct session,
// cache-affinity, tool-failure and worker keys without changing omitted comment paths.
func (r *Runner) RunMainTaskWithFallbackPath(ctx context.Context, messages []llm.Message, taskKey, fallbackPath string) (bool, MainLoopStop, error) {
	ctx = context.WithValue(ctx, commentFallbackPathKey{}, fallbackPath)
	return r.RunMainTask(ctx, messages, taskKey)
}
