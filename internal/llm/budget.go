// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import "errors"

var ErrTokenBudget = errors.New("aggregate LLM token budget exhausted")
