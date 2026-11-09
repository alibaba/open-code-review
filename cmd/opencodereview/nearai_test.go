// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestNearAIProviderNameCasePreservesVerification(t *testing.T) {
	t.Setenv("OCR_LLM_TIMEOUT", "")
	t.Setenv("OCR_LLM_EXTRA_HEADERS", "")
	for _, provider := range []string{"nearai", "NEARAI", "NearAI"} {
		t.Run(provider, func(t *testing.T) {
			for _, policy := range []struct {
				name     string
				explicit bool
				want     llm.NearAISettings
			}{
				{name: "default", want: llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}},
				{name: "gateway", explicit: true, want: llm.NearAISettings{E2EE: false, VerificationLevel: "gateway"}},
			} {
				t.Run(policy.name, func(t *testing.T) {
					cfg := &Config{}
					settings := [][2]string{
						{"provider", provider},
						{"model", "canonical/model"},
						{"providers." + provider + ".api_key", "test"},
					}
					if policy.explicit {
						settings = append(settings,
							[2]string{"providers." + provider + ".e2ee", "false"},
							[2]string{"providers." + provider + ".verification_level", "gateway"},
						)
					}
					for _, setting := range settings {
						if err := setConfigValue(cfg, setting[0], setting[1]); err != nil {
							t.Fatalf("set %s: %v", setting[0], err)
						}
					}
					path := filepath.Join(t.TempDir(), "config.json")
					if err := saveConfig(path, cfg); err != nil {
						t.Fatal(err)
					}
					ep, err := llm.ResolveEndpointWithOptions(path, llm.ResolveOptions{Provider: provider})
					if err != nil {
						t.Fatal(err)
					}
					if ep.NearAI == nil || *ep.NearAI != policy.want {
						t.Fatalf("resolved policy: %+v, want %+v", ep.NearAI, policy.want)
					}
					client := llm.NewLLMClient(ep, nil, nil)
					defer llm.CloseClient(client)
					report := llm.ClientVerificationReport(client)
					if report == nil || report.NearAISettings != policy.want {
						t.Fatalf("client %T verification policy: %+v, want %+v", client, report, policy.want)
					}
				})
			}
		})
	}
}

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
	for _, report := range []*llm.VerificationReport{
		{Status: "verified", Verified: 2},
		{Status: "failed", Failed: 1},
		{Status: "request_failed", RequestFailed: 1, Verified: 1},
	} {
		t.Run(report.Status, func(t *testing.T) {
			state := session.StateComplete
			if report.Failed > 0 {
				state = session.StateFailed
			}
			report.NearAISettings = llm.NearAISettings{E2EE: true, VerificationLevel: "model-tee"}
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
					if result.LLM == nil || result.LLM.Verification == nil || result.LLM.Verification.Status != report.Status || result.LLM.Verification.RequestFailed != report.RequestFailed {
						t.Fatalf("missing verification: %s", out.String())
					}
					if (result.Status == "failed") != (report.Failed > 0) {
						t.Fatalf("run reported %q for %+v", result.Status, report)
					}
				} else if !strings.Contains(out.String(), "NEAR AI verification: "+report.Status) || !strings.Contains(out.String(), "E2EE=true, policy=model-tee") || !strings.Contains(out.String(), fmt.Sprintf("%d request failed", report.RequestFailed)) {
					t.Fatalf("text: %s", out.String())
				}
			}
			var hint bytes.Buffer
			outputSessionRetryHint(&hint, "test-session", identity)
			if strings.Contains(hint.String(), "--resume") || !strings.Contains(hint.String(), "new run") {
				t.Fatalf("unsafe resume hint: %s", hint.String())
			}
		})
	}
	var unchanged bytes.Buffer
	outputVerificationText(&unchanged, nil)
	if unchanged.Len() != 0 {
		t.Fatal("non-NEAR AI output changed")
	}
}
