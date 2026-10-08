// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestVertexProtocolIsRecognized guards the three-part contract documented in
// protocol.go: a new protocol needs a constant, a NormalizeProtocol case, and a
// ValidateProtocol entry. Missing the last one turns a valid config into
// "unsupported protocol".
func TestVertexProtocolIsRecognized(t *testing.T) {
	for _, raw := range []string{"anthropic-vertex", "ANTHROPIC-VERTEX", "  Anthropic-Vertex  "} {
		if got := NormalizeProtocol(raw); got != ProtocolAnthropicVertex {
			t.Errorf("NormalizeProtocol(%q) = %q, want %q", raw, got, ProtocolAnthropicVertex)
		}
	}
	if err := ValidateProtocol(ProtocolAnthropicVertex); err != nil {
		t.Errorf("ValidateProtocol(%q) = %v, want nil", ProtocolAnthropicVertex, err)
	}
}

// TestVertexProviderIsRegistered pins the preset's shape. An api_key or a
// BaseURL here would be wrong: credentials come from Application Default
// Credentials and the host is derived from the region.
func TestVertexProviderIsRegistered(t *testing.T) {
	p, ok := LookupProvider("vertex")
	if !ok {
		t.Fatal("LookupProvider(\"vertex\") not found")
	}
	if p.Protocol != ProtocolAnthropicVertex {
		t.Errorf("Protocol = %q, want %q", p.Protocol, ProtocolAnthropicVertex)
	}
	if !p.AmbientAuth {
		t.Error("AmbientAuth = false, want true — vertex authorizes from Application Default Credentials and has no api_key")
	}
	if p.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty — the region determines the Vertex AI host", p.BaseURL)
	}
	if p.EnvVar != "" {
		t.Errorf("EnvVar = %q, want empty — there is no API key env var to fall back to", p.EnvVar)
	}
}

// TestResolveVertexWithoutAPIKey is the vertex counterpart to
// TestResolveBedrockWithoutAPIKey: the api_key requirement in
// tryProviderConfig, and the URL-and-Token completeness check in
// ResolveEndpointWithModelOverride, must both treat an ambient-auth provider
// as complete without either.
func TestResolveVertexWithoutAPIKey(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider":  "vertex",
		"model":     "claude-sonnet-5",
		"providers": map[string]any{"vertex": map[string]any{"gcp_project": "my-project", "gcp_region": "us-east5"}},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Protocol != ProtocolAnthropicVertex {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolAnthropicVertex)
	}
	if !ep.AmbientAuth {
		t.Error("AmbientAuth = false, want true")
	}
	if ep.Token != "" {
		t.Errorf("Token = %q, want empty", ep.Token)
	}
	if ep.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5", ep.Model)
	}
}

// TestVertexDoesNotRunAPIKeyCmd mirrors TestBedrockDoesNotRunAPIKeyCmd: an
// ambient-auth provider never executes api_key_cmd. A signed/authorized
// request has no use for the output, and the command is typically a
// secret-manager read — running it means a real 1Password / Touch ID prompt
// for a value that is immediately discarded.
func TestVertexDoesNotRunAPIKeyCmd(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "ran")
	path := writeConfig(t, map[string]any{
		"provider": "vertex",
		"model":    "claude-sonnet-5",
		"providers": map[string]any{
			"vertex": map[string]any{"api_key_cmd": "touch '" + sentinel + "'; echo sk-should-never-be-used"},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("api_key_cmd ran for an ambient-auth provider")
	}
	if ep.Token != "" {
		t.Errorf("Token = %q, want empty", ep.Token)
	}
}

// TestResolveVertexPassesGCPSettings covers gcp_project / gcp_region reaching
// the client, so a review run is reproducible without exporting
// GCLOUD_PROJECT first.
func TestResolveVertexPassesGCPSettings(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider": "vertex",
		"model":    "claude-sonnet-5",
		"providers": map[string]any{
			"vertex": map[string]any{"gcp_project": "example-project", "gcp_region": "europe-west1"},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.GCPProject != "example-project" {
		t.Errorf("GCPProject = %q, want example-project", ep.GCPProject)
	}
	if ep.GCPRegion != "europe-west1" {
		t.Errorf("GCPRegion = %q, want europe-west1", ep.GCPRegion)
	}

	cfg := ClientConfig{}
	if c, ok := NewLLMClient(ep, nil, nil).(*AnthropicClient); ok {
		cfg = c.cfg
	} else {
		t.Fatal("NewLLMClient did not return *AnthropicClient for the vertex protocol")
	}
	if cfg.GCPProject != "example-project" || cfg.GCPRegion != "europe-west1" {
		t.Errorf("ClientConfig GCP settings = %q/%q, want example-project/europe-west1", cfg.GCPProject, cfg.GCPRegion)
	}
}

// TestVertexAmbientAuthFollowsTheEffectiveProtocol is the vertex counterpart
// to TestAmbientAuthFollowsTheEffectiveProtocol: an entry may override a
// preset's protocol, so reading ambient auth off the preset alone would let
// `protocol: openai` on the vertex preset resolve with no token and no URL.
func TestVertexAmbientAuthFollowsTheEffectiveProtocol(t *testing.T) {
	t.Run("vertex preset overridden to a token protocol needs a key again", func(t *testing.T) {
		path := writeConfig(t, map[string]any{
			"provider": "vertex",
			"model":    "gpt-5.4",
			"providers": map[string]any{
				"vertex": map[string]any{"protocol": "openai", "url": "https://example.invalid/v1"},
			},
		})
		if _, err := ResolveEndpoint(path); err == nil {
			t.Error("resolved with no api_key after the protocol was overridden away from vertex; want an error")
		}
	})

	t.Run("entry that selects the vertex protocol authorizes without a key", func(t *testing.T) {
		path := writeConfig(t, map[string]any{
			"provider": "anthropic",
			"model":    "claude-sonnet-5",
			"providers": map[string]any{
				"anthropic": map[string]any{"protocol": ProtocolAnthropicVertex, "gcp_project": "p", "gcp_region": "us-east5"},
			},
		})
		t.Setenv("ANTHROPIC_API_KEY", "")
		ep, err := ResolveEndpoint(path)
		if err != nil {
			t.Fatalf("ResolveEndpoint: %v", err)
		}
		if !ep.AmbientAuth {
			t.Error("AmbientAuth = false for an entry whose protocol is anthropic-vertex")
		}
	})
}

// TestVertexIsRejectedOnTheURLAndTokenPaths covers the two strategies that
// describe a single HTTP endpoint. Both validate anthropic-vertex as a
// protocol name and then have nowhere to put a region or a project, so both
// reject it outright rather than silently ignoring gcp_region/gcp_project.
func TestVertexIsRejectedOnTheURLAndTokenPaths(t *testing.T) {
	t.Run("OCR_LLM_PROTOCOL", func(t *testing.T) {
		t.Setenv("OCR_LLM_URL", "https://example.invalid/v1")
		t.Setenv("OCR_LLM_TOKEN", "sk-test")
		t.Setenv("OCR_LLM_MODEL", "claude-sonnet-5")
		t.Setenv("OCR_LLM_PROTOCOL", ProtocolAnthropicVertex)

		_, err := ResolveEndpoint(writeConfig(t, map[string]any{}))
		if err == nil {
			t.Fatal("resolved with OCR_LLM_PROTOCOL=anthropic-vertex; want an error naming the variable")
		}
		for _, want := range []string{"OCR_LLM_PROTOCOL", "gcp_region", `"provider": "vertex"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})

	t.Run("llm.protocol", func(t *testing.T) {
		for _, k := range []string{"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_PROTOCOL"} {
			t.Setenv(k, "")
		}
		path := writeConfig(t, map[string]any{
			"llm": map[string]any{
				"url":        "https://example.invalid/v1",
				"auth_token": "sk-test",
				"model":      "claude-sonnet-5",
				"protocol":   ProtocolAnthropicVertex,
			},
		})

		_, err := ResolveEndpoint(path)
		if err == nil {
			t.Fatal("resolved with llm.protocol=anthropic-vertex; want an error naming the key")
		}
		if !strings.Contains(err.Error(), "llm.protocol") {
			t.Errorf("error %q does not mention llm.protocol", err)
		}
	})
}

// TestCustomProviderCanSelectVertex is the other half of that contract: where
// a provider entry exists there is somewhere to put gcp_region and
// gcp_project, so vertex is configurable — and the url every other protocol
// requires is not demanded, because the region decides the host and the
// client never reads it.
func TestCustomProviderCanSelectVertex(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider": "mine",
		"model":    "claude-sonnet-5",
		"custom_providers": map[string]any{
			"mine": map[string]any{
				"protocol":    ProtocolAnthropicVertex,
				"gcp_region":  "us-east5",
				"gcp_project": "example-project",
			},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if !ep.AmbientAuth {
		t.Error("AmbientAuth = false for a custom provider on the vertex protocol")
	}
	if ep.GCPRegion != "us-east5" || ep.GCPProject != "example-project" {
		t.Errorf("GCPRegion/GCPProject = %q/%q, want us-east5/example-project", ep.GCPRegion, ep.GCPProject)
	}
	if ep.Token != "" {
		t.Errorf("Token = %q, want empty", ep.Token)
	}
}

// TestVertexModelOverrideIsNotGatedByThePresetList mirrors
// TestBedrockModelOverrideIsNotGatedByThePresetList: an ambient-auth
// provider's preset Models list is a picker for `ocr config model`, not an
// allowlist for --model.
func TestVertexModelOverrideIsNotGatedByThePresetList(t *testing.T) {
	path := writeConfig(t, map[string]any{
		"provider":  "vertex",
		"model":     "claude-sonnet-5",
		"providers": map[string]any{"vertex": map[string]any{"gcp_project": "p", "gcp_region": "us-east5"}},
	})

	model := "claude-sonnet-5@20260101" // a versioned Model Garden ID the preset list does not carry
	ep, err := ResolveEndpointWithModelOverride(path, model)
	if err != nil {
		t.Fatalf("ResolveEndpointWithModelOverride(%q) = %v, want it accepted", model, err)
	}
	if ep.Model != model {
		t.Errorf("resolved model = %q, want %q", ep.Model, model)
	}
}

// TestVertexContextReportsResolvedRegion covers what `ocr llm test` prints:
// vertex has no configured URL, so the resolved region and project are the
// only way to see where a request went.
func TestVertexContextReportsResolvedRegion(t *testing.T) {
	client := &AnthropicClient{vertex: true, gcpRegion: "us-east5", gcpProject: "example-project"}
	region, project, ok := client.VertexContext()
	if !ok {
		t.Fatal("ok = false for a vertex client")
	}
	if region != "us-east5" || project != "example-project" {
		t.Errorf("VertexContext() = %q/%q, want us-east5/example-project", region, project)
	}

	if _, _, ok := (&AnthropicClient{}).VertexContext(); ok {
		t.Error("ok = true for a non-vertex client")
	}
}

// TestVertexClientRequiresRegion guards NewAnthropicVertexClient's fail-closed
// check: the SDK panics on an empty region rather than returning an error, so
// the region must be validated before the SDK call is ever reached.
func TestVertexClientRequiresRegion(t *testing.T) {
	t.Setenv("VERTEX_LOCATION", "")
	client := NewAnthropicVertexClient(ClientConfig{Model: "claude-sonnet-5", GCPProject: "p"})
	if client == nil {
		t.Fatal("NewAnthropicVertexClient returned nil; it must always return a client so the error can surface per-request")
	}
	if client.initErr == nil {
		t.Fatal("initErr = nil for a vertex client with no region configured")
	}
	if !strings.Contains(client.initErr.Error(), "gcp_region") {
		t.Errorf("initErr %q does not mention gcp_region", client.initErr)
	}
	if _, err := client.CompletionsWithCtx(t.Context(), ChatRequest{}); err == nil {
		t.Error("CompletionsWithCtx returned nil error despite a construction failure")
	}
}

// TestVertexClientRequiresProject covers the other half of the fail-closed
// check: unlike Bedrock's region, a Vertex project has no equivalent ambient
// default worth trusting silently, so an unresolved project is also an
// initErr rather than a request that reaches Google with an empty path
// segment.
func TestVertexClientRequiresProject(t *testing.T) {
	t.Setenv("GCLOUD_PROJECT", "")
	client := NewAnthropicVertexClient(ClientConfig{Model: "claude-sonnet-5", GCPRegion: "us-east5"})
	if client == nil {
		t.Fatal("NewAnthropicVertexClient returned nil; it must always return a client so the error can surface per-request")
	}
	if client.initErr == nil {
		t.Fatal("initErr = nil for a vertex client with no project configured")
	}
	if !strings.Contains(client.initErr.Error(), "gcp_project") {
		t.Errorf("initErr %q does not mention gcp_project", client.initErr)
	}
}

// TestVertexClientEnvFallbacks covers GCLOUD_PROJECT and VERTEX_LOCATION filling
// in what the config leaves empty, and the config winning when both are set.
func TestVertexClientEnvFallbacks(t *testing.T) {
	t.Setenv("GCLOUD_PROJECT", "env-project")
	t.Setenv("VERTEX_LOCATION", "env-region")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "nonexistent-creds.json"))

	client := NewAnthropicVertexClient(ClientConfig{Model: "claude-sonnet-5"})
	if client.gcpProject != "env-project" || client.gcpRegion != "env-region" {
		t.Errorf("env fallback = %q/%q, want env-region/env-project", client.gcpRegion, client.gcpProject)
	}

	client = NewAnthropicVertexClient(ClientConfig{Model: "claude-sonnet-5", GCPRegion: "cfg-region", GCPProject: "cfg-project"})
	if client.gcpProject != "cfg-project" || client.gcpRegion != "cfg-region" {
		t.Errorf("config precedence = %q/%q, want cfg-region/cfg-project", client.gcpRegion, client.gcpProject)
	}
}

// writeTokenServerCreds points GOOGLE_APPLICATION_CREDENTIALS at a service
// account key whose token_uri is the given handler, so token refreshes hit a mock.
func writeTokenServerCreds(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": "sa@example-project.iam.gserviceaccount.com",
		"token_uri":    srv.URL,
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, creds, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

// TestVertexTokenSourceUsableAfterConstruction guards the first-request
// "context canceled" regression: the token source must keep working once the
// constructor has returned.
func TestVertexTokenSourceUsableAfterConstruction(t *testing.T) {
	writeTokenServerCreds(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
	})

	client := NewAnthropicVertexClient(ClientConfig{Model: "claude-sonnet-5", GCPRegion: "global", GCPProject: "p"})
	if client.initErr != nil {
		t.Fatalf("initErr = %v", client.initErr)
	}
	creds, err := vertexCredentials()
	if err != nil {
		t.Fatalf("vertexCredentials: %v", err)
	}
	tok, err := creds.TokenSource.Token()
	if err != nil || tok.AccessToken != "tok" {
		t.Fatalf("Token() = %v, %v; want tok", tok, err)
	}
}

// TestVertexTokenRefreshIsBounded guards against a stalled token endpoint
// blocking a request forever, independent of the Messages request timeout.
func TestVertexTokenRefreshIsBounded(t *testing.T) {
	release := make(chan struct{})
	writeTokenServerCreds(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })
	old := vertexTokenRefreshTimeout
	vertexTokenRefreshTimeout = 100 * time.Millisecond
	t.Cleanup(func() { vertexTokenRefreshTimeout = old })

	creds, err := vertexCredentials()
	if err != nil {
		t.Fatalf("vertexCredentials: %v", err)
	}
	start := time.Now()
	if _, err := creds.TokenSource.Token(); err == nil {
		t.Fatal("Token() succeeded against a stalled endpoint")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Token() took %v, want it bounded near the refresh timeout", d)
	}
}

// TestVertexClientReportsCredentialFailureAsError is the guard against the
// SDK's panic or error when Application Default
// Credentials cannot be resolved. A CLI must not hand a user a stack trace
// because `gcloud auth application-default login` was never run.
func TestVertexClientReportsCredentialFailureAsError(t *testing.T) {
	// GOOGLE_APPLICATION_CREDENTIALS pointed at a file that does not exist
	// makes google.FindDefaultCredentials fail deterministically, without
	// depending on whether the machine running the test happens to have gcloud
	// ADC configured or is reachable to the GCE metadata server.
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "nonexistent-creds.json"))

	client := NewAnthropicVertexClient(ClientConfig{
		Model:      "claude-sonnet-5",
		GCPRegion:  "us-east5",
		GCPProject: "example-project",
	})
	if client == nil {
		t.Fatal("NewAnthropicVertexClient returned nil; it must always return a client so the error can surface per-request")
	}
	if client.initErr == nil {
		t.Fatal("initErr = nil for a vertex client pointed at a nonexistent credentials file")
	}
	if !strings.Contains(client.initErr.Error(), "gcloud auth application-default login") {
		t.Errorf("initErr %q does not suggest the fix", client.initErr)
	}
	if _, err := client.CompletionsWithCtx(t.Context(), ChatRequest{}); err == nil {
		t.Error("CompletionsWithCtx returned nil error despite a construction failure")
	}
}
