// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import "github.com/alibaba/open-code-review/internal/llm"

func (sh *SessionHistory) SetVerificationReport(report *llm.VerificationReport) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.verification = report
}
