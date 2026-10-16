// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/llm"
)

func nearAIPolicyIdentity(settings *llm.NearAISettings) string {
	if settings == nil {
		return ""
	}
	return fmt.Sprintf("nearai/v1:e2ee=%t:verification=%s", settings.E2EE, settings.VerificationLevel)
}

func outputVerificationText(out io.Writer, identity *jsonLLMIdentity) {
	if identity == nil || identity.Verification == nil {
		return
	}
	r := identity.Verification
	fmt.Fprintf(out, "\nNEAR AI verification: %s (%d verified, %d failed, %d cancelled; E2EE=%t, policy=%s)\n",
		r.Status, r.Verified, r.Failed, r.Cancelled, r.E2EE, r.VerificationLevel)
}

func outputSessionRetryHint(out io.Writer, id string, identity *jsonLLMIdentity) {
	if id == "" {
		return
	}
	if identity != nil && identity.Verification != nil {
		fmt.Fprintf(out, "[ocr] Session: %s (start a new run to reverify responses)\n", id)
		return
	}
	fmt.Fprintf(out, "[ocr] Session: %s (retry with: --resume %s)\n", id, id)
}

func validateVerifiedResume(client llm.LLMClient, resume string) error {
	if resume != "" && llm.ClientVerificationReport(client) != nil {
		return fmt.Errorf("NEAR AI verified runs cannot resume saved sessions: start a new run to verify every response under the current policy")
	}
	return nil
}
