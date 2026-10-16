// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestNearAISettingsPersistAcrossProviderChanges(t *testing.T) {
	cfg := &Config{}
	for _, setting := range [][2]string{{"provider", "nearai"}, {"providers.nearai.e2ee", "false"}, {"providers.nearai.verification_level", "gateway"}, {"model", "canonical/model"}, {"provider", "openai"}, {"model", "gpt-test"}} {
		if err := setConfigValue(cfg, setting[0], setting[1]); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := loaded.Providers["nearai"]
	if entry.E2EE == nil || *entry.E2EE || entry.VerificationLevel != "gateway" || entry.Model != "canonical/model" {
		t.Fatalf("lost policy: %+v", entry)
	}
	cloned := cloneProviderEntry(entry)
	*cloned.E2EE = true
	if *entry.E2EE || cloned.VerificationLevel != "gateway" {
		t.Fatal("policy aliased or lost during clone")
	}
	for _, setting := range [][2]string{{"providers.nearai.e2ee", "yes"}, {"providers.nearai.verification_level", "none"}, {"providers.openai.e2ee", "false"}, {"providers.openai.verification_level", "gateway"}} {
		if err := setConfigValue(cfg, setting[0], setting[1]); err == nil {
			t.Fatalf("invalid policy accepted: %v", setting)
		}
	}
}

func TestNearAIResumeRequiresFreshVerifiedRun(t *testing.T) {
	verified := llm.NewLLMClient(llm.ResolvedEndpoint{Provider: "nearai", Protocol: "openai", URL: "https://cloud-api.near.ai/v1", Token: "test", Model: "canonical/model"}, nil, nil)
	defer llm.CloseClient(verified)
	if err := validateVerifiedResume(verified, "saved-session"); err == nil {
		t.Fatal("unverified cached results allowed")
	}
	if err := validateVerifiedResume(verified, ""); err != nil {
		t.Fatal(err)
	}
	other := llm.NewLLMClient(llm.ResolvedEndpoint{Provider: "openai"}, nil, nil)
	if err := validateVerifiedResume(other, "saved-session"); err != nil {
		t.Fatal("other provider affected", err)
	}
	if nearAIPolicyIdentity(nil) != "" {
		t.Fatal("policy appeared on unrelated provider")
	}
	if nearAIPolicyIdentity(&llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}) == nearAIPolicyIdentity(&llm.NearAISettings{VerificationLevel: "gateway"}) {
		t.Fatal("policies have the same identity")
	}
}

func TestNearAIVerificationInRunOutputs(t *testing.T) {
	for _, failed := range []bool{false, true} {
		state, status := session.StateComplete, "verified"
		if failed {
			state, status = session.StateFailed, "failed"
		}
		report := &llm.VerificationReport{Status: status, NearAISettings: llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}}
		if failed {
			report.Failed = 1
		} else {
			report.Verified = 2
		}
		identity := &jsonLLMIdentity{Provider: "nearai", Model: "canonical/model", Verification: report}
		provider := &mockResultProvider{filesReviewed: 1, manifest: mockManifest(state)}
		for _, format := range []string{"json", "text"} {
			var out bytes.Buffer
			if err := emitRunResult(context.Background(), provider, nil, time.Now(), format, "human", &quietHandle{}, identity, &out, nil); err != nil {
				t.Fatal(err)
			}
			if format == "json" {
				var result struct {
					Status string           `json:"status"`
					LLM    *jsonLLMIdentity `json:"llm"`
				}
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.LLM == nil || result.LLM.Verification == nil || result.LLM.Verification.Status != status {
					t.Fatalf("missing verification: %s", out.String())
				}
				if failed && result.Status != "failed" {
					t.Fatalf("failed run reported %q", result.Status)
				}
			} else if !strings.Contains(out.String(), "NEAR AI verification: "+status) || !strings.Contains(out.String(), "E2EE=true, policy=model-tee") {
				t.Fatalf("text: %s", out.String())
			}
		}
		var hint bytes.Buffer
		outputSessionRetryHint(&hint, "test-session", identity)
		if strings.Contains(hint.String(), "--resume") || !strings.Contains(hint.String(), "new run") {
			t.Fatalf("unsafe resume hint: %s", hint.String())
		}
	}
	var unchanged bytes.Buffer
	outputVerificationText(&unchanged, nil)
	if unchanged.Len() != 0 {
		t.Fatal("non-NEAR AI output changed")
	}
}
